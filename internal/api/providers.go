package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/provider"
)

// providerView is one configured instance plus what claudeq currently knows
// about it: whether it can run a job, and whether it is the default. It carries
// no credentials — the provider CLIs own those, and a readiness check never
// reads them into claudeq.
type providerView struct {
	provider.Instance
	Health  provider.Health `json:"health"`
	Default bool            `json:"default"`
	// Detected is the binary claudeq would use when no path is configured, so
	// the Settings card can offer it instead of asking the operator to find it.
	Detected string `json:"detected,omitempty"`
}

// providerSet reads the configured instances, answering the caller with a plain
// error when the configuration cannot be read at all.
func (s *server) providerSet(w http.ResponseWriter) (provider.Set, bool) {
	set, err := app.Providers(s.d.Store)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return provider.Set{}, false
	}
	return set, true
}

// views renders instances with their health. fresh re-probes rather than
// reusing a recent verdict, which is what the Settings card's "Check again"
// asks for.
func (s *server) views(ctx context.Context, set provider.Set, only string, fresh bool) []providerView {
	out := make([]providerView, 0, len(set.All()))
	for _, inst := range set.All() {
		if only != "" && inst.ID != only {
			continue
		}
		h := s.d.Providers.CheckMaybeFresh(ctx, inst, fresh)
		v := providerView{Instance: inst, Health: h, Default: inst.ID == set.DefaultID()}
		if inst.BinaryPath == "" {
			if ad, err := s.d.Registry.Lookup(inst.Kind); err == nil {
				v.Detected = ad.DetectBinary()
			}
		}
		out = append(out, v)
	}
	return out
}

func (s *server) listProviders(w http.ResponseWriter, r *http.Request) {
	set, ok := s.providerSet(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.views(r.Context(), set, "", false))
}

// checkProvider re-probes one instance and returns its fresh verdict. This is
// the only endpoint that always spends the probe, so the operator gets an
// answer about the state right now rather than one from a minute ago.
func (s *server) checkProvider(w http.ResponseWriter, r *http.Request) {
	s.writeOne(w, r, r.PathValue("id"), http.StatusOK)
}

// writeOne answers with one provider and a verdict probed just now, which is
// what every caller that just changed something wants to see.
func (s *server) writeOne(w http.ResponseWriter, r *http.Request, id string, status int) {
	set, ok := s.providerSet(w)
	if !ok {
		return
	}
	views := s.views(r.Context(), set, id, true)
	if len(views) == 0 {
		writeErr(w, http.StatusNotFound, fmt.Errorf("%w %q", provider.ErrUnknownProvider, id))
		return
	}
	writeJSON(w, status, views[0])
}

// providerInput is the editable part of an instance. The id comes from the URL
// on an update, and from the body when one is created.
type providerInput struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	BinaryPath   string `json:"binary_path"`
	ConfigDir    string `json:"config_dir"`
	DefaultModel string `json:"default_model"`
	Enabled      *bool  `json:"enabled"`
}

func (s *server) addProvider(w http.ResponseWriter, r *http.Request) {
	var in providerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	inst := provider.Instance{
		ID:           in.ID,
		Kind:         provider.Kind(in.Kind),
		Name:         in.Name,
		BinaryPath:   in.BinaryPath,
		ConfigDir:    in.ConfigDir,
		DefaultModel: in.DefaultModel,
		Enabled:      in.Enabled == nil || *in.Enabled,
	}
	if inst.Name == "" {
		inst.Name = inst.ID
	}
	if err := app.AddProvider(s.d.Store, s.d.Registry, inst); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.writeOne(w, r, inst.ID, http.StatusCreated)
}

// updateProvider changes one instance. Its kind is fixed: a configured instance
// keeps the harness it was created for, and switching that would silently
// re-point its tasks and sessions at a different CLI.
func (s *server) updateProvider(w http.ResponseWriter, r *http.Request) {
	var in providerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	id := r.PathValue("id")
	err := app.EditProvider(s.d.Store, s.d.Registry, id, func(inst *provider.Instance) error {
		if in.Kind != "" && provider.Kind(in.Kind) != inst.Kind {
			return errors.New("a provider's kind cannot be changed; remove it and add a new one")
		}
		inst.Name = in.Name
		if inst.Name == "" {
			inst.Name = inst.ID
		}
		inst.BinaryPath = in.BinaryPath
		inst.ConfigDir = in.ConfigDir
		inst.DefaultModel = in.DefaultModel
		if in.Enabled != nil {
			inst.Enabled = *in.Enabled
		}
		return nil
	})
	if err != nil {
		writeErr(w, statusForProviderErr(err), err)
		return
	}
	// The configuration changed, so the verdict recorded for the old one no
	// longer describes what would run.
	s.d.Providers.Forget(id)
	s.writeOne(w, r, id, http.StatusOK)
}

func (s *server) enableProvider(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := app.SetProviderEnabled(s.d.Store, id, enabled); err != nil {
			writeErr(w, statusForProviderErr(err), err)
			return
		}
		s.d.Providers.Forget(id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := app.RemoveProvider(s.d.Store, id); err != nil {
		writeErr(w, statusForProviderErr(err), err)
		return
	}
	s.d.Providers.Forget(id)
	w.WriteHeader(http.StatusNoContent)
}

// setDefaultProvider chooses which instance a task runs on when it names none.
func (s *server) setDefaultProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := app.SetDefaultProvider(s.d.Store, id); err != nil {
		writeErr(w, statusForProviderErr(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// statusForProviderErr separates "no such provider" from "that change is not
// allowed", so the dashboard can tell a stale id from a rejected edit.
func statusForProviderErr(err error) int {
	if errors.Is(err, provider.ErrUnknownProvider) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
