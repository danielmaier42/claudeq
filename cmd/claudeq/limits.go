package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/provider/adapters"
	"github.com/danielmaier42/claudeq/internal/store"
)

// newProviderRegistry is the adapter set `provider limits` reads through; a
// variable so tests can put a harness with a scripted allowance in its place.
var newProviderRegistry = adapters.Default

// limitsView is what `provider limits --json` prints per provider: the same
// shape the dashboard's /api/limits answers with.
type limitsView struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	TypeName string          `json:"type_name"`
	Default  bool            `json:"default"`
	Limits   provider.Limits `json:"limits"`
}

// cmdProviderLimits reads every provider's allowance (or one provider's) right
// now, straight from the harnesses — the same readings the daemon takes, and
// like them it costs no usage.
func cmdProviderLimits(st *store.Store, args []string) error {
	var only string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		only, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("provider limits", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the limits as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// The id may come after the flags too (`--json codex`); anything beyond one
	// id is a mistake, not something to ignore.
	if rest := fs.Args(); len(rest) > 0 {
		if only != "" || len(rest) > 1 {
			return fmt.Errorf("expected at most one provider id, got %q", strings.Join(append([]string{only}, rest...), " "))
		}
		only = rest[0]
	}
	set, err := app.Providers(st)
	if err != nil {
		return err
	}
	var insts []provider.Instance
	for _, inst := range set.All() {
		if only == "" || inst.ID == only {
			insts = append(insts, inst)
		}
	}
	if only != "" && len(insts) == 0 {
		return fmt.Errorf("%w %q", provider.ErrUnknownProvider, only)
	}

	reg := newProviderRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), providerCheckTimeout)
	defer cancel()
	limits := provider.NewLimitMonitor(reg, nil).Get(ctx, insts, true)
	views := make([]limitsView, len(insts))
	for i, inst := range insts {
		views[i] = limitsView{ID: inst.ID, Name: inst.Label(), Default: inst.ID == set.DefaultID(), Limits: limits[i]}
		if ad, err := reg.Lookup(inst.Kind); err == nil {
			views[i].TypeName = ad.Describe().Name
		}
	}
	if *asJSON {
		return printJSON(views)
	}
	return printLimits(views, time.Now())
}

// printLimits writes one line per window, and one line for a provider that has
// none to show, with the reason in the last column.
func printLimits(views []limitsView, now time.Time) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tWINDOW\tUSED\tRESETS\tNOTE")
	for _, v := range views {
		l := v.Limits
		note := ""
		switch l.State {
		case provider.LimitsDisabled:
			note = "switched off"
		case provider.LimitsUnsupported:
			note = l.Reason
		case provider.LimitsUnavailable:
			note = l.Reason
			if !l.UpdatedAt.IsZero() {
				note += " (figures from " + l.UpdatedAt.Local().Format("Mon 15:04") + ")"
			}
		}
		if len(l.Windows) == 0 {
			fmt.Fprintf(w, "%s\t-\t-\t-\t%s\n", v.ID, note)
			continue
		}
		for _, win := range l.Windows {
			fmt.Fprintf(w, "%s\t%s\t%.0f%%\t%s\t%s\n", v.ID, win.Label, win.UsedPercent, resetsText(win.ResetsAt, now), note)
		}
	}
	return w.Flush()
}

// resetsText is when a window starts over: the local day and time, and how far
// off that is.
func resetsText(at *time.Time, now time.Time) string {
	if at == nil {
		return "-"
	}
	d := at.Sub(now)
	if d <= 0 {
		return "now"
	}
	return at.Local().Format("Mon 15:04") + " (in " + shortDuration(d) + ")"
}

func shortDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dm", max(1, int(d.Minutes())))
	}
}
