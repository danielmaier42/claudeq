package review

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsShellScript(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"curl -s x", true},
		{"#!/bin/zsh\necho", true},
		{"#!/usr/bin/env bash\necho", true},
		{"#!/usr/bin/env -S bash -e\necho", true},
		{"#!/usr/bin/env python3\nprint(1)", false},
		{"#!/opt/homebrew/bin/node\n", false},
	}
	for _, tc := range tests {
		if got := isShellScript(tc.src); got != tc.want {
			t.Errorf("isShellScript(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestExtractCommands(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"pipeline and lists", "curl -s x | jq .a && gh pr list; rg foo || true", []string{"curl", "jq", "gh", "rg"}},
		{"shebang and comments skipped", "#!/bin/zsh\n# run brew here\nfd x # then brew", []string{"fd"}},
		{"builtins and keywords", "cd /x\nif [ -f a ]; then echo hi; fi\nexport A=1", nil},
		{"prefixes name the real command", "sudo -n launchctl list\nFOO=1 BAR=2 make all\nnohup rsync a b", []string{"launchctl", "make", "rsync"}},
		{"command substitution", "n=$(wc -l < f)\nx=`date +%s`", []string{"wc", "date"}},
		{"quoted text is data", "echo \"run jq; then gh\" 'x | yq'", nil},
		{"paths and variables are not names", "/opt/homebrew/bin/jq .\n$CLAUDEQ_BIN queue\n./build.sh", nil},
		{"functions the script defines", "notify() { osascript -e x; }\nfunction sync_it { :; }\nnotify\nsync_it", []string{"osascript"}},
		{"here-document body is data", "cat <<EOF > out.txt\nfoo bar\nbaz | qux\nEOF\ntail out.txt", []string{"cat", "tail"}},
		{"here-string is not a here-document", "grep x <<< hello\nsort f", []string{"grep", "sort"}},
		{"for and case", "for f in a b; do gzip \"$f\"; done\ncase $1 in\n  start) launchctl start x ;;\n  *) exit 1 ;;\nesac\nuptime", []string{"gzip", "launchctl", "uptime"}},
		{"probing is not running", "if ! command -v jq >/dev/null; then exit 0; fi\nuptime", []string{"uptime"}},
		{"fd redirections are not commands", "make 2>&1 | tee log\necho x >&2\nls &>/dev/null", []string{"make", "tee", "ls"}},
		{"parentheses that start no command", "n=$((n + 1))\narr=(alpha beta)\n[[ $x =~ (a|b) ]] && uptime\n( cd /x && gzip f )", []string{"uptime", "gzip"}},
		{"here-document in a comment starts nothing", "# see cat <<EOF\nfoo x\ncat <<'END'\nbar y\nEND\nbaz", []string{"foo", "cat", "baz"}},
		{"sudo options with a value", "sudo -u bob -n make", []string{"make"}},
		{"duplicates collapse", "jq a\njq b", []string{"jq"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractCommands(tc.src)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("ExtractCommands(%q) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestInspectCommands(t *testing.T) {
	home := t.TempDir()
	onPath, elsewhere := t.TempDir(), filepath.Join(home, ".local", "bin")
	mustExec(t, filepath.Join(onPath, "curl"))
	mustExec(t, filepath.Join(elsewhere, "cq-fake-tool"))
	mustWrite(t, filepath.Join(onPath, "notexec"), "x")

	got := InspectCommands("curl x | cq-fake-tool . && notexec && nothere", onPath, home)
	want := []Command{
		{Name: "curl", OnPath: filepath.Join(onPath, "curl")},
		{Name: "cq-fake-tool", Elsewhere: filepath.Join(elsewhere, "cq-fake-tool")},
		{Name: "notexec"},
		{Name: "nothere"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("command %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestInspectResolvesHomeVariable(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, "in.csv"), "a,b\n")
	cands := Inspect("cat \"$HOME/in.csv\" > \"${HOME}/out/x.csv\"", "", home)
	if got := find(t, cands, "$HOME/in.csv"); !got.Exists {
		t.Errorf("$HOME must resolve like ~: %+v", got)
	}
	if got := find(t, cands, "$HOME/out/x.csv"); got.Exists || got.ParentExists {
		t.Errorf("a write into a missing directory must be visible: %+v", got)
	}
}

func TestScriptReviewStatesCommandsAndScriptRules(t *testing.T) {
	dir := t.TempDir()
	bin := t.TempDir()
	mustExec(t, filepath.Join(bin, "curl"))
	r, c := reviewer(t, `{"ok":true}`)
	r.PATH = bin
	src := "#!/bin/sh\ncurl -s x | jqq . > out/r.json"
	if _, err := r.Review(context.Background(), Request{Kind: KindScript, WorkingDir: dir, Prompt: src, Provider: claude}); err != nil {
		t.Fatal(err)
	}
	if c.req.System != systemPrompt(KindScript) || !strings.Contains(c.req.System, "executed as a program") {
		t.Error("a script must be reviewed by the script rules")
	}
	for _, want := range []string{
		"## The script to review",
		"`curl` → " + filepath.Join(bin, "curl") + " — found",
		"`jqq` — NOT on the PATH",
		"its parent directory " + filepath.Join(dir, "out") + " is missing too",
	} {
		if !strings.Contains(c.req.Text, want) {
			t.Errorf("message missing %q\n---\n%s", want, c.req.Text)
		}
	}
}

func TestNonShellScriptListsNoCommands(t *testing.T) {
	r, c := reviewer(t, `{"ok":true}`)
	src := "#!/usr/bin/env python3\nimport os\nprint(os.getcwd())"
	if _, err := r.Review(context.Background(), Request{Kind: KindScript, WorkingDir: t.TempDir(), Prompt: src, Provider: claude}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.req.Text, "None found (not a shell script") {
		t.Errorf("a python script has no shell commands to look up:\n%s", c.req.Text)
	}
}

func TestTaskReviewListsNoCommands(t *testing.T) {
	r, c := reviewer(t, `{"ok":true}`)
	if _, err := r.Review(context.Background(), Request{Kind: KindTask, WorkingDir: t.TempDir(), Prompt: "run jq on it", Provider: claude}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.req.Text, "Commands the script runs") {
		t.Error("an agent prompt is prose, not a program: no command lookup")
	}
}

func mustExec(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // a fake executable for the lookup
		t.Fatal(err)
	}
}
