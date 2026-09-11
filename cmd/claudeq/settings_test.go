package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/store"
)

// parsePatch runs the flag plumbing the way cmdSettings does.
func parsePatch(t *testing.T, args []string, readFile func(string) ([]byte, error)) settingsPatch {
	t.Helper()
	fs := flag.NewFlagSet("settings", flag.ContinueOnError)
	fs.SetOutput(&strings.Builder{})
	var p settingsPatch
	p.register(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := p.resolve(fs, readFile); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return p
}

func TestSettingsPatchApply(t *testing.T) {
	base := store.Settings{
		DefaultModel: "sonnet", ClaudePath: "/bin/claude",
		HeartbeatMinutes: 30, IdleTimeoutMinutes: 45, MaxRunHistory: 100,
		SystemPrompt: "old", Paused: true, PromptReviewModel: "opus",
		Pushover: store.Pushover{Enabled: true, Token: "tok", UserKey: "usr"},
	}

	tests := []struct {
		name string
		args []string
		want func(store.Settings) store.Settings
	}{
		{
			name: "no flags changes nothing",
			args: nil,
			want: func(s store.Settings) store.Settings { return s },
		},
		{
			name: "empty model clears the default",
			args: []string{"--default-model", ""},
			want: func(s store.Settings) store.Settings { s.DefaultModel = ""; return s },
		},
		{
			name: "reliability settings",
			args: []string{"--idle-timeout-minutes=-1", "--max-run-history=-1", "--heartbeat-minutes=0"},
			want: func(s store.Settings) store.Settings {
				s.IdleTimeoutMinutes, s.MaxRunHistory, s.HeartbeatMinutes = -1, -1, 0
				return s
			},
		},
		{
			name: "pause switch off",
			args: []string{"--paused=false"},
			want: func(s store.Settings) store.Settings { s.Paused = false; return s },
		},
		{
			name: "claude path and system prompt",
			args: []string{"--claude-path", "/usr/local/bin/claude", "--system-prompt", "be brief"},
			want: func(s store.Settings) store.Settings {
				s.ClaudePath, s.SystemPrompt = "/usr/local/bin/claude", "be brief"
				return s
			},
		},
		{
			name: "prompt review off",
			args: []string{"--prompt-review=false"},
			want: func(s store.Settings) store.Settings { s.PromptReviewDisabled = true; return s },
		},
		{
			name: "prompt review model",
			args: []string{"--prompt-review-model", "haiku"},
			want: func(s store.Settings) store.Settings { s.PromptReviewModel = "haiku"; return s },
		},
		{
			name: "prompt review model cleared falls back to the default model",
			args: []string{"--prompt-review-model", ""},
			want: func(s store.Settings) store.Settings { s.PromptReviewModel = ""; return s },
		},
		{
			name: "pushover toggle keeps the credentials",
			args: []string{"--pushover=false"},
			want: func(s store.Settings) store.Settings { s.Pushover.Enabled = false; return s },
		},
		{
			name: "pushover credentials",
			args: []string{"--pushover-token", "t2", "--pushover-user", "u2"},
			want: func(s store.Settings) store.Settings {
				s.Pushover.Token, s.Pushover.UserKey = "t2", "u2"
				return s
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := parsePatch(t, tc.args, failingRead)
			got, err := p.apply(base)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if want := tc.want(base); got != want {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestSettingsPatchSystemPromptFile(t *testing.T) {
	p := parsePatch(t, []string{"--system-prompt-file", "guide.md"}, func(path string) ([]byte, error) {
		if path != "guide.md" {
			t.Fatalf("read path = %q", path)
		}
		return []byte("house style\nrules"), nil
	})
	got, err := p.apply(store.Settings{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got.SystemPrompt != "house style\nrules" {
		t.Errorf("system prompt = %q", got.SystemPrompt)
	}
}

func TestSettingsPatchRejects(t *testing.T) {
	fs := flag.NewFlagSet("settings", flag.ContinueOnError)
	fs.SetOutput(&strings.Builder{})
	var p settingsPatch
	p.register(fs)
	if err := fs.Parse([]string{"--system-prompt", "a", "--system-prompt-file", "b"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := p.resolve(fs, failingRead); err == nil {
		t.Error("expected an error when both system-prompt flags are given")
	}

	neg := parsePatch(t, []string{"--heartbeat-minutes=-5"}, failingRead)
	if _, err := neg.apply(store.Settings{}); err == nil {
		t.Error("expected an error for a negative heartbeat")
	}
}

func TestCmdSettingsPersists(t *testing.T) {
	st := newTestStore(t)
	if err := cmdSettings(st, []string{"--default-model", "opus", "--max-run-history=50"}); err != nil {
		t.Fatalf("cmdSettings: %v", err)
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Settings.DefaultModel != "opus" || cfg.Settings.MaxRunHistory != 50 {
		t.Fatalf("settings not persisted: %+v", cfg.Settings)
	}
	// A read-only call must not disturb what is stored.
	if err := cmdSettings(st, nil); err != nil {
		t.Fatalf("cmdSettings read: %v", err)
	}
	if again, _ := st.LoadConfig(); again.Settings != cfg.Settings {
		t.Errorf("read-only call changed settings: %+v", again.Settings)
	}
}

func TestSettingsViewMasksCredentials(t *testing.T) {
	v := newSettingsView(store.Settings{
		Pushover: store.Pushover{Enabled: true, Token: "secret-token", UserKey: "secret-user"},
	})
	if !v.PushoverConfigured || !v.PushoverEnabled {
		t.Fatalf("view = %+v", v)
	}
	if strings.Contains(pushoverLabel(v), "secret") {
		t.Error("the label leaks the credentials")
	}
}

func TestSettingsPatchTurnsPauseOn(t *testing.T) {
	p := parsePatch(t, []string{"--paused=true"}, nil)
	got, err := p.apply(store.Settings{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !got.Paused {
		t.Fatal("--paused=true did not set the switch")
	}
}

func TestPausedLabel(t *testing.T) {
	if got := pausedLabel(true); !strings.Contains(got, "yes") {
		t.Errorf("pausedLabel(true) = %q", got)
	}
	if got := pausedLabel(false); got != "no" {
		t.Errorf("pausedLabel(false) = %q", got)
	}
}

func TestNumericLabel(t *testing.T) {
	tests := []struct {
		v, def    int
		neg, want string
	}{
		{0, 30, "never", "30 (default)"},
		{-1, 30, "never", "-1 (never)"},
		{45, 30, "never", "45"},
		{0, 60, "", "60 (default)"},
	}
	for _, tc := range tests {
		if got := numericLabel(tc.v, tc.def, tc.neg); got != tc.want {
			t.Errorf("numericLabel(%d, %d, %q) = %q, want %q", tc.v, tc.def, tc.neg, got, tc.want)
		}
	}
}

func TestSettingsPatchAppliesTheNewChannels(t *testing.T) {
	p := parsePatch(t, []string{
		"--ntfy=true", "--ntfy-server", "https://ntfy.home.lan", "--ntfy-topic", "claudeq",
		"--ntfy-token", "tk_1", "--webhook=true", "--webhook-url", "https://hooks.example.com/a",
		"--webhook-template", `{"text":"{{title}}"}`,
	}, nil)
	got, err := p.apply(store.Settings{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := store.Settings{
		Ntfy:    store.Ntfy{Enabled: true, Server: "https://ntfy.home.lan", Topic: "claudeq", Token: "tk_1"},
		Webhook: store.Webhook{Enabled: true, URL: "https://hooks.example.com/a", Template: `{"text":"{{title}}"}`},
	}
	if got != want {
		t.Fatalf("apply = %+v, want %+v", got, want)
	}
	// An unmentioned channel keeps what it had, like every other setting.
	kept, err := parsePatch(t, []string{"--default-model", "opus"}, nil).apply(want)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if kept.Ntfy != want.Ntfy || kept.Webhook != want.Webhook {
		t.Fatalf("unmentioned channels were changed: %+v", kept)
	}
}

// Switching a channel on from the CLI has to fail loudly when it cannot
// deliver: the alternative is a queue that looks fine and notifies nobody.
func TestCmdSettingsRefusesAChannelThatCannotDeliver(t *testing.T) {
	st := newTestStore(t)
	if err := cmdSettings(st, []string{"--ntfy=true"}); err == nil {
		t.Fatal("expected --ntfy without a topic to be refused")
	}
	if cfg, _ := st.LoadConfig(); cfg.Settings.Ntfy.Enabled {
		t.Fatal("the refused channel was switched on anyway")
	}
	if err := cmdSettings(st, []string{"--webhook=true", "--webhook-url", "example.com/hook"}); err == nil {
		t.Fatal("expected a non-http webhook URL to be refused")
	}
	if err := cmdSettings(st, []string{"--ntfy=true", "--ntfy-topic", "claudeq"}); err != nil {
		t.Fatalf("a topic on the default server is enough: %v", err)
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Settings.Ntfy.Enabled || cfg.Settings.Ntfy.Topic != "claudeq" {
		t.Fatalf("ntfy not persisted: %+v", cfg.Settings.Ntfy)
	}
}

func TestSettingsViewMasksTheNewChannelSecrets(t *testing.T) {
	v := newSettingsView(store.Settings{
		Ntfy:    store.Ntfy{Enabled: true, Topic: "claudeq", Token: "tk_secret"},
		Webhook: store.Webhook{Enabled: true, URL: "https://hooks.example.com/secret-path"},
	})
	if !v.NtfyTokenSet || !v.WebhookConfigured {
		t.Fatalf("view = %+v", v)
	}
	// A Slack or Discord webhook URL is itself the credential, and an ntfy token
	// always is: `claudeq settings` must not put either into scrollback.
	for _, line := range []string{ntfyLabel(v), webhookLabel(v)} {
		if strings.Contains(line, "tk_secret") || strings.Contains(line, "secret-path") {
			t.Errorf("label leaks a secret: %q", line)
		}
	}
	if !strings.Contains(ntfyLabel(v), "token set") {
		t.Errorf("ntfy label should say a token is configured, got %q", ntfyLabel(v))
	}
}
