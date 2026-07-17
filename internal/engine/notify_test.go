package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/store"
)

type capturingNotifier struct {
	mu   sync.Mutex
	msgs []notify.Notification
}

func (c *capturingNotifier) Notify(_ context.Context, n notify.Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, n)
	return nil
}

func (c *capturingNotifier) all() []notify.Notification {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]notify.Notification(nil), c.msgs...)
}

func TestNotifyOnFailureNotOnSuccess(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &stub{result: func(_ executor.Request, _ int) executor.Result {
		return executor.Result{Status: store.StatusFailed, ExitCode: 2, Message: "run failed (exit 2)"}
	}}
	e, st := newTestEngine(t, r, fc)
	n := &capturingNotifier{}
	e.SetNotifier(n)
	saveTasks(t, st, asapTask("boom", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	msgs := n.all()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification on failure, got %d", len(msgs))
	}
	if msgs[0].Title == "" || msgs[0].Message == "" {
		t.Fatalf("notification should have title and message: %+v", msgs[0])
	}
}

func TestNoNotifyOnSuccess(t *testing.T) {
	fc := clock.NewFake(time.Now())
	e, st := newTestEngine(t, &stub{}, fc) // stub default = success
	n := &capturingNotifier{}
	e.SetNotifier(n)
	saveTasks(t, st, asapTask("ok", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	if got := len(n.all()); got != 0 {
		t.Fatalf("expected no notification on success, got %d", got)
	}
}

func TestNotifyOnAuthError(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &stub{result: func(_ executor.Request, _ int) executor.Result {
		return executor.Result{Status: store.StatusAuthError, ExitCode: 1}
	}}
	e, st := newTestEngine(t, r, fc)
	n := &capturingNotifier{}
	e.SetNotifier(n)
	saveTasks(t, st, asapTask("auth", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	msgs := n.all()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification on auth error, got %d", len(msgs))
	}
}
