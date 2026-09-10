package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/review"
	"github.com/danielmaier42/claudeq/internal/store"
)

// fakeReviewer records the request it was given and answers with a canned
// result, so the endpoint can be tested without a Claude binary.
type fakeReviewer struct {
	mu     sync.Mutex
	got    review.Request
	calls  int
	res    review.Result
	err    error
	block  chan struct{} // when set, the review waits here until closed or cancelled
	inFlgt chan struct{} // closed once a blocking review has started
}

func (f *fakeReviewer) Review(ctx context.Context, req review.Request) (review.Result, error) {
	f.mu.Lock()
	f.got, f.calls = req, f.calls+1
	block, started := f.block, f.inFlgt
	f.mu.Unlock()
	if block != nil {
		if started != nil {
			select {
			case <-started:
			default:
				close(started)
			}
		}
		select {
		case <-block:
		case <-ctx.Done():
			return review.Result{}, ctx.Err()
		}
	}
	return f.res, f.err
}

func (f *fakeReviewer) request() review.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got
}

func (f *fakeReviewer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newReviewServer(t *testing.T, rv PromptReviewer, s store.Settings) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := st.UpdateConfig(func(cfg *store.Config) error { cfg.Settings = s; return nil }); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, Review: rv}))
	t.Cleanup(srv.Close)
	return srv, st
}

func TestReviewPromptReturnsTheFinding(t *testing.T) {
	rv := &fakeReviewer{res: review.Result{Message: "docs/ is missing", RevisedPrompt: "better"}}
	srv, _ := newReviewServer(t, rv, store.Settings{DefaultModel: "opus", ClaudePath: "/opt/claude"})

	r := do(t, srv, "POST", "/api/review/prompt", reviewRequest{Kind: "task", Prompt: "p", WorkingDir: "/w"})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d: %s", r.Status, r.Body)
	}
	var got reviewResponse
	r.into(t, &got)
	if !got.Enabled || got.OK || got.Message != "docs/ is missing" || got.RevisedPrompt != "better" {
		t.Errorf("got %+v", got)
	}

	req := rv.request()
	if req.Kind != review.KindTask || req.Prompt != "p" || req.WorkingDir != "/w" {
		t.Errorf("reviewer got %+v", req)
	}
	if req.Model != "opus" || req.Bin != "/opt/claude" {
		t.Errorf("the review must run with the configured model and binary, got model=%q bin=%q", req.Model, req.Bin)
	}
}

func TestReviewPromptUsesTheReviewModelWhenSet(t *testing.T) {
	rv := &fakeReviewer{res: review.Result{OK: true}}
	srv, _ := newReviewServer(t, rv, store.Settings{DefaultModel: "opus", PromptReviewModel: "haiku"})

	do(t, srv, "POST", "/api/review/prompt", reviewRequest{Kind: "task", Prompt: "p"})
	if got := rv.request().Model; got != "haiku" {
		t.Errorf("model = %q, want the review model to win over the default", got)
	}
}

func TestReviewPromptForTheSystemPromptDropsTheWorkingDir(t *testing.T) {
	rv := &fakeReviewer{res: review.Result{OK: true}}
	srv, _ := newReviewServer(t, rv, store.Settings{})

	do(t, srv, "POST", "/api/review/prompt", reviewRequest{Kind: "system", Prompt: "p", WorkingDir: "/w"})
	req := rv.request()
	if req.Kind != review.KindSystem || req.WorkingDir != "" {
		t.Errorf("got %+v, want a system review with no working directory", req)
	}
}

func TestReviewPromptWhenSwitchedOff(t *testing.T) {
	rv := &fakeReviewer{res: review.Result{Message: "should never be asked"}}
	srv, _ := newReviewServer(t, rv, store.Settings{PromptReviewDisabled: true})

	r := do(t, srv, "POST", "/api/review/prompt", reviewRequest{Kind: "task", Prompt: "p"})
	var got reviewResponse
	r.into(t, &got)
	if r.Status != http.StatusOK || got.Enabled || !got.OK {
		t.Errorf("status %d, body %+v: want a disabled, quiet answer", r.Status, got)
	}
	if rv.count() != 0 {
		t.Errorf("a disabled review must not reach the reviewer, got %d calls", rv.count())
	}
}

func TestReviewPromptWithoutAReviewer(t *testing.T) {
	srv, _ := newReviewServer(t, nil, store.Settings{})
	r := do(t, srv, "POST", "/api/review/prompt", reviewRequest{Kind: "task", Prompt: "p"})
	var got reviewResponse
	r.into(t, &got)
	if r.Status != http.StatusOK || got.Enabled {
		t.Errorf("status %d, body %+v: a daemon without a reviewer reports it as unavailable", r.Status, got)
	}
}

func TestReviewPromptWithoutAClaudeBinary(t *testing.T) {
	// Not knowing where claude is is a Settings problem, already reported there.
	// The prompt sheet stays quiet instead of showing an error the operator
	// cannot act on from where they are.
	srv, _ := newReviewServer(t, &fakeReviewer{err: review.ErrNoBinary}, store.Settings{})
	r := do(t, srv, "POST", "/api/review/prompt", reviewRequest{Kind: "task", Prompt: "p"})
	var got reviewResponse
	r.into(t, &got)
	if r.Status != http.StatusOK || got.Enabled || !got.OK {
		t.Errorf("status %d, body %+v", r.Status, got)
	}
}

func TestReviewPromptReportsAFailedReview(t *testing.T) {
	srv, _ := newReviewServer(t, &fakeReviewer{err: errors.New("claude exploded")}, store.Settings{})
	if r := do(t, srv, "POST", "/api/review/prompt", reviewRequest{Kind: "task", Prompt: "p"}); r.Status != http.StatusBadGateway {
		t.Errorf("status %d, want 502: %s", r.Status, r.Body)
	}
}

func TestReviewPromptRejectsGarbage(t *testing.T) {
	srv, _ := newReviewServer(t, &fakeReviewer{}, store.Settings{})
	req, err := http.NewRequest("POST", srv.URL+"/api/review/prompt", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", res.StatusCode)
	}
}

func TestReviewPromptCancelsThePreviousReview(t *testing.T) {
	// The dashboard reviews on every change, so a keystroke must stop the model
	// call the previous one started instead of letting both finish.
	rv := &fakeReviewer{res: review.Result{OK: true}, block: make(chan struct{}), inFlgt: make(chan struct{})}
	srv, _ := newReviewServer(t, rv, store.Settings{})

	// Sent without the do() helper: it fails the test from inside, which is not
	// allowed off the test's own goroutine.
	first := make(chan int, 1)
	go func() {
		body, _ := json.Marshal(reviewRequest{Kind: "task", Prompt: "one"})
		res, err := http.Post(srv.URL+"/api/review/prompt", "application/json", bytes.NewReader(body))
		if err != nil {
			first <- 0
			return
		}
		defer func() { _ = res.Body.Close() }()
		first <- res.StatusCode
	}()
	select {
	case <-rv.inFlgt:
	case <-time.After(5 * time.Second):
		t.Fatal("the first review never started")
	}

	// The second review finds nothing blocking it, because block is only read
	// once; unblock it so the handler can answer.
	rv.mu.Lock()
	rv.block = nil
	rv.mu.Unlock()
	if r := do(t, srv, "POST", "/api/review/prompt", reviewRequest{Kind: "task", Prompt: "two"}); r.Status != http.StatusOK {
		t.Fatalf("the newest review must answer normally, got %d: %s", r.Status, r.Body)
	}

	select {
	case status := <-first:
		if status != http.StatusNoContent {
			t.Errorf("the superseded review answered %d, want 204", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the superseded review was never cancelled")
	}
}
