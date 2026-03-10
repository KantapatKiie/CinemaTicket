package booking

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrAlreadyBooked = errors.New("seat already booked")
	ErrAlreadyLocked = errors.New("seat already locked")
	ErrLockNotFound  = errors.New("lock not found")
	ErrNotLockOwner  = errors.New("not lock owner")
)

type BookingRecord struct {
	ShowID   string
	SeatID   string
	UserID   string
	Movie    string
	ShowDate string
	Status   string
}

type LockStore interface {
	TryLock(showID, seatID, userID string, ttl time.Duration) (bool, error)
	GetOwner(showID, seatID string) (string, bool)
	Release(showID, seatID string) error
}

type BookingStore interface {
	IsBooked(showID, seatID string) bool
	Save(record BookingRecord) error
}

type Service struct {
	locks    LockStore
	bookings BookingStore
	now      func() time.Time
}

func NewService(locks LockStore, bookings BookingStore) *Service {
	return &Service{locks: locks, bookings: bookings, now: time.Now}
}

func (s *Service) LockSeat(showID, seatID, userID string, ttl time.Duration) error {
	if s.bookings.IsBooked(showID, seatID) {
		return ErrAlreadyBooked
	}
	ok, err := s.locks.TryLock(showID, seatID, userID, ttl)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAlreadyLocked
	}
	return nil
}

func (s *Service) ReleaseSeat(showID, seatID, userID string) error {
	owner, ok := s.locks.GetOwner(showID, seatID)
	if !ok {
		return ErrLockNotFound
	}
	if owner != userID {
		return ErrNotLockOwner
	}
	return s.locks.Release(showID, seatID)
}

func (s *Service) ConfirmSeat(showID, seatID, userID, movie, showDate string) error {
	if s.bookings.IsBooked(showID, seatID) {
		return ErrAlreadyBooked
	}
	owner, ok := s.locks.GetOwner(showID, seatID)
	if !ok {
		return ErrLockNotFound
	}
	if owner != userID {
		return ErrNotLockOwner
	}
	err := s.bookings.Save(BookingRecord{
		ShowID:   showID,
		SeatID:   seatID,
		UserID:   userID,
		Movie:    movie,
		ShowDate: showDate,
		Status:   "BOOKED",
	})
	if err != nil {
		return err
	}
	return s.locks.Release(showID, seatID)
}

type lockEntry struct {
	userID    string
	expiresAt time.Time
}

type InMemoryLockStore struct {
	mu    sync.Mutex
	locks map[string]lockEntry
	now   func() time.Time
}

func NewInMemoryLockStore() *InMemoryLockStore {
	return &InMemoryLockStore{locks: map[string]lockEntry{}, now: time.Now}
}

func (m *InMemoryLockStore) key(showID, seatID string) string {
	return showID + ":" + seatID
}

func (m *InMemoryLockStore) cleanExpired() {
	now := m.now()
	for k, v := range m.locks {
		if now.After(v.expiresAt) {
			delete(m.locks, k)
		}
	}
}

func (m *InMemoryLockStore) TryLock(showID, seatID, userID string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanExpired()
	k := m.key(showID, seatID)
	if _, ok := m.locks[k]; ok {
		return false, nil
	}
	m.locks[k] = lockEntry{userID: userID, expiresAt: m.now().Add(ttl)}
	return true, nil
}

func (m *InMemoryLockStore) GetOwner(showID, seatID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanExpired()
	k := m.key(showID, seatID)
	v, ok := m.locks[k]
	if !ok {
		return "", false
	}
	return v.userID, true
}

func (m *InMemoryLockStore) Release(showID, seatID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.locks, m.key(showID, seatID))
	return nil
}

type InMemoryBookingStore struct {
	mu    sync.Mutex
	items map[string]BookingRecord
}

func NewInMemoryBookingStore() *InMemoryBookingStore {
	return &InMemoryBookingStore{items: map[string]BookingRecord{}}
}

func (m *InMemoryBookingStore) key(showID, seatID string) string {
	return showID + ":" + seatID
}

func (m *InMemoryBookingStore) IsBooked(showID, seatID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.items[m.key(showID, seatID)]
	return ok && v.Status == "BOOKED"
}

func (m *InMemoryBookingStore) Save(record BookingRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[m.key(record.ShowID, record.SeatID)] = record
	return nil
}
