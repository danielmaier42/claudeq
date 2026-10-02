package api

import (
	"net/http"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// limitsView is one provider's allowance as the dashboard shows it: which
// account, what kind, and what its windows say.
type limitsView struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	TypeName string          `json:"type_name"`
	Default  bool            `json:"default"`
	Limits   provider.Limits `json:"limits"`
}

// listLimits answers with every configured provider's allowance, in
// configuration order. Without ?fresh=1 it is answered from the daemon's last
// readings and costs nothing; with it, readings older than half a minute are
// taken again — what opening the dashboard and View > Refresh ask for.
func (s *server) listLimits(w http.ResponseWriter, r *http.Request) {
	set, ok := s.providerSet(w)
	if !ok {
		return
	}
	insts := set.All()
	out := make([]limitsView, 0, len(insts))
	if s.d.Limits == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	limits := s.d.Limits.Get(r.Context(), insts, r.URL.Query().Get("fresh") == "1")
	for i, inst := range insts {
		v := limitsView{ID: inst.ID, Name: inst.Label(), Default: inst.ID == set.DefaultID(), Limits: limits[i]}
		if ad, err := s.d.Registry.Lookup(inst.Kind); err == nil {
			v.TypeName = ad.Describe().Name
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}
