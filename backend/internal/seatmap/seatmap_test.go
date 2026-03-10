package seatmap

import "testing"

func TestLockKey(t *testing.T) {
	got := LockKey("show-1", "A1")
	want := "lock:seat:show-1:A1"
	if got != want {
		t.Fatalf("expected %s but got %s", want, got)
	}
}

func TestBuild(t *testing.T) {
	seats := Build(2, 3)
	if len(seats) != 6 {
		t.Fatalf("expected 6 seats but got %d", len(seats))
	}
	if seats[0].SeatID != "A1" || seats[5].SeatID != "B3" {
		t.Fatalf("unexpected seat ids: first=%s last=%s", seats[0].SeatID, seats[5].SeatID)
	}
	for _, s := range seats {
		if s.Status != "AVAILABLE" {
			t.Fatalf("expected AVAILABLE status but got %s", s.Status)
		}
	}
}
