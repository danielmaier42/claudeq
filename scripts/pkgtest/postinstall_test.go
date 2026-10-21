package pkgtest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Run the shipped shell script against a simulated Mac. No real launchd job,
// process, application or user data is changed. Fake time makes retries instant.
func TestPostinstall(t *testing.T) {
	for _, mode := range []string{"upgrade", "retry", "stale", "legacy", "missing", "broken-version", "install-failure", "console-root", "login-record", "unresolved-live", "headless"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			home := filepath.Join(dir, "Home with spaces")
			app := filepath.Join(dir, "DispatchQ.app")
			bin := filepath.Join(dir, "bin")
			for _, path := range []string{home, bin, filepath.Join(app, "Contents", "MacOS"), filepath.Join(home, "Library", "LaunchAgents")} {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path, body string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			// Both stale plists must be removed before invoking install, including
			// ClaudeQ's old label, which would otherwise restart the 1.6.2 daemon.
			for _, label := range []string{"claudeq", "dispatchq"} {
				write(filepath.Join(home, "Library", "LaunchAgents", "de.maierdaniel."+label+".plist"), "stale")
			}
			stub := `#!/bin/sh
name=${0##*/}
echo "$name $*" >> "$FIXTURE/events"
case "$name" in
stat)
 case "$MODE" in console-root|login-record|unresolved-live|headless) echo root ;; *) echo fixture ;; esac ;;
scutil)
 case "$MODE" in login-record|unresolved-live|headless) echo ' Name : loginwindow' ;; *) echo ' Name : fixture' ;; esac ;;
who) [ "$MODE" != login-record ] || echo 'fixture console Oct 21 10:00' ;;
id) echo 501 ;;
dscl) echo "NFSHomeDirectory: $FIXTURE/Home with spaces" ;;
launchctl)
 if [ "$1" = asuser ]; then shift 2; exec "$@"; fi ;;
sudo) shift 3; exec "$@" ;;
dispatchqd)
 if [ "$1" = --version ]; then
  [ "$MODE" = broken-version ] && exit 1
  echo 2.0.2; exit 0
 fi
 for label in claudeq dispatchq; do
  [ ! -f "$FIXTURE/Home with spaces/Library/LaunchAgents/de.maierdaniel.$label.plist" ] || exit 9
 done
 [ "$MODE" = install-failure ] && exit 1
 if [ -f "$FIXTURE/attempt" ]; then touch "$FIXTURE/ready"; else touch "$FIXTURE/attempt"; fi
 [ "$MODE" != retry ] && touch "$FIXTURE/ready"
 exit 0 ;;
curl)
 if [ "$MODE" = install-failure ] || { [ -f "$FIXTURE/ready" ] && [ "$MODE" != stale ]; }; then
  echo '{"current":"2.0.2"}'
 else echo '{"current":"1.6.2"}'; fi ;;
pgrep) [ "$MODE" = legacy ] || [ "$MODE" = unresolved-live ] ;;
lsof|mdfind) exit 0 ;;
date)
 n=0; [ ! -f "$FIXTURE/clock" ] || n=$(cat "$FIXTURE/clock")
 n=$((n+10)); echo "$n" > "$FIXTURE/clock"; echo "$n" ;;
sleep|pkill|pkgutil|open) exit 0 ;;
visudo) exit 1 ;;
*) exit 99 ;;
esac
`
			for _, name := range []string{"stat", "scutil", "who", "id", "dscl", "launchctl", "sudo", "curl", "pgrep", "lsof", "mdfind", "date", "sleep", "pkill", "pkgutil", "open", "visudo"} {
				write(filepath.Join(bin, name), stub)
			}
			if mode != "missing" {
				write(filepath.Join(app, "Contents", "MacOS", "dispatchqd"), stub)
			}
			source, err := os.ReadFile("../pkg/postinstall")
			if err != nil {
				t.Fatal(err)
			}
			script := strings.NewReplacer("/Applications/DispatchQ.app", app, "/Applications/ClaudeQ.app", filepath.Join(dir, "ClaudeQ.app"), "/etc/sudoers.d/", dir+"/sudoers-", "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "PATH="+bin+":/usr/bin:/bin").Replace(string(source))
			path := filepath.Join(dir, "postinstall")
			write(path, script)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", path)
			cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "FIXTURE="+dir, "MODE="+mode)
			out, runErr := cmd.CombinedOutput()
			success := mode == "upgrade" || mode == "retry" || mode == "console-root" || mode == "login-record"
			headless := mode == "headless"
			if (runErr == nil) != (success || headless) {
				t.Fatalf("mode %s: %v\n%s", mode, runErr, out)
			}
			events, _ := os.ReadFile(filepath.Join(dir, "events"))
			log := string(events)
			if strings.Contains(log, "open "+app) != success {
				t.Fatalf("opened before successful takeover:\n%s", log)
			}
			if mode != "missing" && mode != "broken-version" && mode != "unresolved-live" && !headless {
				stop := strings.Index(log, "pkill -TERM")
				for _, label := range []string{"claudeq", "dispatchq"} {
					bootout := strings.Index(log, "launchctl bootout gui/501/de.maierdaniel."+label)
					if bootout < 0 || stop < bootout {
						t.Fatalf("killed before bootout:\n%s", log)
					}
				}
				if !strings.Contains(log, "pkill -KILL -u 501 -x claudeqd") {
					t.Fatalf("missing scoped legacy escalation:\n%s", log)
				}
			}
			if mode == "unresolved-live" && !strings.Contains(string(out), "FAILED: cannot resolve GUI user") {
				t.Fatalf("silently skipped a live daemon: %s", out)
			}
			if mode == "console-root" && !strings.Contains(log, "scutil") {
				t.Fatal("SystemConfiguration not consulted")
			}
			if mode == "retry" && strings.Count(log, "\ndispatchqd install\n") != 2 {
				t.Fatalf("retry not exercised:\n%s", log)
			}
		})
	}
}
