package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// TestTickResolvesTheExecutionIdentity checks that what the engine hands the
// executor follows the resolution table, and that a pre-provider configuration
// still produces exactly the Claude Code run it always did.
func TestTickResolvesTheExecutionIdentity(t *testing.T) {
	tests := []struct {
		name           string
		settings       store.Settings
		taskProvider   string
		taskModel      string
		permissions    task.Permissions
		wantProviderID string
		wantBinary     string
		wantModel      string
		wantAccess     provider.AccessMode
	}{
		{
			name:           "migrated configuration runs on the claude instance",
			settings:       store.Settings{ClaudePath: "/opt/claude", DefaultModel: "sonnet"},
			permissions:    task.PermissionsDefault,
			wantProviderID: provider.DefaultInstanceID,
			wantBinary:     "/opt/claude",
			wantModel:      "sonnet",
			wantAccess:     provider.AccessProviderDefault,
		},
		{
			name:           "a task model overrides the provider default",
			settings:       store.Settings{DefaultModel: "sonnet"},
			taskModel:      "opus",
			permissions:    task.PermissionsDefault,
			wantProviderID: provider.DefaultInstanceID,
			wantModel:      "opus",
			wantAccess:     provider.AccessProviderDefault,
		},
		{
			name:           "naming the default provider explicitly changes nothing",
			settings:       store.Settings{DefaultModel: "sonnet"},
			taskProvider:   provider.DefaultInstanceID,
			permissions:    task.PermissionsDefault,
			wantProviderID: provider.DefaultInstanceID,
			wantModel:      "sonnet",
			wantAccess:     provider.AccessProviderDefault,
		},
		{
			name:           "skip permissions asks for full access",
			settings:       store.Settings{},
			permissions:    task.PermissionsSkip,
			wantProviderID: provider.DefaultInstanceID,
			wantAccess:     provider.AccessFullAccess,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
			r := &stub{}
			e, st := newTestEngine(t, r, fc)

			tk := asapTask("a", false)
			tk.Provider, tk.Model, tk.Permissions = tc.taskProvider, tc.taskModel, tc.permissions
			if err := st.SaveConfig(store.Config{Settings: tc.settings, Tasks: []task.Task{tk}}); err != nil {
				t.Fatalf("SaveConfig: %v", err)
			}

			if err := e.Tick(context.Background()); err != nil {
				t.Fatalf("Tick: %v", err)
			}
			e.WaitIdle()

			reqs := r.requests()
			if len(reqs) != 1 {
				t.Fatalf("expected 1 run, got %d", len(reqs))
			}
			got := reqs[0]
			if got.Provider.ID != tc.wantProviderID {
				t.Fatalf("provider = %q, want %q", got.Provider.ID, tc.wantProviderID)
			}
			if got.Provider.Kind != provider.KindClaudeCode {
				t.Fatalf("kind = %q, want %q", got.Provider.Kind, provider.KindClaudeCode)
			}
			if got.Provider.BinaryPath != tc.wantBinary {
				t.Fatalf("binary = %q, want %q", got.Provider.BinaryPath, tc.wantBinary)
			}
			if got.Model != tc.wantModel {
				t.Fatalf("model = %q, want %q", got.Model, tc.wantModel)
			}
			if got.AccessMode != tc.wantAccess {
				t.Fatalf("access mode = %q, want %q", got.AccessMode, tc.wantAccess)
			}
		})
	}
}

func TestTickFailsATaskNamingAnUnknownProvider(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)

	tk := asapTask("a", false)
	tk.Provider = "codex-work"
	if err := st.SaveConfig(store.Config{Tasks: []task.Task{tk}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	// No substitute provider was used: nothing ran at all.
	if got := r.requests(); len(got) != 0 {
		t.Fatalf("an unresolvable provider must not run anything, got %+v", got)
	}

	runs, err := st.Runs()
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected the attempt to be recorded once, got %d runs", len(runs))
	}
	if runs[0].Status != store.StatusFailed {
		t.Fatalf("status = %q, want failed", runs[0].Status)
	}
	if !strings.Contains(runs[0].Error, "codex-work") {
		t.Fatalf("error = %q, should name the provider the task asked for", runs[0].Error)
	}

	// A one-shot task that cannot resolve does not sit in the queue retrying
	// forever; it leaves like any other failed one-shot.
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Tasks) != 0 {
		t.Fatalf("the one-shot task should have left the queue, got %+v", cfg.Tasks)
	}
}
