// Package limit implements the reactive, global rate-limit gate (PLAN.md D1).
//
// There is no reliable way to query the remaining allowance ahead of time, so
// the gate is purely reactive: a task simply runs, and if Claude Code reports a
// rate limit the gate is Blocked until a derived reset time. While blocked, no
// new task is started; the block clears automatically once the time passes.
package limit

import (
	"sync"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
)

// Gate is a concurrency-safe global rate-limit gate shared by all tasks.
type Gate struct {
	clock clock.Clock

	mu           sync.Mutex
	blockedUntil time.Time // zero => open
}

// New returns an open Gate using the given clock.
func New(c clock.Clock) *Gate {
	return &Gate{clock: c}
}

// Block closes the gate until until. A call with an earlier time than the
// current block is ignored, so the latest/longest known reset always wins.
func (g *Gate) Block(until time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if until.After(g.blockedUntil) {
		g.blockedUntil = until
	}
}

// BlockFor closes the gate for the given delay from now. It is the fallback
// entry point for rate-limit events that expose a retry delay but no absolute
// reset timestamp (PLAN.md V2); when the CLI reports one (rate_limit_event),
// the engine uses Block with that time directly.
func (g *Gate) BlockFor(delay time.Duration) {
	g.Block(g.clock.Now().Add(delay))
}

// Open reports whether a task may start now.
func (g *Gate) Open() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.now().Before(g.blockedUntil)
}

// BlockedUntil returns the time the gate reopens, or the zero time if open.
func (g *Gate) BlockedUntil() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.now().Before(g.blockedUntil) {
		return g.blockedUntil
	}
	return time.Time{}
}

func (g *Gate) now() time.Time { return g.clock.Now() }
