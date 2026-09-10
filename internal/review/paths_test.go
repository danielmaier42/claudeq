package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractPaths(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   []string
	}{
		{"absolute", "Read /etc/hosts and stop.", []string{"/etc/hosts"}},
		{"home", "Guidelines live in ~/docs/rules.md.", []string{"~/docs/rules.md"}},
		{"dot relative", "Check ./build and ../shared/lib.", []string{"./build", "../shared/lib"}},
		{"bare relative", "Review src/main.go please.", []string{"src/main.go"}},
		{"bare filename with known extension", "Follow AGENTS.md exactly.", []string{"AGENTS.md"}},
		{"bare filename unknown extension", "Version v1.2 of the thing.", nil},
		{"backticks and quotes", "Open `docs/spec.md` then \"notes.txt\".", []string{"docs/spec.md", "notes.txt"}},
		{"markdown link", "See [the spec](docs/spec.md).", []string{"docs/spec.md"}},
		{"url is not a path", "Fetch https://example.com/a/b.md now.", nil},
		{"env var is not a path", "Write to $HOME/out.md.", nil},
		{"scp remote is not a path", "Copy from host:~/x or user@host/y.md.", nil},
		{"editor line suffix is cut", "Fix src/main.go:42:7 now.", []string{"src/main.go"}},
		{"prose is not a path", "Do it, e.g. quickly. Then stop.", nil},
		{"trailing punctuation", "Update README.md, then docs/x.md; finally /tmp/y.md.", []string{"README.md", "docs/x.md", "/tmp/y.md"}},
		{"bare dot and slash ignored", "Run . and / and ~ nowhere.", nil},
		{"duplicates collapse", "Open a/b.md and a/b.md again.", []string{"a/b.md"}},
		{"glob splits on the star", "Review src/*.go now.", []string{"src/"}},
		{"empty prompt", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractPaths(tc.prompt)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("ExtractPaths(%q) = %q, want %q", tc.prompt, got, tc.want)
			}
		})
	}
}

func TestExtractPathsCapsCandidates(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxCandidates*2; i++ {
		b.WriteString("/tmp/p")
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(string(rune('a' + i/26)))
		b.WriteString(" ")
	}
	if got := len(ExtractPaths(b.String())); got != maxCandidates {
		t.Errorf("got %d candidates, want the cap %d", got, maxCandidates)
	}
}

// find returns the candidate for a raw path, so a test can assert on one entry
// without depending on the order of the rest.
func find(t *testing.T, cands []Candidate, raw string) Candidate {
	t.Helper()
	for _, c := range cands {
		if c.Raw == raw {
			return c
		}
	}
	t.Fatalf("no candidate for %q in %+v", raw, cands)
	return Candidate{}
}

func TestInspect(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(dir, "GUIDELINES.md"), "always run the tests\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(home, "notes.md"), "home notes\n")

	prompt := "Follow GUIDELINES.md, look in docs/, write docs/out.md and reports/summary.md, " +
		"read ~/notes.md and /nope/really/missing.md."
	cands := Inspect(prompt, dir, home)

	guide := find(t, cands, "GUIDELINES.md")
	if !guide.Exists || guide.IsDir || guide.Content != "always run the tests\n" {
		t.Errorf("GUIDELINES.md: %+v", guide)
	}
	if got := find(t, cands, "docs/"); !got.Exists || !got.IsDir {
		t.Errorf("docs should be an existing directory: %+v", got)
	}
	// A file that does not exist yet inside a directory that does is the normal
	// "write a report here" case, not a problem.
	out := find(t, cands, "docs/out.md")
	if out.Exists || !out.ParentExists {
		t.Errorf("docs/out.md: want missing with an existing parent, got %+v", out)
	}
	// A file inside a directory that is missing too is the case worth flagging.
	sum := find(t, cands, "reports/summary.md")
	if sum.Exists || sum.ParentExists || sum.Parent != filepath.Join(dir, "reports") {
		t.Errorf("reports/summary.md: want missing with a missing parent, got %+v", sum)
	}
	if got := find(t, cands, "~/notes.md"); got.Resolved != filepath.Join(home, "notes.md") || !got.Exists {
		t.Errorf("~/notes.md should expand to home and exist: %+v", got)
	}
	if got := find(t, cands, "/nope/really/missing.md"); got.Exists || got.ParentExists {
		t.Errorf("/nope/really/missing.md should be missing: %+v", got)
	}
}

func TestInspectWithoutWorkingDir(t *testing.T) {
	cands := Inspect("Read notes.md and /etc/hosts.", "", t.TempDir())
	if got := find(t, cands, "notes.md"); got.Resolved != "" || got.Exists {
		t.Errorf("a relative path with no working directory stays unresolved: %+v", got)
	}
	if got := find(t, cands, "/etc/hosts"); got.Resolved == "" {
		t.Errorf("an absolute path resolves without a working directory: %+v", got)
	}
}

func TestInspectWithoutHome(t *testing.T) {
	if got := find(t, Inspect("Read ~/notes.md.", t.TempDir(), ""), "~/notes.md"); got.Resolved != "" {
		t.Errorf("~ must not be guessed without a home directory: %+v", got)
	}
}

func TestInspectSkipsContentItCannotUse(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "big.md"), strings.Repeat("x", maxFileBytes+1))
	mustWrite(t, filepath.Join(dir, "empty.md"), "")
	if err := os.WriteFile(filepath.Join(dir, "blob.txt"), []byte{'a', 0, 'b'}, 0o600); err != nil {
		t.Fatal(err)
	}
	cands := Inspect("Read big.md, empty.md and blob.txt.", dir, dir)
	for _, raw := range []string{"big.md", "empty.md", "blob.txt"} {
		if c := find(t, cands, raw); c.Content != "" {
			t.Errorf("%s: content should be skipped, got %d bytes", raw, len(c.Content))
		}
	}
	if c := find(t, cands, "big.md"); !c.Exists || c.Size != maxFileBytes+1 {
		t.Errorf("big.md should still be reported as an existing file: %+v", c)
	}
}

func TestInspectCapsFileCount(t *testing.T) {
	dir := t.TempDir()
	var prompt strings.Builder
	for i := 0; i < maxFiles+3; i++ {
		name := "f" + string(rune('a'+i)) + ".md"
		mustWrite(t, filepath.Join(dir, name), "content\n")
		prompt.WriteString(name + " ")
	}
	var withBody int
	for _, c := range Inspect(prompt.String(), dir, dir) {
		if c.Content != "" {
			withBody++
		}
	}
	if withBody != maxFiles {
		t.Errorf("read %d file contents, want the cap %d", withBody, maxFiles)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
