// Reading and changing the global settings from the command line — the same
// values the app's Settings view edits.
package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/danielmaier42/claudeq/internal/store"
)

func cmdSettings(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("settings", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the settings as JSON (credentials masked)")
	var p settingsPatch
	p.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := p.resolve(fs, readPromptFile); err != nil {
		return err
	}

	var settings store.Settings
	if len(p.set) == 0 {
		cfg, err := st.LoadConfig()
		if err != nil {
			return err
		}
		settings = cfg.Settings
	} else if err := st.UpdateConfig(func(cfg *store.Config) error {
		s, err := p.apply(cfg.Settings)
		if err != nil {
			return err
		}
		cfg.Settings = s
		settings = s
		return nil
	}); err != nil {
		return err
	}

	if *asJSON {
		return printJSON(newSettingsView(settings))
	}
	printSettings(settings)
	return nil
}

// settingsPatch is a parsed set of `claudeq settings` flags: the values plus
// which of them the caller passed, so unmentioned settings keep their value.
type settingsPatch struct {
	set              map[string]bool
	model            string
	claudePath       string
	heartbeat        int
	idleTimeout      int
	maxRunHistory    int
	systemPrompt     string
	systemPromptFile string
	pushoverOn       bool
	pushoverToken    string
	pushoverUser     string
}

func (p *settingsPatch) register(fs *flag.FlagSet) {
	fs.StringVar(&p.model, "default-model", "", "global default model (empty = Claude's own default)")
	fs.StringVar(&p.claudePath, "claude-path", "", "absolute path to the claude binary (empty = auto-detect)")
	fs.IntVar(&p.heartbeat, "heartbeat-minutes", 0, "how often to look for due tasks (0 = default)")
	fs.IntVar(&p.idleTimeout, "idle-timeout-minutes", 0, "kill a run with no output for this long (0 = default, negative = never)")
	fs.IntVar(&p.maxRunHistory, "max-run-history", 0, "runs to keep before pruning (0 = default, negative = keep all)")
	fs.StringVar(&p.systemPrompt, "system-prompt", "", "custom system prompt appended to every run")
	fs.StringVar(&p.systemPromptFile, "system-prompt-file", "", "read the custom system prompt from a file ('-' = stdin)")
	fs.BoolVar(&p.pushoverOn, "pushover", false, "send notifications to Pushover")
	fs.StringVar(&p.pushoverToken, "pushover-token", "", "Pushover API token")
	fs.StringVar(&p.pushoverUser, "pushover-user", "", "Pushover user key")
}

// resolve records which flags were passed and folds --system-prompt-file into
// --system-prompt. readFile is injected so this stays testable.
func (p *settingsPatch) resolve(fs *flag.FlagSet, readFile func(string) ([]byte, error)) error {
	var err error
	if p.set, err = passedFlags(fs); err != nil {
		return err
	}
	if p.set["system-prompt"] && p.set["system-prompt-file"] {
		return fmt.Errorf("choose either --system-prompt or --system-prompt-file")
	}
	if p.set["system-prompt-file"] {
		data, err := readFile(p.systemPromptFile)
		if err != nil {
			return fmt.Errorf("read system prompt file: %w", err)
		}
		p.systemPrompt = string(data)
		p.set["system-prompt"] = true
	}
	return nil
}

func (p settingsPatch) apply(s store.Settings) (store.Settings, error) {
	if p.set["default-model"] {
		s.DefaultModel = p.model
	}
	if p.set["claude-path"] {
		s.ClaudePath = p.claudePath
	}
	if p.set["heartbeat-minutes"] {
		if p.heartbeat < 0 {
			return store.Settings{}, fmt.Errorf("--heartbeat-minutes must not be negative (0 = default)")
		}
		s.HeartbeatMinutes = p.heartbeat
	}
	if p.set["idle-timeout-minutes"] {
		s.IdleTimeoutMinutes = p.idleTimeout
	}
	if p.set["max-run-history"] {
		s.MaxRunHistory = p.maxRunHistory
	}
	if p.set["system-prompt"] {
		s.SystemPrompt = p.systemPrompt
	}
	if p.set["pushover"] {
		s.Pushover.Enabled = p.pushoverOn
	}
	if p.set["pushover-token"] {
		s.Pushover.Token = p.pushoverToken
	}
	if p.set["pushover-user"] {
		s.Pushover.UserKey = p.pushoverUser
	}
	return s, nil
}

// settingsView is what the CLI reports: every setting, but with the Pushover
// credentials reduced to whether they are set — config.toml holds them in clear
// text and printing them would leak them into terminal scrollback and logs.
type settingsView struct {
	DefaultModel       string `json:"default_model"`
	ClaudePath         string `json:"claude_path"`
	HeartbeatMinutes   int    `json:"heartbeat_minutes"`
	IdleTimeoutMinutes int    `json:"idle_timeout_minutes"`
	MaxRunHistory      int    `json:"max_run_history"`
	SystemPrompt       string `json:"system_prompt"`
	PushoverEnabled    bool   `json:"pushover_enabled"`
	PushoverConfigured bool   `json:"pushover_configured"`
}

func newSettingsView(s store.Settings) settingsView {
	return settingsView{
		DefaultModel:       s.DefaultModel,
		ClaudePath:         s.ClaudePath,
		HeartbeatMinutes:   s.HeartbeatMinutes,
		IdleTimeoutMinutes: s.IdleTimeoutMinutes,
		MaxRunHistory:      s.MaxRunHistory,
		SystemPrompt:       s.SystemPrompt,
		PushoverEnabled:    s.Pushover.Enabled,
		PushoverConfigured: s.Pushover.Token != "" && s.Pushover.UserKey != "",
	}
}

func printSettings(s store.Settings) {
	v := newSettingsView(s)
	fmt.Printf("default_model:             %s\n", orDefault(v.DefaultModel, "(Claude's own default)"))
	fmt.Printf("claude_path:               %s\n", orDefault(v.ClaudePath, "(auto-detect)"))
	fmt.Printf("heartbeat_minutes:         %s\n", numericLabel(v.HeartbeatMinutes, store.DefaultHeartbeatMinutes, ""))
	fmt.Printf("idle_timeout_minutes:      %s\n", numericLabel(v.IdleTimeoutMinutes, store.DefaultIdleTimeoutMinutes, "never kill a run"))
	fmt.Printf("max_run_history:           %s\n", numericLabel(v.MaxRunHistory, store.DefaultMaxRunHistory, "keep every run"))
	fmt.Printf("pushover:                  %s\n", pushoverLabel(v))
	if strings.TrimSpace(v.SystemPrompt) == "" {
		fmt.Printf("system_prompt:             (none)\n")
		return
	}
	fmt.Printf("\nsystem_prompt:\n%s\n", v.SystemPrompt)
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// numericLabel renders a numeric setting with its zero and negative meanings
// spelled out, so `claudeq settings` never shows a bare 0 the reader has to look
// up. negative is empty when the setting has no negative meaning.
func numericLabel(v, def int, negative string) string {
	switch {
	case v == 0:
		return fmt.Sprintf("%d (default)", def)
	case v < 0 && negative != "":
		return fmt.Sprintf("%d (%s)", v, negative)
	default:
		return fmt.Sprintf("%d", v)
	}
}

func pushoverLabel(v settingsView) string {
	state := "off"
	if v.PushoverEnabled {
		state = "on"
	}
	creds := "no credentials"
	if v.PushoverConfigured {
		creds = "credentials set"
	}
	return state + ", " + creds
}
