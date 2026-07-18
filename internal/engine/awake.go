package engine

import (
	"os/exec"
	"runtime"
	"sync"
)

// sleepGuard keeps the Mac awake while runs are in flight by holding a
// `caffeinate -i` assertion (prevents idle system sleep), so a task can't be
// frozen mid-run when the machine would otherwise idle-sleep. It is
// reference-counted, so concurrent runs share a single assertion. No-op off
// macOS or if caffeinate can't start.
type sleepGuard struct {
	mu  sync.Mutex
	n   int
	cmd *exec.Cmd
}

func (g *sleepGuard) acquire() {
	if runtime.GOOS != "darwin" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	if g.n == 1 && g.cmd == nil {
		cmd := exec.Command("caffeinate", "-i")
		if err := cmd.Start(); err == nil {
			g.cmd = cmd
			go func() { _ = cmd.Wait() }() // reap once it's killed
		}
	}
}

func (g *sleepGuard) release() {
	if runtime.GOOS != "darwin" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.n > 0 {
		g.n--
	}
	if g.n == 0 && g.cmd != nil {
		_ = g.cmd.Process.Kill()
		g.cmd = nil
	}
}
