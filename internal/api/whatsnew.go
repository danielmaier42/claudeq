package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/danielmaier42/claudeq/internal/review"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// WhatsNewEdition names the release the "What's new" dialog introduces. The
// dialog shows until the user closes it once for this edition.
const WhatsNewEdition = "2.0"

// renameCheckParallel bounds how many tasks the rename check reviews at once:
// enough that a long queue does not take minutes, few enough not to flood the
// reviewing account.
const renameCheckParallel = 3

type whatsNewStatus struct {
	Edition string `json:"edition"`
	Show    bool   `json:"show"`
	// RenamePending asks for the rename check: some task or the custom system
	// prompt still names ClaudeQ, and the check has not settled that yet.
	RenamePending bool `json:"rename_pending"`
}

func (s *server) getWhatsNew(w http.ResponseWriter, _ *http.Request) {
	st, err := s.d.Store.LoadState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := whatsNewStatus{Edition: WhatsNewEdition, Show: st.WhatsNewSeen != WhatsNewEdition}
	if !st.RenameCheckDone {
		cfg, err := s.d.Store.LoadConfig()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		out.RenamePending = len(renameCandidates(cfg)) > 0
	}
	writeJSON(w, http.StatusOK, out)
}

// renameCandidates are the prompts the rename check reviews: the tasks that
// mention ClaudeQ, and the custom system prompt if it does.
func renameCandidates(cfg store.Config) []task.Task {
	var todo []task.Task
	for _, t := range cfg.Tasks {
		if mentionsClaudeQ(t.Prompt) {
			todo = append(todo, t)
		}
	}
	if mentionsClaudeQ(cfg.Settings.SystemPrompt) {
		todo = append(todo, task.Task{ID: renameSystemID, Name: "Custom system prompt", Prompt: cfg.Settings.SystemPrompt})
	}
	return todo
}

// dismissRenameCheck waves the check off: the banner offering it goes away
// for good.
func (s *server) dismissRenameCheck(w http.ResponseWriter, _ *http.Request) {
	if err := s.markRenameCheckDone(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) markRenameCheckDone() error {
	return s.d.Store.UpdateState(func(st *store.State) error {
		st.RenameCheckDone = true
		return nil
	})
}

// settled reports whether a finished check leaves nothing to do: every prompt
// was reviewed and no rewrite is waiting to be applied.
func (c renameCheck) settled() bool {
	if c.State != "done" || len(c.Errors) > 0 {
		return false
	}
	for _, f := range c.Findings {
		if f.RevisedPrompt != "" {
			return false
		}
	}
	return true
}

func (s *server) dismissWhatsNew(w http.ResponseWriter, _ *http.Request) {
	err := s.d.Store.UpdateState(func(st *store.State) error {
		st.WhatsNewSeen = WhatsNewEdition
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, whatsNewStatus{Edition: WhatsNewEdition})
}

// renameSystemID stands for the custom system prompt among the findings: it
// goes into every run, so a ClaudeQ path there breaks them all.
const renameSystemID = "@system"

// renameFinding is one task (or the custom system prompt) the rename check
// wants to change.
type renameFinding struct {
	TaskID        string `json:"task_id"`
	Name          string `json:"name"`
	Message       string `json:"message"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	// prompt is the text that was checked. Applying the revision requires the
	// task to still read exactly this, so an edit made since is never lost.
	prompt string
}

// renameCheck is the state of the check the What's new dialog starts. It runs
// in the background: a queue of many tasks takes longer than a request should.
type renameCheck struct {
	// State is "idle" (never started), "running", "done" or "unavailable"
	// (no reviewer is configured, or it cannot answer).
	State    string          `json:"state"`
	Total    int             `json:"total"`
	Checked  int             `json:"checked"`
	Skipped  int             `json:"skipped"`
	Findings []renameFinding `json:"findings"`
	Errors   []string        `json:"errors,omitempty"`
}

// renameJob guards the one rename check of this daemon.
type renameJob struct {
	mu    sync.Mutex
	check renameCheck
}

func (j *renameJob) snapshot() renameCheck {
	j.mu.Lock()
	defer j.mu.Unlock()
	c := j.check
	if c.State == "" {
		c.State = "idle"
	}
	c.Findings = append([]renameFinding{}, c.Findings...)
	c.Errors = append([]string(nil), c.Errors...)
	return c
}

// mentionsClaudeQ reports whether a prompt can need the rename at all. Only
// these go to the reviewer: a prompt that never names ClaudeQ has nothing to
// migrate, and checking it would only cost model usage. The working directory
// is not looked at: only the prompt can be rewritten, the old data folder
// stays reachable through its link, and a repository called claudeq keeps
// its name.
func mentionsClaudeQ(text string) bool {
	return strings.Contains(strings.ToLower(text), "claudeq")
}

func (s *server) getRenameCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.rename.snapshot())
}

// startRenameCheck reviews every task that mentions ClaudeQ with the prompt
// reviewer. A check already running is left to finish and reported as is.
func (s *server) startRenameCheck(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Resolved before taking the lock: it probes the filesystem for the
	// provider's binary, and the dialog's polls must not wait on that.
	inst, model, ok := s.reviewTarget(cfg)
	s.rename.mu.Lock()
	if s.rename.check.State == "running" {
		s.rename.mu.Unlock()
		writeJSON(w, http.StatusOK, s.rename.snapshot())
		return
	}
	if s.d.Review == nil || !ok {
		s.rename.check = renameCheck{State: "unavailable"}
		s.rename.mu.Unlock()
		writeJSON(w, http.StatusOK, s.rename.snapshot())
		return
	}
	todo := renameCandidates(cfg)
	skipped := len(cfg.Tasks)
	for _, t := range todo {
		if t.ID != renameSystemID {
			skipped--
		}
	}
	s.rename.check = renameCheck{State: "running", Total: len(todo), Skipped: skipped}
	s.rename.mu.Unlock()

	go func() {
		sem := make(chan struct{}, renameCheckParallel)
		var wg sync.WaitGroup
		for _, t := range todo {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				res, err := s.d.Review.Review(context.Background(), review.Request{
					Kind: review.KindRename, Prompt: t.Prompt, WorkingDir: t.WorkingDir,
					Model: model, Provider: inst,
				})
				s.rename.mu.Lock()
				defer s.rename.mu.Unlock()
				s.rename.check.Checked++
				switch {
				case err != nil:
					s.rename.check.Errors = append(s.rename.check.Errors, t.Name+": "+err.Error())
				case !res.OK:
					s.rename.check.Findings = append(s.rename.check.Findings, renameFinding{
						TaskID: t.ID, Name: t.Name, Message: res.Message,
						RevisedPrompt: res.RevisedPrompt, prompt: t.Prompt,
					})
				}
			}()
		}
		wg.Wait()
		// Remembered before "done" is published: the dialog asks for the
		// banner's state as soon as it sees "done", and must not read the old one.
		s.rename.mu.Lock()
		finished := s.rename.check
		finished.State = "done"
		s.rename.mu.Unlock()
		var persistErr error
		if finished.settled() {
			persistErr = s.markRenameCheckDone()
		}
		s.rename.mu.Lock()
		s.rename.check.State = "done"
		if persistErr != nil {
			s.rename.check.Errors = append(s.rename.check.Errors, "remembering the check: "+persistErr.Error())
		}
		s.rename.mu.Unlock()
	}()
	writeJSON(w, http.StatusOK, s.rename.snapshot())
}

type renameApplyRequest struct {
	TaskIDs []string `json:"task_ids"`
}

type renameApplyResponse struct {
	Applied []string `json:"applied"`
	// Changed lists the tasks left alone because they were edited after the
	// check; their revision would overwrite that edit.
	Changed []string `json:"changed,omitempty"`
}

// applyRename writes the revised prompt of each selected finding to its task,
// or to the custom system prompt.
func (s *server) applyRename(w http.ResponseWriter, r *http.Request) {
	var in renameApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	check := s.rename.snapshot()
	if check.State != "done" {
		writeErr(w, http.StatusConflict, errors.New("the check has not finished"))
		return
	}
	byID := map[string]renameFinding{}
	s.rename.mu.Lock()
	for _, f := range s.rename.check.Findings {
		if f.RevisedPrompt != "" {
			byID[f.TaskID] = f
		}
	}
	s.rename.mu.Unlock()

	var out renameApplyResponse
	err := s.d.Store.UpdateConfig(func(cfg *store.Config) error {
		out = renameApplyResponse{}
		for _, id := range in.TaskIDs {
			f, ok := byID[id]
			if !ok {
				continue
			}
			if id == renameSystemID {
				if cfg.Settings.SystemPrompt != f.prompt {
					out.Changed = append(out.Changed, id)
					continue
				}
				cfg.Settings.SystemPrompt = f.RevisedPrompt
				out.Applied = append(out.Applied, id)
				continue
			}
			for i := range cfg.Tasks {
				t := &cfg.Tasks[i]
				if t.ID != id {
					continue
				}
				if t.Prompt != f.prompt {
					out.Changed = append(out.Changed, id)
					break
				}
				t.Prompt = f.RevisedPrompt
				out.Applied = append(out.Applied, id)
				break
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.rename.mu.Lock()
	kept := s.rename.check.Findings[:0]
	for _, f := range s.rename.check.Findings {
		if !contains(out.Applied, f.TaskID) {
			kept = append(kept, f)
		}
	}
	s.rename.check.Findings = kept
	settled := s.rename.check.settled()
	s.rename.mu.Unlock()
	if settled {
		if err := s.markRenameCheckDone(); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
