package api

import (
	"context"
	"errors"
	"testing"
)

type accentRunner struct {
	out string
	err error
}

func (a accentRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return []byte(a.out), a.err
}

func TestMacAccentMapsIndex(t *testing.T) {
	if got := MacAccent(accentRunner{out: "1\n"})(); got != "#f7821b" {
		t.Fatalf("orange accent = %q, want #f7821b", got)
	}
	if got := MacAccent(accentRunner{out: "4"})(); got != "#007aff" {
		t.Fatalf("blue accent = %q, want #007aff", got)
	}
}

func TestMacAccentUnavailableOrUnknown(t *testing.T) {
	if got := MacAccent(accentRunner{err: errors.New("unset")})(); got != "" {
		t.Fatalf("missing key should yield empty, got %q", got)
	}
	if got := MacAccent(accentRunner{out: "99"})(); got != "" {
		t.Fatalf("unknown index should yield empty, got %q", got)
	}
}
