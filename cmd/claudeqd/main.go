// Command claudeqd is the claudeq background daemon: it runs the scheduling
// loop that executes due tasks via the Claude Code CLI (PLAN.md build phase 2).
//
// Usage:
//
//	claudeqd --version
//	claudeqd run [--interval 5s]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/engine"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "claudeqd:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Println(version.String())
		return nil
	}
	if len(args) == 0 || args[0] != "run" {
		fmt.Println("usage: claudeqd run [--interval 5s] | claudeqd --version")
		return nil
	}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	interval := fs.Duration("interval", 5*time.Second, "scheduler tick interval")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	home, err := store.DefaultHome()
	if err != nil {
		return err
	}
	st, err := store.Open(home)
	if err != nil {
		return err
	}

	c := clock.Real{}
	eng := engine.New(st, limit.New(c), &executor.Executor{}, c)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("claudeqd %s: watching %s (tick %s)\n", version.String(), home, *interval)
	if err := eng.Loop(ctx, *interval); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	fmt.Println("claudeqd: stopped")
	return nil
}
