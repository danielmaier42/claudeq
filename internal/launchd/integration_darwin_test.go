package launchd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/system"
)

// Opt in only inside a logged-in macOS GUI session. The unique service runs
// sleep, not DispatchQ, and never touches the user's real LaunchAgents.
func TestDisabledServiceRealLaunchd(t *testing.T) {
	if os.Getenv("DISPATCHQ_TEST_LAUNCHD") != "1" {
		t.Skip("requires explicit GUI launchd integration run")
	}
	ctx := context.Background()
	label := fmt.Sprintf("de.maierdaniel.dispatchq.test.%d", os.Getpid())
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("launchctl", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("launchctl %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	t.Cleanup(func() {
		_ = exec.Command("launchctl", "bootout", target).Run()
		_ = exec.Command("launchctl", "enable", target).Run()
	})
	run("disable", target)
	dir := t.TempDir()
	plist, err := Plist(Config{Label: label, BinPath: "/bin/sleep", Args: []string{"60"}, StdoutPath: filepath.Join(dir, "out.log"), StderrPath: filepath.Join(dir, "err.log")})
	if err != nil {
		t.Fatal(err)
	}
	a := Agent{Runner: system.Real{}, Dir: dir, Label: label, UID: os.Getuid()}
	if err := a.Install(ctx, plist); err != nil {
		t.Fatal(err)
	}
	if out := run("print", target); !strings.Contains(out, "pid = ") {
		t.Fatalf("enabled service did not start: %s", out)
	}
}
