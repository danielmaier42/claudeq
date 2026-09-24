package review

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A script job runs under the daemon's environment, and launchd hands the
// daemon a minimal PATH that reads no shell profile. A tool the operator calls
// by name every day in Terminal is then simply not there at 3 a.m. — the most
// common way a script that "works" fails unattended. So for a shell script
// claudeq looks up each command it runs on that very PATH, and, when one is
// missing, in the places tools usually live, so the reviewer can offer the
// absolute path instead of guessing one.

// maxCommands caps how many distinct commands are looked up.
const maxCommands = 60

// Command is a program a shell script runs by name, and where claudeq finds it.
type Command struct {
	// Name is the command as written.
	Name string
	// OnPath is the executable the script's PATH resolves Name to, empty when
	// the PATH has none.
	OnPath string
	// Elsewhere is an executable of that name outside the PATH, in one of the
	// usual install locations. Only looked for when OnPath is empty.
	Elsewhere string
}

// shellInterpreters are the interpreters whose scripts claudeq reads commands
// from. Anything else (python, node, …) has no commands to look up this way.
var shellInterpreters = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true}

// isShellScript reports whether the script runs in a POSIX-style shell: no
// shebang (claudeq falls back to /bin/sh) or one naming a shell, directly or
// through env.
func isShellScript(src string) bool {
	if !strings.HasPrefix(src, "#!") {
		return true
	}
	line, _, _ := strings.Cut(src[2:], "\n")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return true
	}
	name := filepath.Base(fields[0])
	if name == "env" {
		for _, f := range fields[1:] {
			if !strings.HasPrefix(f, "-") {
				name = filepath.Base(f)
				break
			}
		}
	}
	return shellInterpreters[name]
}

// shellPrefix are words at a command position that are not the program but
// hand over to the next word: keywords, and the wrappers that run a command.
var shellPrefix = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "while": true, "until": true, "do": true,
	"time": true, "!": true, "{": true, "exec": true, "nohup": true, "sudo": true, "noglob": true,
}

// shellBuiltin are words that are not programs and whose arguments are not
// commands either: builtins and closing keywords. "command" belongs here too —
// "command -v jq" probes for jq rather than running it.
var shellBuiltin = map[string]bool{
	"fi": true, "done": true, "esac": true, "}": true, "[": true, "[[": true, "]]": true, "cd": true,
	"echo": true, "printf": true, "read": true, "test": true, "true": true, "false": true,
	"export": true, "local": true, "readonly": true, "declare": true, "typeset": true, "set": true,
	"unset": true, "shift": true, "source": true, ".": true, ":": true, "eval": true, "exit": true,
	"return": true, "break": true, "continue": true, "trap": true, "wait": true, "kill": true,
	"pushd": true, "popd": true, "umask": true, "alias": true, "unalias": true, "type": true,
	"command": true, "builtin": true, "let": true, "getopts": true, "hash": true, "shopt": true,
	"setopt": true, "print": true, "whence": true, "ulimit": true, "jobs": true, "fg": true, "bg": true,
	"disown": true, "emulate": true, "autoload": true, "zmodload": true,
}

// shellNoCommand are keywords after which the rest of the segment names no
// program at all (a loop variable, a case subject, a function name).
var shellNoCommand = map[string]bool{"for": true, "case": true, "select": true, "function": true, "in": true}

var (
	// heredocRe finds a here-document and the word that ends it.
	// A here-string (<<<) has no end word.
	heredocRe = regexp.MustCompile(`(?:^|[^<])<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)
	// caseStartRe and caseEndRe bracket a case statement, whose pattern lines
	// ("start)", "*)") name no command.
	caseStartRe = regexp.MustCompile(`(?:^|[\s;])case\s.*\sin(?:\s|$)`)
	caseEndRe   = regexp.MustCompile(`(?:^|[\s;])esac(?:[\s;)]|$)`)
	// casePatternRe is the pattern that opens a case branch.
	casePatternRe = regexp.MustCompile(`^\s*\(?[^()]*\)`)
	// quotedRe matches single-line quoted strings; their content is data.
	quotedRe = regexp.MustCompile(`'[^']*'|"(?:[^"\\]|\\.)*"`)
	// segmentRe splits a line where a new command can start.
	segmentRe = regexp.MustCompile("\\|\\|?|&&|;|&|\\$\\(|`|\\(")
	// funcDefRe finds the functions a script defines, which are not programs.
	funcDefRe = regexp.MustCompile(`(?m)^\s*(?:function\s+([A-Za-z_][A-Za-z0-9_.-]*)|([A-Za-z_][A-Za-z0-9_.-]*)\s*\(\s*\))`)
	// commandNameRe is what a program name looks like.
	commandNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]*$`)
)

// ExtractCommands finds the programs a shell script runs by name, in order of
// first appearance. Unlike ExtractPaths it errs on the side of missing one: a
// command that is not there becomes a finding, so a word that only looks like a
// command must not turn into a false alarm.
func ExtractCommands(src string) []string {
	defined := map[string]bool{}
	for _, m := range funcDefRe.FindAllStringSubmatch(src, -1) {
		defined[m[1]+m[2]] = true
	}
	var out []string
	seen := map[string]bool{}
	heredocEnd, caseDepth := "", 0
	for i, line := range strings.Split(src, "\n") {
		if heredocEnd != "" {
			if strings.TrimSpace(line) == heredocEnd {
				heredocEnd = ""
			}
			continue
		}
		if i == 0 && strings.HasPrefix(line, "#!") {
			continue
		}
		if m := heredocRe.FindStringSubmatch(line); m != nil {
			heredocEnd = m[1]
		}
		line = quotedRe.ReplaceAllString(line, `""`)
		if c := strings.Index(line, " #"); c >= 0 {
			line = line[:c]
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if caseDepth > 0 {
			line = casePatternRe.ReplaceAllString(line, "")
		}
		if caseStartRe.MatchString(line) {
			caseDepth++
		}
		if caseEndRe.MatchString(line) && caseDepth > 0 {
			caseDepth--
		}
		for _, seg := range segmentRe.Split(line, -1) {
			name := segmentCommand(seg)
			if name == "" || defined[name] || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
			if len(out) >= maxCommands {
				return out
			}
		}
	}
	return out
}

// segmentCommand returns the program a single command segment starts, or "".
func segmentCommand(seg string) string {
	for _, w := range strings.Fields(seg) {
		w = strings.TrimRight(w, ")}")
		switch {
		case w == "":
			continue
		case shellNoCommand[w], shellBuiltin[w]:
			return ""
		case shellPrefix[w]:
			continue
		case strings.Contains(w, "=") && !strings.HasPrefix(w, "="):
			continue // FOO=bar prefixing the command
		case strings.HasPrefix(w, "-"):
			continue // a flag of a skipped prefix (sudo -n, command -v)
		case !commandNameRe.MatchString(w):
			return "" // a path, a variable, a redirect: nothing to look up by name
		default:
			return w
		}
	}
	return ""
}

// InspectCommands looks up each command on pathEnv (a PATH value) and, for the
// ones it lacks, in the usual install locations under home.
func InspectCommands(src, pathEnv, home string) []Command {
	names := ExtractCommands(src)
	dirs := filepath.SplitList(pathEnv)
	extra := []string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin", "/usr/local/sbin"}
	if home != "" {
		extra = append(extra, filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin"),
			filepath.Join(home, "go", "bin"), filepath.Join(home, ".cargo", "bin"))
	}
	out := make([]Command, 0, len(names))
	for _, n := range names {
		c := Command{Name: n, OnPath: findExecutable(n, dirs)}
		if c.OnPath == "" {
			c.Elsewhere = findExecutable(n, extra)
		}
		out = append(out, c)
	}
	return out
}

// findExecutable returns the first executable file called name in dirs.
func findExecutable(name string, dirs []string) string {
	for _, d := range dirs {
		if d == "" {
			continue
		}
		p := filepath.Join(d, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}
