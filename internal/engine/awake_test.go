package engine

import (
	"runtime"
	"testing"
)

func TestSleepGuardRefcount(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("caffeinate is macOS-only")
	}
	var g sleepGuard
	g.acquire()
	g.acquire()
	if g.cmd == nil {
		t.Skip("caffeinate not available in this environment")
	}
	g.release()
	if g.cmd == nil {
		t.Fatal("assertion released while a run is still active (refcount broken)")
	}
	g.release()
	if g.cmd != nil {
		t.Fatal("assertion not released once no runs are active")
	}
}
