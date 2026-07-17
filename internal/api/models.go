package api

import (
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// Model is a selectable Claude model (an alias that maps to the latest model,
// or a full model id).
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// fallbackModels is used when the claude binary cannot be queried. These are
// stable aliases that always resolve to the latest model of each tier.
var fallbackModels = []Model{
	{ID: "opus", Label: "Opus (latest)"},
	{ID: "sonnet", Label: "Sonnet (latest)"},
	{ID: "haiku", Label: "Haiku (latest)"},
	{ID: "fable", Label: "Fable (latest)"},
}

// aliasPref orders known aliases; unknown ones are appended alphabetically.
var aliasPref = []string{"opus", "sonnet", "haiku", "fable"}

var quotedRe = regexp.MustCompile(`'([a-zA-Z0-9-]+)'`)

// BinaryModelLister returns a cached lister that derives the model list from the
// claude binary's own `--help` output (the aliases it advertises for --model).
// It falls back to fallbackModels if the binary can't be run or advertises none.
func BinaryModelLister(bin string) func() []Model {
	var once sync.Once
	var cached []Model
	return func() []Model {
		once.Do(func() {
			if out, err := exec.Command(bin, "--help").CombinedOutput(); err == nil { //nolint:gosec // fixed binary, no user input
				cached = modelsFromHelp(string(out))
			}
			if len(cached) == 0 {
				cached = fallbackModels
			}
		})
		return cached
	}
}

// modelsFromHelp extracts the model aliases advertised in the --model help text.
func modelsFromHelp(help string) []Model {
	i := strings.Index(help, "--model <model>")
	if i < 0 {
		return nil
	}
	end := i + 500
	if end > len(help) {
		end = len(help)
	}
	window := help[i:end]

	seen := map[string]bool{}
	var aliases []string
	for _, m := range quotedRe.FindAllStringSubmatch(window, -1) {
		tok := m[1]
		// Skip full model names (e.g. claude-fable-5); we present the aliases.
		if strings.HasPrefix(tok, "claude-") || seen[tok] {
			continue
		}
		seen[tok] = true
		aliases = append(aliases, tok)
	}
	return orderAliases(aliases)
}

func orderAliases(aliases []string) []Model {
	in := map[string]bool{}
	for _, a := range aliases {
		in[a] = true
	}
	var out []Model
	for _, pref := range aliasPref {
		if in[pref] {
			out = append(out, Model{ID: pref, Label: title(pref) + " (latest)"})
			delete(in, pref)
		}
	}
	// Any advertised aliases we don't have a preference for, in input order.
	for _, a := range aliases {
		if in[a] {
			out = append(out, Model{ID: a, Label: title(a)})
			delete(in, a)
		}
	}
	return out
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
