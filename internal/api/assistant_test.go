package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/assistant"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// newAssistantServer wires a server whose assistant answers through ask, on a
// claude provider with an explicit binary so availability does not depend on
// what is installed here.
func newAssistantServer(t *testing.T, ask assistant.Asker) (*httptest.Server, *store.Store) {
	t.Helper()
	_, st := newFeedbackServer(t, nil)
	var svc *assistant.Service
	if ask != nil {
		svc = assistant.New(ask, &assistant.StoreSource{Store: st})
	}
	srv := httptest.NewServer(handler(Deps{Store: st, Assistant: svc}))
	t.Cleanup(srv.Close)
	return srv, st
}

func TestAssistantStatus(t *testing.T) {
	srv, _ := newAssistantServer(t, fakeAsker{})
	var st assistantStatus
	do(t, srv, http.MethodGet, "/api/assistant", nil).into(t, &st)
	if !st.Available || st.Reason != "" || st.Provider == "" {
		t.Fatalf("status = %+v, want available with the provider named", st)
	}

	srv, _ = newAssistantServer(t, nil)
	do(t, srv, http.MethodGet, "/api/assistant", nil).into(t, &st)
	if st.Available || st.Reason == "" {
		t.Fatalf("status = %+v, want unavailable with a reason", st)
	}
}

func TestAssistantStatusNamesADefaultThatCannotAnswer(t *testing.T) {
	_, st := newAssistantServer(t, nil)
	reg := provider.NewRegistry(stubAdapter{
		health: provider.Health{State: provider.HealthReady},
		caps:   provider.Capabilities{InteractiveResume: true},
	})
	srv := httptest.NewServer(handler(Deps{
		Store: st, Assistant: assistant.New(fakeAsker{}, &assistant.StoreSource{Store: st}),
		Registry: reg, Providers: provider.NewChecker(reg),
	}))
	t.Cleanup(srv.Close)

	var got assistantStatus
	do(t, srv, http.MethodGet, "/api/assistant", nil).into(t, &got)
	if got.Available || !strings.Contains(got.Reason, "the default provider") || !strings.Contains(got.Reason, "assistant provider") {
		t.Fatalf("status = %+v, want the default provider named as unable to answer", got)
	}
	r := do(t, srv, http.MethodPost, "/api/assistant/turn", map[string]string{"text": "hi"})
	if r.Status != http.StatusServiceUnavailable {
		t.Fatalf("turn status = %d, want 503", r.Status)
	}
}

func TestAssistantTurn(t *testing.T) {
	srv, _ := newAssistantServer(t, fakeAsker{answer: validated(`{"reply":"Opened.","actions":[{"type":"open_view","view":"log"},{"type":"open_run","id":"invented"}]}`)})
	var got assistant.Reply
	r := do(t, srv, http.MethodPost, "/api/assistant/turn", map[string]any{"text": "show me the log", "focus": map[string]string{"view": "queue"}})
	if r.Status != http.StatusOK {
		t.Fatalf("status = %d (%s)", r.Status, r.Body)
	}
	r.into(t, &got)
	if got.Text != "Opened." || got.SessionID == "" || len(got.Actions) != 1 || got.Actions[0].View != "log" {
		t.Fatalf("reply = %+v, want the invented run dropped", got)
	}
}

func TestAssistantTurnFailures(t *testing.T) {
	srv, _ := newAssistantServer(t, fakeAsker{err: errors.New("limit reached")})
	if r := do(t, srv, http.MethodPost, "/api/assistant/turn", map[string]string{"text": "hi"}); r.Status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", r.Status)
	}
	srv, _ = newAssistantServer(t, nil)
	if r := do(t, srv, http.MethodPost, "/api/assistant/turn", map[string]string{"text": "hi"}); r.Status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", r.Status)
	}
}
