package review

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Limits on how much of the machine a single review is allowed to look at, so a
// prompt full of paths can neither stall the review nor blow up its input.
const (
	// maxCandidates caps how many distinct paths are checked.
	maxCandidates = 40
	// maxFileBytes is the largest file whose content is shown to the reviewer.
	maxFileBytes = 16 * 1024
	// maxFiles caps how many file contents are shown.
	maxFiles = 6
	// maxTotalFileBytes caps their combined size.
	maxTotalFileBytes = 48 * 1024
)

// Candidate is a path mentioned in a prompt, together with what claudeq found
// at that location on this machine.
type Candidate struct {
	// Raw is the path exactly as it appears in the prompt.
	Raw string
	// Resolved is the absolute path Raw refers to. Empty when a relative path
	// cannot be resolved because the task has no usable working directory.
	Resolved string
	// Exists reports whether something is at Resolved. A path claudeq is not
	// allowed to look at counts as existing (see Unreadable).
	Exists bool
	// Unreadable marks a path macOS refused to let claudeq stat, so Exists and
	// IsDir are not actually known.
	Unreadable bool
	// IsDir reports whether Resolved is a directory.
	IsDir bool
	// Parent is the directory Resolved sits in, and ParentExists whether that
	// directory is there — the case that separates "a file to be created in an
	// existing folder" from "a path into nowhere".
	Parent       string
	ParentExists bool
	// Content is the file's text, included only for an existing text file small
	// enough to inline (maxFileBytes), so the reviewer can judge whether
	// inlining it into the prompt is the right call. A bigger file is reported
	// by size alone — quoting a slice of it would cost tokens without changing
	// the answer, since it is too long to inline either way.
	Content string
	// Size is the file's size in bytes (0 for anything that is not a file).
	Size int64
}

// knownExts are the file extensions that make a bare, slash-less token (e.g.
// "AGENTS.md") count as a path. Anything with a slash is treated as a path
// regardless of extension.
var knownExts = map[string]bool{
	"md": true, "markdown": true, "txt": true, "rst": true, "adoc": true,
	"go": true, "py": true, "js": true, "mjs": true, "cjs": true, "ts": true, "tsx": true, "jsx": true,
	"rb": true, "rs": true, "java": true, "kt": true, "swift": true, "c": true, "h": true,
	"cc": true, "cpp": true, "hpp": true, "cs": true, "php": true, "sh": true, "bash": true, "zsh": true,
	"json": true, "yaml": true, "yml": true, "toml": true, "ini": true, "cfg": true, "conf": true,
	"env": true, "properties": true, "plist": true, "xml": true, "html": true, "htm": true,
	"css": true, "scss": true, "sql": true, "csv": true, "tsv": true, "log": true, "lock": true,
	"pdf": true, "png": true, "jpg": true, "jpeg": true, "gif": true, "svg": true, "webp": true,
	"zip": true, "tar": true, "gz": true, "sqlite": true, "db": true, "mod": true, "sum": true,
	"claudeq": true, "plantuml": true, "puml": true, "tf": true, "gradle": true, "make": true,
}

// tokenSeparators end a path token. Quotes, brackets and markdown punctuation
// wrap paths far more often than they appear inside one, so splitting on them
// pulls `path`, "path", [text](path) and <path> apart without any parsing.
const tokenSeparators = " \t\r\n\v\f\"'`<>|()[]{},;*=&"

// trailingPunct is stripped from a token's end: a path at the end of a sentence
// or clause carries the sentence's punctuation, never its own.
const trailingPunct = ".,;:!?"

// ExtractPaths finds the filesystem paths a prompt mentions, in order of first
// appearance and without duplicates. It is deliberately generous about what
// counts as a path — a false positive costs one stat and shows up as an extra
// line of context, while a miss is a check that never happens.
func ExtractPaths(prompt string) []string {
	var out []string
	seen := map[string]bool{}
	for _, tok := range strings.FieldsFunc(prompt, func(r rune) bool {
		return strings.ContainsRune(tokenSeparators, r)
	}) {
		p := cleanToken(tok)
		if p == "" || seen[p] || !looksLikePath(p) {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) >= maxCandidates {
			break
		}
	}
	return out
}

// lineSuffixRe matches the ":42" / ":42:7" a tool or an editor appends to a
// path. The location is not part of the path, so it is cut before the check.
var lineSuffixRe = regexp.MustCompile(`:\d+(:\d+)?$`)

// cleanToken strips the punctuation a path picks up from the prose around it.
func cleanToken(tok string) string {
	tok = strings.Trim(tok, "“”‘’…")
	// A trailing "." is sentence punctuation unless the whole token is "." or
	// "..", which are directories in their own right.
	for len(tok) > 0 && strings.ContainsRune(trailingPunct, rune(tok[len(tok)-1])) {
		if tok == "." || tok == ".." {
			break
		}
		tok = tok[:len(tok)-1]
	}
	return strings.TrimSpace(lineSuffixRe.ReplaceAllString(tok, ""))
}

// looksLikePath decides whether a cleaned token is worth checking on disk.
func looksLikePath(p string) bool {
	if len(p) < 2 || len(p) > 512 || !utf8.ValidString(p) {
		return false
	}
	// URLs, scp-style remotes and shell/template placeholders are not local
	// paths. A colon survives only in those forms here: a "file.go:42" location
	// suffix has already been cut by cleanToken.
	if strings.ContainsAny(p, ":$@") {
		return false
	}
	switch {
	case p == "." || p == ".." || p == "/" || p == "~":
		return false
	case strings.HasPrefix(p, "/"), strings.HasPrefix(p, "~/"),
		strings.HasPrefix(p, "./"), strings.HasPrefix(p, "../"):
		return true
	case strings.Contains(p, "/"):
		// A relative path like "docs/spec.md". Reject a lone trailing slash on a
		// single word ("and/or" is still accepted; a stat simply finds nothing).
		return strings.Trim(p, "/") != ""
	}
	// No slash: only a bare filename with a recognisable extension counts, so
	// ordinary prose ("e.g", "v1.2") does not turn into a path check.
	dot := strings.LastIndexByte(p, '.')
	if dot <= 0 || dot == len(p)-1 {
		return false
	}
	return knownExts[strings.ToLower(p[dot+1:])]
}

// Inspect resolves each path against workingDir and reports what is actually
// there. home is the user's home directory, used to expand "~"; an empty home
// leaves "~" paths unresolved rather than guessing.
func Inspect(prompt, workingDir, home string) []Candidate {
	raws := ExtractPaths(prompt)
	out := make([]Candidate, 0, len(raws))
	files, total := 0, 0
	for _, raw := range raws {
		c := Candidate{Raw: raw, Resolved: resolvePath(raw, workingDir, home)}
		if c.Resolved == "" {
			out = append(out, c)
			continue
		}
		fi, err := os.Stat(c.Resolved)
		switch {
		case err == nil:
			c.Exists, c.IsDir, c.Size = true, fi.IsDir(), fi.Size()
		case errors.Is(err, fs.ErrPermission):
			// macOS may withhold a folder from the daemon until the user grants
			// access. "Missing" would be a lie, so say we could not look.
			c.Exists, c.Unreadable = true, true
		}
		c.Parent = filepath.Dir(c.Resolved)
		c.ParentExists = dirExists(c.Parent)
		if !c.Exists || c.IsDir || c.Unreadable || files >= maxFiles || total >= maxTotalFileBytes {
			out = append(out, c)
			continue
		}
		if body, ok := readTextFile(c.Resolved, fi.Size()); ok {
			c.Content = body
			files++
			total += len(body)
		}
		out = append(out, c)
	}
	return out
}

// resolvePath turns a path as written into an absolute one. A relative path
// needs a working directory; without one it stays unresolved, because guessing
// a base would produce checks about a directory the task never runs in.
func resolvePath(raw, workingDir, home string) string {
	switch {
	case strings.HasPrefix(raw, "~/"):
		if home == "" {
			return ""
		}
		return filepath.Join(home, raw[2:])
	case filepath.IsAbs(raw):
		return filepath.Clean(raw)
	case workingDir == "":
		return ""
	default:
		return filepath.Join(workingDir, raw)
	}
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return errors.Is(err, fs.ErrPermission)
	}
	return fi.IsDir()
}

// readTextFile reads a file small enough to be inlined into a prompt. Anything
// bigger, binary, or unreadable is skipped: its content could not be the answer
// anyway, and shipping it would only cost tokens and time.
func readTextFile(path string, size int64) (body string, ok bool) {
	if size <= 0 || size > maxFileBytes {
		return "", false
	}
	data, err := os.ReadFile(path) //nolint:gosec // path comes from the prompt being reviewed, read-only
	if err != nil || len(data) == 0 || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", false
	}
	return string(data), true
}
