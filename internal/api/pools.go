package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/provider"
)

// poolMemberView is one member of a pool as the app edits it.
type poolMemberView struct {
	Provider string  `json:"provider"`
	Weight   float64 `json:"weight"`
}

// poolView is a pool with where its members stand right now: the order a run
// started this moment would try them in, best first.
type poolView struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Members []poolMemberView       `json:"members"`
	Ranking []provider.MemberScore `json:"ranking"`
}

// poolInput is the editable part of a pool. The id comes from the URL on an
// update, and from the body when one is created.
type poolInput struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Members []poolMemberView `json:"members"`
}

func (in poolInput) pool(id string) provider.Pool {
	p := provider.Pool{ID: id, Name: in.Name, Members: make([]provider.PoolMember, len(in.Members))}
	if p.Name == "" {
		p.Name = id
	}
	for i, m := range in.Members {
		p.Members[i] = provider.PoolMember{ProviderID: m.Provider, Weight: m.Weight}
	}
	return p
}

func (s *server) listPools(w http.ResponseWriter, r *http.Request) {
	set, ok := s.providerSet(w)
	if !ok {
		return
	}
	pools := set.Pools()
	out := make([]poolView, len(pools))
	health := s.memberHealth(r.Context(), set, pools...)
	for i, p := range pools {
		out[i] = s.poolView(set, p, health)
	}
	writeJSON(w, http.StatusOK, out)
}

// memberHealth asks for the readiness of every member of pools, all at once.
// The verdicts come from the shared checker, which the scheduler keeps warm;
// a stale one is probed, in parallel, like the queue's blocked markers.
func (s *server) memberHealth(ctx context.Context, set provider.Set, pools ...provider.Pool) map[string]provider.Health {
	var insts []provider.Instance
	seen := map[string]bool{}
	for _, p := range pools {
		for _, inst := range set.PoolMembers(p) {
			if !seen[inst.ID] {
				seen[inst.ID] = true
				insts = append(insts, inst)
			}
		}
	}
	out := make(map[string]provider.Health, len(insts))
	for i, h := range s.d.Providers.CheckEach(ctx, insts) {
		out[insts[i].ID] = h
	}
	return out
}

// poolView ranks a pool's members from what the daemon already knows: the
// last limit readings (never read here), the readiness verdicts in health and
// the rate-limit gates. It is the order a run started now would take.
func (s *server) poolView(set provider.Set, p provider.Pool, health map[string]provider.Health) poolView {
	v := poolView{ID: p.ID, Name: p.Label(), Members: make([]poolMemberView, len(p.Members))}
	for i, m := range p.Members {
		v.Members[i] = poolMemberView{Provider: m.ProviderID, Weight: m.Weight}
	}
	var blocked map[string]time.Time
	if s.d.BlockedProviders != nil {
		blocked = s.d.BlockedProviders()
	}
	limits := func(provider.Instance) (provider.Limits, bool) { return provider.Limits{}, false }
	if s.d.Limits != nil {
		limits = s.d.Limits.Cached
	}
	v.Ranking = provider.RankPool(set, p, provider.DisplayInputs(time.Now(), limits,
		func(inst provider.Instance) provider.Health { return health[inst.ID] },
		func(id string) bool { _, limited := blocked[id]; return limited }))
	return v
}

func (s *server) addPool(w http.ResponseWriter, r *http.Request) {
	var in poolInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := app.AddPool(s.d.Store, in.pool(in.ID)); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.writePool(w, r, in.ID, http.StatusCreated)
}

func (s *server) updatePool(w http.ResponseWriter, r *http.Request) {
	var in poolInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	id := r.PathValue("id")
	if err := app.EditPool(s.d.Store, id, func(p *provider.Pool) error {
		*p = in.pool(id)
		return nil
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.writePool(w, r, id, http.StatusOK)
}

func (s *server) deletePool(w http.ResponseWriter, r *http.Request) {
	if err := app.RemovePool(s.d.Store, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) writePool(w http.ResponseWriter, r *http.Request, id string, status int) {
	set, ok := s.providerSet(w)
	if !ok {
		return
	}
	p, found := set.LookupPool(id)
	if !found {
		writeErr(w, http.StatusNotFound, fmt.Errorf("%w %q", provider.ErrUnknownPool, id))
		return
	}
	writeJSON(w, status, s.poolView(set, p, s.memberHealth(r.Context(), set, p)))
}
