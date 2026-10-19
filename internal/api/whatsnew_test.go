package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/review"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func TestWhatsNewShowsUntilDismissed(t *testing.T) {
	srv, _ := newReviewServer(t, nil, store.Settings{}, "", "")

	var got whatsNewStatus
	do(t, srv, "GET", "/api/whats-new", nil).into(t, &got)
	if !got.Show || got.Edition != WhatsNewEdition {
		t.Fatalf("before dismissing: %+v", got)
	}
	if r := do(t, srv, "POST", "/api/whats-new/dismiss", nil); r.Status != http.StatusOK {
		t.Fatalf("dismiss: %d %s", r.Status, r.Body)
	}
	do(t, srv, "GET", "/api/whats-new", nil).into(t, &got)
	if got.Show {
		t.Fatalf("after dismissing: %+v", got)
	}
}

// promptReviewer answers per prompt, so one check can find some tasks and
// pass others.
type promptReviewer struct {
	mu      sync.Mutex
	answers map[string]review.Result
	seen    []review.Request
}

func (p *promptReviewer) Review(_ context.Context, req review.Request) (review.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = append(p.seen, req)
	if res, ok := p.answers[req.Prompt]; ok {
		return res, nil
	}
	return review.Result{OK: true}, nil
}

func TestRenameCheckReviewsOnlyTasksNamingClaudeQAndAppliesTheSelection(t *testing.T) {
	rv := &promptReviewer{answers: map[string]review.Result{
		"run /Applications/ClaudeQ.app/Contents/MacOS/claudeq status": {
			Message: "Calls the old CLI.", RevisedPrompt: "run /Applications/DispatchQ.app/Contents/MacOS/dispatchq status",
		},
		"echo $CLAUDEQ_BIN": {Message: "Old variable.", RevisedPrompt: "echo $DISPATCHQ_BIN"},
		"see ClaudeQ docs":  {Message: "Only you know the right docs."},
	}}
	srv, st := newReviewServer(t, rv, store.Settings{}, "/opt/claude", "")
	mk := func(id, prompt string) task.Task {
		tk := sampleTask(id)
		tk.Prompt = prompt
		return tk
	}
	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Tasks = []task.Task{
			mk("cli", "run /Applications/ClaudeQ.app/Contents/MacOS/claudeq status"),
			mk("env", "echo $CLAUDEQ_BIN"),
			mk("docs", "see ClaudeQ docs"),
			mk("plain", "write a poem"),
			mk("fine", "claudeq repo is fine"),
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if !renamePending(t, srv) {
		t.Fatal("tasks naming ClaudeQ that were never checked must ask for the check")
	}
	var c renameCheck
	do(t, srv, "POST", "/api/rename-check", nil).into(t, &c)
	if c.State != "running" || c.Total != 4 || c.Skipped != 1 {
		t.Fatalf("started check: %+v", c)
	}
	deadline := time.Now().Add(5 * time.Second)
	for c.State != "done" {
		if time.Now().After(deadline) {
			t.Fatalf("check did not finish: %+v", c)
		}
		time.Sleep(5 * time.Millisecond)
		do(t, srv, "GET", "/api/rename-check", nil).into(t, &c)
	}
	if len(c.Findings) != 3 || c.Checked != 4 {
		t.Fatalf("findings: %+v", c)
	}
	rv.mu.Lock()
	seen := rv.seen
	rv.mu.Unlock()
	for _, req := range seen {
		if req.Kind != review.KindRename {
			t.Errorf("review kind %q, want rename", req.Kind)
		}
		if req.Prompt == "write a poem" {
			t.Error("a task that never names ClaudeQ must not be reviewed")
		}
	}

	// The operator deselected "env". "docs" has no revision to apply.
	var res renameApplyResponse
	do(t, srv, "POST", "/api/rename-check/apply", renameApplyRequest{TaskIDs: []string{"cli", "docs"}}).into(t, &res)
	if len(res.Applied) != 1 || res.Applied[0] != "cli" {
		t.Fatalf("applied: %+v", res)
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	prompts := map[string]string{}
	for _, tk := range cfg.Tasks {
		prompts[tk.ID] = tk.Prompt
	}
	if prompts["cli"] != "run /Applications/DispatchQ.app/Contents/MacOS/dispatchq status" {
		t.Errorf("cli prompt = %q", prompts["cli"])
	}
	if prompts["env"] != "echo $CLAUDEQ_BIN" || prompts["docs"] != "see ClaudeQ docs" {
		t.Errorf("unselected tasks changed: %+v", prompts)
	}
	do(t, srv, "GET", "/api/rename-check", nil).into(t, &c)
	if len(c.Findings) != 2 {
		t.Errorf("an applied finding must leave the list, got %+v", c.Findings)
	}
	if !renamePending(t, srv) {
		t.Error("a deselected fix is still waiting, so the banner must stay")
	}
}

func renamePending(t *testing.T, srv *httptest.Server) bool {
	t.Helper()
	var got whatsNewStatus
	do(t, srv, "GET", "/api/whats-new", nil).into(t, &got)
	return got.RenamePending
}

func TestRenameCheckIsNotPendingWithoutClaudeQNames(t *testing.T) {
	srv, st := newReviewServer(t, nil, store.Settings{}, "/opt/claude", "")
	tk := sampleTask("plain")
	tk.Prompt = "write a poem"
	if err := st.UpdateConfig(func(cfg *store.Config) error { cfg.Tasks = []task.Task{tk}; return nil }); err != nil {
		t.Fatal(err)
	}
	if renamePending(t, srv) {
		t.Fatal("nothing names ClaudeQ, so there is nothing to check")
	}
}

func TestRenameCheckDismissEndsThePrompt(t *testing.T) {
	srv, _ := newReviewServer(t, nil, store.Settings{SystemPrompt: "use claudeq"}, "/opt/claude", "")
	if !renamePending(t, srv) {
		t.Fatal("a system prompt naming ClaudeQ must ask for the check")
	}
	if r := do(t, srv, "POST", "/api/rename-check/dismiss", nil); r.Status != http.StatusOK {
		t.Fatalf("dismiss: %d %s", r.Status, r.Body)
	}
	if renamePending(t, srv) {
		t.Fatal("a dismissed check must not ask again")
	}
}

func TestRenameCheckWithNothingToFixSettles(t *testing.T) {
	srv, st := newReviewServer(t, &promptReviewer{}, store.Settings{}, "/opt/claude", "")
	tk := sampleTask("fine")
	tk.Prompt = "the claudeq repo is fine"
	if err := st.UpdateConfig(func(cfg *store.Config) error { cfg.Tasks = []task.Task{tk}; return nil }); err != nil {
		t.Fatal(err)
	}
	var c renameCheck
	do(t, srv, "POST", "/api/rename-check", nil).into(t, &c)
	deadline := time.Now().Add(5 * time.Second)
	for renamePending(t, srv) {
		if time.Now().After(deadline) {
			t.Fatal("a check that finds nothing to fix must end the prompt")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRenameApplyLeavesATaskEditedSinceTheCheck(t *testing.T) {
	rv := &promptReviewer{answers: map[string]review.Result{
		"echo $CLAUDEQ_BIN": {Message: "Old variable.", RevisedPrompt: "echo $DISPATCHQ_BIN"},
	}}
	srv, st := newReviewServer(t, rv, store.Settings{}, "/opt/claude", "")
	tk := sampleTask("env")
	tk.Prompt = "echo $CLAUDEQ_BIN"
	if err := st.UpdateConfig(func(cfg *store.Config) error { cfg.Tasks = []task.Task{tk}; return nil }); err != nil {
		t.Fatal(err)
	}
	var c renameCheck
	do(t, srv, "POST", "/api/rename-check", nil).into(t, &c)
	deadline := time.Now().Add(5 * time.Second)
	for c.State != "done" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		do(t, srv, "GET", "/api/rename-check", nil).into(t, &c)
	}
	if err := st.UpdateConfig(func(cfg *store.Config) error { cfg.Tasks[0].Prompt = "hand edited"; return nil }); err != nil {
		t.Fatal(err)
	}
	var res renameApplyResponse
	do(t, srv, "POST", "/api/rename-check/apply", renameApplyRequest{TaskIDs: []string{"env"}}).into(t, &res)
	if len(res.Applied) != 0 || len(res.Changed) != 1 {
		t.Fatalf("got %+v", res)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Tasks[0].Prompt != "hand edited" {
		t.Errorf("the hand edit was overwritten: %q", cfg.Tasks[0].Prompt)
	}
}

func TestRenameCheckWithoutAReviewer(t *testing.T) {
	srv, _ := newReviewServer(t, nil, store.Settings{}, "/opt/claude", "")
	var c renameCheck
	do(t, srv, "POST", "/api/rename-check", nil).into(t, &c)
	if c.State != "unavailable" {
		t.Fatalf("got %+v", c)
	}
}

func TestRenameCheckCoversTheCustomSystemPrompt(t *testing.T) {
	old := "Queue follow-ups with /Applications/ClaudeQ.app/Contents/MacOS/claudeq queue."
	rv := &promptReviewer{answers: map[string]review.Result{
		old: {Message: "Old CLI.", RevisedPrompt: "Queue follow-ups with /Applications/DispatchQ.app/Contents/MacOS/dispatchq queue."},
	}}
	srv, st := newReviewServer(t, rv, store.Settings{SystemPrompt: old}, "/opt/claude", "")
	var c renameCheck
	do(t, srv, "POST", "/api/rename-check", nil).into(t, &c)
	deadline := time.Now().Add(5 * time.Second)
	for c.State != "done" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		do(t, srv, "GET", "/api/rename-check", nil).into(t, &c)
	}
	if len(c.Findings) != 1 || c.Findings[0].TaskID != renameSystemID {
		t.Fatalf("findings: %+v", c.Findings)
	}
	var res renameApplyResponse
	do(t, srv, "POST", "/api/rename-check/apply", renameApplyRequest{TaskIDs: []string{renameSystemID}}).into(t, &res)
	cfg, _ := st.LoadConfig()
	if len(res.Applied) != 1 || !strings.Contains(cfg.Settings.SystemPrompt, "DispatchQ.app") {
		t.Fatalf("applied %+v, system prompt %q", res, cfg.Settings.SystemPrompt)
	}
	if renamePending(t, srv) {
		t.Error("every fix is applied, so the check is done")
	}
}
