package api

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRunner records the last command and returns canned output.
type fakeRunner struct {
	name string
	args []string
	out  []byte
	err  error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.name = name
	f.args = args
	return f.out, f.err
}

func TestOSAScriptTerminalOpenerBuildsCommand(t *testing.T) {
	fr := &fakeRunner{}
	open := OSAScriptTerminalOpener(fr)
	if err := open(context.Background(), "/Users/me/my proj", []string{"/opt/claude", "--resume", "sess-1"}); err != nil {
		t.Fatalf("open: %v", err)
	}
	if fr.name != "osascript" {
		t.Fatalf("command = %q, want osascript", fr.name)
	}
	// The shell command is the trailing run-handler argument, after the -e pairs.
	got := fr.args[len(fr.args)-1]
	want := `cd '/Users/me/my proj' && '/opt/claude' '--resume' 'sess-1'`
	if got != want {
		t.Fatalf("shell command = %q, want %q", got, want)
	}
	// The script itself must reference the argument, not embed the command —
	// that is what makes AppleScript-escaping unnecessary.
	script := strings.Join(fr.args[:len(fr.args)-1], "\n")
	if !strings.Contains(script, "item 1 of argv") {
		t.Fatalf("script should pass the command via argv, got: %s", script)
	}
	if strings.Contains(script, "sess-1") {
		t.Fatal("script must not embed the shell command")
	}
}

func TestOSAScriptTerminalOpenerError(t *testing.T) {
	fr := &fakeRunner{out: []byte("execution error: Not authorized"), err: errors.New("exit status 1")}
	open := OSAScriptTerminalOpener(fr)
	err := open(context.Background(), "/tmp", []string{"claude"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Not authorized") {
		t.Fatalf("error should carry osascript output, got: %v", err)
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "'plain'"},
		{"with space", "'with space'"},
		{"o'brien", `'o'\''brien'`},
		{"", "''"},
		{`a"b$c`, `'a"b$c'`},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
