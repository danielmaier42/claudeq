package limit

import (
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
)

func TestGateStartsOpen(t *testing.T) {
	g := New(clock.NewFake(time.Unix(0, 0)))
	if !g.Open() {
		t.Fatal("new gate should be open")
	}
	if !g.BlockedUntil().IsZero() {
		t.Fatal("open gate should report zero BlockedUntil")
	}
}

func TestBlockForClosesThenReopens(t *testing.T) {
	start := time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC)
	fc := clock.NewFake(start)
	g := New(fc)

	g.BlockFor(2 * time.Hour)

	if g.Open() {
		t.Fatal("gate should be closed right after BlockFor")
	}
	if want := start.Add(2 * time.Hour); !g.BlockedUntil().Equal(want) {
		t.Fatalf("BlockedUntil = %v, want %v", g.BlockedUntil(), want)
	}

	fc.Advance(2 * time.Hour)
	if !g.Open() {
		t.Fatal("gate should reopen once the block time passes")
	}
	if !g.BlockedUntil().IsZero() {
		t.Fatal("reopened gate should report zero BlockedUntil")
	}
}

func TestLongestBlockWins(t *testing.T) {
	start := time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC)
	fc := clock.NewFake(start)
	g := New(fc)

	g.BlockFor(3 * time.Hour)
	g.BlockFor(1 * time.Hour) // shorter: must not shorten the existing block

	if want := start.Add(3 * time.Hour); !g.BlockedUntil().Equal(want) {
		t.Fatalf("BlockedUntil = %v, want %v (shorter block must not win)", g.BlockedUntil(), want)
	}
}

func TestConcurrentBlockAndRead(_ *testing.T) {
	fc := clock.NewFake(time.Unix(0, 0))
	g := New(fc)

	done := make(chan struct{})
	go func() {
		for range 1000 {
			g.BlockFor(time.Minute)
		}
		close(done)
	}()
	for range 1000 {
		_ = g.Open()
		_ = g.BlockedUntil()
	}
	<-done
}
