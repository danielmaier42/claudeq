package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// newProviderServer wires a server whose one adapter reports health.
func newProviderServer(t *testing.T, health provider.Health) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	reg := provider.NewRegistry(stubAdapter{health: health})
	srv := httptest.NewServer(Handler(Deps{
		Store: st, Registry: reg, Providers: provider.NewChecker(reg),
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

func TestListProvidersOnAFreshInstall(t *testing.T) {
	srv, _ := newProviderServer(t, provider.Health{
		State: provider.HealthNotInstalled, Reason: "Claude Code was not found.",
	})
	var got []providerView
	do(t, srv, http.MethodGet, "/api/providers", nil).into(t, &got)

	if len(got) != 1 {
		t.Fatalf("got %d providers, want the seeded claude instance", len(got))
	}
	p := got[0]
	if p.ID != provider.DefaultInstanceID || !p.Default {
		t.Fatalf("provider = %+v, want the default claude instance", p)
	}
	// A fresh installation must not imply Claude Code is there just because it
	// is the default provider.
	if p.Health.State != provider.HealthNotInstalled || p.Health.Reason == "" {
		t.Fatalf("health = %+v, want the real state with a reason", p.Health)
	}
}

func TestProviderEndpointsEditAndCheck(t *testing.T) {
	srv, st := newProviderServer(t, provider.Health{State: provider.HealthReady})

	body := map[string]any{
		"name": "Claude Code", "binary_path": "/opt/claude",
		"config_dir": "/tmp/cfg", "default_model": "sonnet", "enabled": true,
	}
	var updated providerView
	r := do(t, srv, http.MethodPut, "/api/providers/claude", body)
	if r.Status != http.StatusOK {
		t.Fatalf("update = %d (%s)", r.Status, r.Body)
	}
	r.into(t, &updated)
	if updated.BinaryPath != "/opt/claude" || updated.DefaultModel != "sonnet" || updated.ConfigDir != "/tmp/cfg" {
		t.Fatalf("provider = %+v, want the edit applied", updated.Instance)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Providers[0].BinaryPath != "/opt/claude" {
		t.Fatalf("stored = %+v, want the edit persisted", cfg.Providers[0])
	}

	var checked providerView
	r = do(t, srv, http.MethodPost, "/api/providers/claude/check", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("check = %d (%s)", r.Status, r.Body)
	}
	r.into(t, &checked)
	if !checked.Health.Ready() || checked.Health.CheckedAt.IsZero() {
		t.Fatalf("health = %+v, want a fresh ready verdict", checked.Health)
	}
}

func TestProviderEndpointsRejectBadChanges(t *testing.T) {
	srv, _ := newProviderServer(t, provider.Health{State: provider.HealthReady})

	tests := []struct {
		name   string
		method string
		path   string
		body   any
		want   int
	}{
		{
			name: "a relative binary path", method: http.MethodPut, path: "/api/providers/claude",
			body: map[string]any{"binary_path": "bin/claude"}, want: http.StatusBadRequest,
		},
		{
			name: "changing the kind", method: http.MethodPut, path: "/api/providers/claude",
			body: map[string]any{"kind": "codex"}, want: http.StatusBadRequest,
		},
		{
			name: "editing an id nothing is configured under", method: http.MethodPut, path: "/api/providers/nope",
			body: map[string]any{"name": "x"}, want: http.StatusNotFound,
		},
		{
			name: "checking an unknown id", method: http.MethodPost, path: "/api/providers/nope/check",
			want: http.StatusNotFound,
		},
		{
			name: "adding a kind no adapter implements", method: http.MethodPost, path: "/api/providers",
			body: map[string]any{"id": "oc", "kind": "opencode"}, want: http.StatusBadRequest,
		},
		{
			name: "removing the only provider", method: http.MethodDelete, path: "/api/providers/claude",
			want: http.StatusBadRequest,
		},
		{
			name: "making an unknown provider the default", method: http.MethodPost, path: "/api/providers/nope/default",
			want: http.StatusNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if r := do(t, srv, tc.method, tc.path, tc.body); r.Status != tc.want {
				t.Fatalf("status = %d (%s), want %d", r.Status, r.Body, tc.want)
			}
		})
	}
}

func TestAddAndDefaultProvider(t *testing.T) {
	srv, st := newProviderServer(t, provider.Health{State: provider.HealthReady})

	body := map[string]any{"id": "second", "kind": string(provider.KindClaudeCode), "name": "Second account"}
	if r := do(t, srv, http.MethodPost, "/api/providers", body); r.Status != http.StatusCreated {
		t.Fatalf("add = %d (%s)", r.Status, r.Body)
	}
	if r := do(t, srv, http.MethodPost, "/api/providers/second/default", nil); r.Status != http.StatusNoContent {
		t.Fatalf("default = %d (%s)", r.Status, r.Body)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Settings.DefaultProvider != "second" {
		t.Fatalf("default provider = %q, want the new one", cfg.Settings.DefaultProvider)
	}

	// Saving the settings form must not take the choice back with it.
	if r := do(t, srv, http.MethodPut, "/api/settings", store.Settings{HeartbeatMinutes: 30}); r.Status != http.StatusOK {
		t.Fatalf("put settings = %d (%s)", r.Status, r.Body)
	}
	cfg, _ = st.LoadConfig()
	if cfg.Settings.DefaultProvider != "second" {
		t.Fatalf("default provider = %q, want it untouched by a settings save", cfg.Settings.DefaultProvider)
	}

	if r := do(t, srv, http.MethodPost, "/api/providers/second/disable", nil); r.Status != http.StatusNoContent {
		t.Fatalf("disable = %d (%s)", r.Status, r.Body)
	}
	cfg, _ = st.LoadConfig()
	if cfg.Providers[1].Enabled {
		t.Fatalf("provider = %+v, want it switched off", cfg.Providers[1])
	}
}

// TestQueueSaysWhyATaskIsBlocked is the queue's half of the readiness rule: the
// task keeps its place and the row says what is wrong, instead of looking like
// a job that simply never runs.
func TestQueueSaysWhyATaskIsBlocked(t *testing.T) {
	srv, st := newProviderServer(t, provider.Health{
		State: provider.HealthNotAuthenticated, Reason: "Claude Code is not logged in.",
	})
	if err := st.SaveConfig(store.Config{Tasks: []task.Task{sampleTask("a")}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	var got []taskView
	do(t, srv, http.MethodGet, "/api/tasks", nil).into(t, &got)
	if len(got) != 1 {
		t.Fatalf("got %d tasks, want the blocked one still queued", len(got))
	}
	if got[0].BlockedReason != "Claude Code is not logged in." {
		t.Fatalf("blocked reason = %q, want the readiness reason", got[0].BlockedReason)
	}
}

func TestQueueSaysNothingWhenTheProviderIsReady(t *testing.T) {
	srv, st := newProviderServer(t, provider.Health{State: provider.HealthReady})
	if err := st.SaveConfig(store.Config{Tasks: []task.Task{sampleTask("a")}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	var got []taskView
	do(t, srv, http.MethodGet, "/api/tasks", nil).into(t, &got)
	if len(got) != 1 || got[0].BlockedReason != "" {
		t.Fatalf("tasks = %+v, want no blocked marker", got)
	}
}

// TestAddTaskRefusesAnUnreadyProvider keeps a job that is known in advance to
// fail from being filed at all.
func TestAddTaskRefusesAnUnreadyProvider(t *testing.T) {
	srv, st := newProviderServer(t, provider.Health{
		State: provider.HealthNotInstalled, Reason: "Claude Code was not found.",
	})
	r := do(t, srv, http.MethodPost, "/api/tasks", sampleTask("a"))
	if r.Status != http.StatusBadRequest {
		t.Fatalf("add = %d (%s), want it refused", r.Status, r.Body)
	}
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Fatalf("a refused task must not be stored, got %+v", cfg.Tasks)
	}
}
