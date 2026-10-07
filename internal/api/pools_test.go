package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// newPoolServer serves a store with a second Claude account, ready to be
// pooled with the default one.
func newPoolServer(t *testing.T, h provider.Health) (*httptest.Server, *store.Store) {
	t.Helper()
	srv, st := newProviderServer(t, h)
	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Providers = append(cfg.Providers, store.Provider{ID: "team", Kind: string(provider.KindClaudeCode), Name: "Team", Enabled: true})
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	return srv, st
}

func TestPoolEndpoints(t *testing.T) {
	srv, _ := newPoolServer(t, provider.Health{State: provider.HealthReady})

	body := map[string]any{"id": "claude-pool", "name": "Claude", "members": []map[string]any{
		{"provider": provider.DefaultInstanceID}, {"provider": "team", "weight": 5},
	}}
	var created poolView
	if r := do(t, srv, http.MethodPost, "/api/pools", body); r.Status != http.StatusCreated {
		t.Fatalf("POST status = %d: %s", r.Status, r.Body)
	} else {
		r.into(t, &created)
	}
	if created.ID != "claude-pool" || len(created.Members) != 2 || len(created.Ranking) != 2 {
		t.Fatalf("created = %+v", created)
	}

	// A task can be put on it, and the queue names nothing blocking it.
	tk := map[string]any{"id": "a", "name": "a", "prompt": "p", "working_dir": "/tmp", "trigger": "asap",
		"enabled": true, "pool": "claude-pool"}
	if r := do(t, srv, http.MethodPost, "/api/tasks", tk); r.Status != http.StatusCreated && r.Status != http.StatusOK {
		t.Fatalf("add task status = %d: %s", r.Status, r.Body)
	}
	var tasks []taskView
	do(t, srv, http.MethodGet, "/api/tasks", nil).into(t, &tasks)
	if len(tasks) != 1 || tasks[0].Pool != "claude-pool" || tasks[0].BlockedReason != "" {
		t.Fatalf("tasks = %+v", tasks)
	}

	// Edits replace name and members; a member of another type is refused.
	if r := do(t, srv, http.MethodPut, "/api/pools/claude-pool", map[string]any{"name": "Both",
		"members": []map[string]any{{"provider": "team"}}}); r.Status != http.StatusOK {
		t.Fatalf("PUT status = %d: %s", r.Status, r.Body)
	}
	if r := do(t, srv, http.MethodPut, "/api/pools/claude-pool", map[string]any{"members": []map[string]any{{"provider": "nope"}}}); r.Status != http.StatusBadRequest {
		t.Fatalf("PUT with an unknown member = %d, want 400", r.Status)
	}
	// In use: not removable.
	if r := do(t, srv, http.MethodDelete, "/api/pools/claude-pool", nil); r.Status != http.StatusBadRequest {
		t.Fatalf("DELETE in use = %d, want 400", r.Status)
	}
	var pools []poolView
	do(t, srv, http.MethodGet, "/api/pools", nil).into(t, &pools)
	if len(pools) != 1 || pools[0].Name != "Both" || len(pools[0].Members) != 1 {
		t.Fatalf("pools = %+v", pools)
	}
}

func TestATaskOnAPoolWithNoReadyMemberIsBlocked(t *testing.T) {
	srv, st := newPoolServer(t, provider.Health{State: provider.HealthNotAuthenticated, Reason: "Not logged in."})
	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Pools = []store.Pool{{ID: "p", Name: "P", Members: []store.PoolMember{{Provider: provider.DefaultInstanceID}, {Provider: "team"}}}}
		cfg.Tasks = []task.Task{{ID: "a", Name: "a", Prompt: "p", WorkingDir: "/tmp", Trigger: task.TriggerASAP,
			Enabled: true, Permissions: task.PermissionsDefault, Pool: "p"}}
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	var tasks []taskView
	do(t, srv, http.MethodGet, "/api/tasks", nil).into(t, &tasks)
	if len(tasks) != 1 || tasks[0].BlockedReason == "" {
		t.Fatalf("tasks = %+v, want the pool task blocked", tasks)
	}
	// Adding another task on it is refused for the same reason.
	tk := map[string]any{"id": "b", "name": "b", "prompt": "p", "working_dir": "/tmp", "trigger": "asap", "enabled": true, "pool": "p"}
	if r := do(t, srv, http.MethodPost, "/api/tasks", tk); r.Status != http.StatusBadRequest {
		t.Fatalf("add on a dead pool = %d, want 400", r.Status)
	}
}

func TestGetPoolRanksWithTheDaemonsRateLimits(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Providers = append(cfg.Providers, store.Provider{ID: "team", Kind: string(provider.KindClaudeCode), Name: "Team", Enabled: true})
		cfg.Pools = []store.Pool{{ID: "p", Name: "P", Members: []store.PoolMember{{Provider: provider.DefaultInstanceID}, {Provider: "team"}}}}
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	reg := provider.NewRegistry(stubAdapter{health: provider.Health{State: provider.HealthReady}})
	srv := httptest.NewServer(Handler(Deps{
		Store: st, Registry: reg, Providers: provider.NewChecker(reg),
		BlockedProviders: func() map[string]time.Time {
			return map[string]time.Time{provider.DefaultInstanceID: time.Now().Add(time.Hour)}
		},
	}))
	t.Cleanup(srv.Close)

	var v poolView
	if r := do(t, srv, http.MethodGet, "/api/pools/p", nil); r.Status != http.StatusOK {
		t.Fatalf("GET status = %d: %s", r.Status, r.Body)
	} else {
		r.into(t, &v)
	}
	// The member waiting out its rate limit goes last, behind the unread one.
	if len(v.Ranking) != 2 || v.Ranking[0].ProviderID != "team" || v.Ranking[1].Tier != 3 ||
		v.Ranking[1].Note != "waiting for its rate limit" {
		t.Fatalf("ranking = %+v", v.Ranking)
	}
	if r := do(t, srv, http.MethodGet, "/api/pools/nope", nil); r.Status != http.StatusNotFound {
		t.Fatalf("GET unknown = %d, want 404", r.Status)
	}
}
