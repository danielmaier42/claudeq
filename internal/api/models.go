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

// aliasPref are the tier aliases we know claude accepts, in the order we offer
// them. Every one is always selectable: the binary's --model help text only
// names a couple of them as examples, so it cannot be read as the full set.
var aliasPref = []string{"opus", "sonnet", "haiku", "fable"}

// fallbackModels is used when the claude binary cannot be queried at all.
var fallbackModels = orderAliases(nil)

var quotedRe = regexp.MustCompile(`'([a-zA-Z0-9-]+)'`)

// BinaryModelLister returns a cached lister for the selectable models: the
// known tier aliases, plus any further alias the claude binary's own `--help`
// advertises for --model. It falls back to the known tiers alone if the binary
// can't be run.
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

// orderAliases turns the aliases advertised by --help into the selectable list:
// every known tier first, in aliasPref order, then any alias the help mentions
// that we don't know about, in the order it appeared.
func orderAliases(aliases []string) []Model {
	in := map[string]bool{}
	for _, a := range aliases {
		in[a] = true
	}
	var out []Model
	for _, pref := range aliasPref {
		out = append(out, Model{ID: pref, Label: title(pref) + " (latest)"})
		delete(in, pref)
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
