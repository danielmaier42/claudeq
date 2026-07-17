package clock

import (
	"testing"
	"time"
)

func TestFakeSetAndAdvance(t *testing.T) {
	start := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	f := NewFake(start)

	if !f.Now().Equal(start) {
		t.Fatalf("Now() = %v, want %v", f.Now(), start)
	}

	f.Advance(90 * time.Minute)
	if want := start.Add(90 * time.Minute); !f.Now().Equal(want) {
		t.Fatalf("after Advance, Now() = %v, want %v", f.Now(), want)
	}

	reset := time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)
	f.Set(reset)
	if !f.Now().Equal(reset) {
		t.Fatalf("after Set, Now() = %v, want %v", f.Now(), reset)
	}
}

func TestRealClockAdvances(t *testing.T) {
	var c Real
	first := c.Now()
	if c.Now().Before(first) {
		t.Fatal("real clock went backwards")
	}
}
