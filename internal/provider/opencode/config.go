package opencode

import "sync"

// cachedProbe holds the answer to a question that is expensive to ask and
// cannot change while the daemon runs (where the CLI is).
type cachedProbe[T any] struct {
	once sync.Once
	val  T
}

func (c *cachedProbe[T]) do(ask func() T) T {
	c.once.Do(func() { c.val = ask() })
	return c.val
}
