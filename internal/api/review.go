package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/review"
	"github.com/danielmaier42/claudeq/internal/store"
)

// PromptReviewer checks a draft prompt against this machine (satisfied by
// *review.Reviewer). Optional; without it the dashboard shows no suggestions.
type PromptReviewer interface {
	Review(ctx context.Context, req review.Request) (review.Result, error)
}

type reviewRequest struct {
	Kind       string `json:"kind"`
	Prompt     string `json:"prompt"`
	WorkingDir string `json:"working_dir"`
}

type reviewResponse struct {
	// Enabled is false when the operator turned the review off (or the daemon
	// cannot run one), so the dashboard hides the suggestion banner entirely
	// instead of reporting a problem that is not one.
	Enabled       bool   `json:"enabled"`
	OK            bool   `json:"ok"`
	Message       string `json:"message,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// reviewPrompt reviews a draft prompt without storing anything. The dashboard
// calls it while the operator types, so only the newest review matters: a new
// request cancels the one still running, which kills its Claude process rather
// than leaving it to finish an answer nobody will read.
func (s *server) reviewPrompt(w http.ResponseWriter, r *http.Request) {
	var in reviewRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.d.Review == nil || cfg.Settings.PromptReviewDisabled {
		writeJSON(w, http.StatusOK, reviewResponse{Enabled: false, OK: true})
		return
	}

	kind := review.KindTask
	if in.Kind == string(review.KindSystem) {
		kind = review.KindSystem
		in.WorkingDir = "" // the system prompt belongs to no single directory
	}

	// The review runs on the Claude Code instance, with the account claudeq is
	// actually configured for. Its own model setting wins; empty means that
	// provider's default model.
	bin, model := s.reviewTarget(cfg)
	if bin == "" {
		// No harness the reviewer can speak to. Same situation as the review
		// being switched off: nothing to show, and why is already on the
		// provider's card in Settings.
		writeJSON(w, http.StatusOK, reviewResponse{Enabled: false, OK: true})
		return
	}

	ctx, done := s.beginReview(r.Context())
	defer done()
	res, err := s.d.Review.Review(ctx, review.Request{
		Kind:       kind,
		Prompt:     in.Prompt,
		WorkingDir: in.WorkingDir,
		Model:      model,
		Bin:        bin,
	})
	switch {
	case ctx.Err() != nil:
		// Superseded by a newer keystroke, or the dashboard went away. There is
		// no answer to give and nothing went wrong.
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, review.ErrNoBinary):
		// Same situation as the review being switched off: nothing to show, and
		// the missing binary is already reported in Settings.
		writeJSON(w, http.StatusOK, reviewResponse{Enabled: false, OK: true})
	case err != nil:
		writeErr(w, http.StatusBadGateway, err)
	default:
		writeJSON(w, http.StatusOK, reviewResponse{
			Enabled: true, OK: res.OK, Message: res.Message, RevisedPrompt: res.RevisedPrompt,
		})
	}
}

// reviewTarget resolves the binary and model the prompt review runs with.
//
// It asks the Claude Code instance by name, not the default provider: the
// reviewer builds Claude Code's own flags (see internal/review), so pointing it
// at another harness would exec a command that harness does not have. Making
// the review provider-neutral is its own piece of work; until then this is the
// honest dependency rather than a silent one.
//
// No instance, no binary, or a provider that is switched off yields an empty
// binary, which the reviewer reports as ErrNoBinary — the same "nothing to show
// here" the dashboard already handles.
func (s *server) reviewTarget(cfg store.Config) (bin, model string) {
	set, err := provider.FromConfig(cfg)
	if err != nil {
		return "", ""
	}
	inst, ok := set.Lookup(provider.DefaultInstanceID)
	if !ok || inst.Kind != provider.KindClaudeCode {
		return "", ""
	}
	ad, err := s.d.Registry.Lookup(inst.Kind)
	if err != nil {
		return "", ""
	}
	model = cfg.Settings.PromptReviewModel
	if model == "" {
		model = inst.DefaultModel
	}
	return ad.ResolveBinary(inst), model
}

// beginReview makes this the only live review: it cancels whichever one was
// running and returns the new one's context plus the func that retires it.
func (s *server) beginReview(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	s.reviewMu.Lock()
	if s.reviewCancel != nil {
		s.reviewCancel()
	}
	s.reviewGen++
	gen := s.reviewGen
	s.reviewCancel = cancel
	s.reviewMu.Unlock()

	return ctx, func() {
		cancel()
		s.reviewMu.Lock()
		// Only clear the slot if a later review has not already claimed it.
		if s.reviewGen == gen {
			s.reviewCancel = nil
		}
		s.reviewMu.Unlock()
	}
}
