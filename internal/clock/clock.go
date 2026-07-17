// Package clock provides a small time abstraction so scheduling and limit
// logic can be tested deterministically without wall-clock sleeps.
package clock

import "time"

// Clock reports the current time. Production code uses [Real]; tests use [Fake].
type Clock interface {
	Now() time.Time
}

// Real is a [Clock] backed by the system clock.
type Real struct{}

// Now returns the current system time.
func (Real) Now() time.Time { return time.Now() }

// Fake is a controllable [Clock] for tests. The zero value is not usable;
// construct it with [NewFake].
type Fake struct {
	current time.Time
}

// NewFake returns a [Fake] clock fixed at t.
func NewFake(t time.Time) *Fake { return &Fake{current: t} }

// Now returns the fake clock's current time.
func (f *Fake) Now() time.Time { return f.current }

// Set moves the fake clock to t.
func (f *Fake) Set(t time.Time) { f.current = t }

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) { f.current = f.current.Add(d) }
