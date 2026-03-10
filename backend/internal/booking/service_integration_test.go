package booking

import (
	"errors"
	"testing"
	"time"
)

func TestBookingFlowWithInMemoryDoubles(t *testing.T) {
	locks := NewInMemoryLockStore()
	bookings := NewInMemoryBookingStore()
	svc := NewService(locks, bookings)

	if err := svc.LockSeat("show-1", "A1", "user-1", 5*time.Minute); err != nil {
		t.Fatalf("lock user-1 failed: %v", err)
	}

	if err := svc.LockSeat("show-1", "A1", "user-2", 5*time.Minute); !errors.Is(err, ErrAlreadyLocked) {
		t.Fatalf("expected ErrAlreadyLocked but got %v", err)
	}

	if err := svc.ConfirmSeat("show-1", "A1", "user-2", "Demo Movie", "2026-03-10"); !errors.Is(err, ErrNotLockOwner) {
		t.Fatalf("expected ErrNotLockOwner but got %v", err)
	}

	if err := svc.ConfirmSeat("show-1", "A1", "user-1", "Demo Movie", "2026-03-10"); err != nil {
		t.Fatalf("confirm user-1 failed: %v", err)
	}

	if !bookings.IsBooked("show-1", "A1") {
		t.Fatalf("expected seat to be booked")
	}

	if _, ok := locks.GetOwner("show-1", "A1"); ok {
		t.Fatalf("expected lock to be released after confirm")
	}

	if err := svc.LockSeat("show-1", "A1", "user-2", 5*time.Minute); !errors.Is(err, ErrAlreadyBooked) {
		t.Fatalf("expected ErrAlreadyBooked but got %v", err)
	}
}

func TestReleaseSeatOwnership(t *testing.T) {
	locks := NewInMemoryLockStore()
	bookings := NewInMemoryBookingStore()
	svc := NewService(locks, bookings)

	if err := svc.LockSeat("show-2", "B2", "owner", 5*time.Minute); err != nil {
		t.Fatalf("lock owner failed: %v", err)
	}

	if err := svc.ReleaseSeat("show-2", "B2", "other"); !errors.Is(err, ErrNotLockOwner) {
		t.Fatalf("expected ErrNotLockOwner but got %v", err)
	}

	if err := svc.ReleaseSeat("show-2", "B2", "owner"); err != nil {
		t.Fatalf("owner release failed: %v", err)
	}

	if _, ok := locks.GetOwner("show-2", "B2"); ok {
		t.Fatalf("expected lock removed")
	}
}
