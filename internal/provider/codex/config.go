package codex

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

// cachedProbe holds the answer to a question that is expensive to ask and
// cannot change while the daemon runs (where the CLI is) or changes rarely
// enough that a restart is an acceptable refresh (which models it bundles).
type cachedProbe[T any] struct {
	once sync.Once
	val  T
}

func (c *cachedProbe[T]) do(ask func() T) T {
	c.once.Do(func() { c.val = ask() })
	return c.val
}

// configHome is the directory the instance's Codex configuration lives in: its
// own when it has one, otherwise the CLI's default.
func configHome(configDir string) string {
	if configDir != "" {
		return configDir
	}
	if env := os.Getenv(ConfigDirEnv); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// ownInstructions reads the `developer_instructions` an operator configured for
// this Codex instance.
//
// It exists because of how the override works: `-c developer_instructions=…`
// *replaces* the configured value instead of adding to it. claudeq delivers its
// run contract through that key, so using it blindly would silently throw the
// operator's own instructions away — which is exactly the failure the provider
// spike called out. They are read here and carried along instead.
//
// Anything that goes wrong — no file, no permission, unparseable TOML — yields
// no instructions rather than an error: a Codex configuration claudeq cannot
// read is not a reason to refuse to run, and a misread would be reported by
// Codex itself. The value is never logged.
func ownInstructions(configDir string) string {
	home := configHome(configDir)
	if home == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, "config.toml")) //nolint:gosec // the instance's own configuration directory
	if err != nil {
		return ""
	}
	var cfg struct {
		DeveloperInstructions string `toml:"developer_instructions"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.DeveloperInstructions)
}

// developerInstructions combines claudeq's run contract with whatever the
// provider's own configuration sets, claudeq's first: its subject is how a
// headless run has to behave, which the operator's standing guidance then adds
// to rather than replaces. The layering is the same one a Claude Code run gets
// from the custom system prompt.
func developerInstructions(contract, own string) string {
	contract, own = strings.TrimSpace(contract), strings.TrimSpace(own)
	switch {
	case own == "":
		return contract
	case contract == "":
		return own
	}
	return contract + configuredInstructionsIntro + own
}

// configuredInstructionsIntro frames the operator's own Codex instructions
// inside the combined value and settles conflicts, so the harness is not left
// to guess which of two voices wins.
const configuredInstructionsIntro = `

The following are additional instructions from this Codex installation's own configuration. Follow them alongside the guidance above; if they ever conflict with it, the guidance above takes precedence.

`
