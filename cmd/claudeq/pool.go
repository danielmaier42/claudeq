// The `claudeq pool` command group: provider pools, which spread a task's runs
// over several accounts. It is the command-line half of Settings → Pools.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

const poolUsage = `claudeq pool - provider pools: one task, several accounts

A task on a pool runs on the member whose weekly allowance is most at risk of
expiring unused: free share x weight / hours until it resets.

Usage:
  claudeq pool list [--json]
  claudeq pool show ID [--json]        (the members, ranked as a run started now would try them)
  claudeq pool add  ID --member PROVIDER[=WEIGHT]... [--name N]
  claudeq pool edit ID [--name N] [--member PROVIDER[=WEIGHT]]...
                                       (--member replaces the whole member list)
  claudeq pool rm ID

WEIGHT is the plan's size relative to the others (Max 20x = 20). Leave it out
to take what the provider reports about its plan, or 1 when it reports none.`

func cmdPool(st *store.Store, args []string) error {
	if len(args) == 0 {
		fmt.Println(poolUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return cmdPoolList(st, rest)
	case "show":
		return cmdPoolShow(st, rest)
	case "add":
		return cmdPoolAdd(st, rest)
	case "edit":
		return cmdPoolEdit(st, rest)
	case "rm":
		if len(rest) != 1 {
			return fmt.Errorf("expected exactly one pool id")
		}
		if err := app.RemovePool(st, rest[0]); err != nil {
			return err
		}
		fmt.Printf("pool %q removed\n", rest[0])
		return nil
	default:
		fmt.Println(poolUsage)
		return fmt.Errorf("unknown pool command %q", sub)
	}
}

// memberList collects repeated --member flags.
type memberList []provider.PoolMember

func (m *memberList) String() string {
	parts := make([]string, len(*m))
	for i, mem := range *m {
		parts[i] = mem.ProviderID
	}
	return strings.Join(parts, ",")
}

func (m *memberList) Set(v string) error {
	id, weight, hasWeight := strings.Cut(strings.TrimSpace(v), "=")
	mem := provider.PoolMember{ProviderID: id}
	if hasWeight {
		w, err := strconv.ParseFloat(weight, 64)
		if err != nil {
			return fmt.Errorf("weight of %q: %w", id, err)
		}
		mem.Weight = w
	}
	*m = append(*m, mem)
	return nil
}

// poolListView is what `pool list --json` prints per pool.
type poolListView struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Members []store.PoolMember `json:"members"`
}

func cmdPoolList(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("pool list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the pools as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	set, err := app.Providers(st)
	if err != nil {
		return err
	}
	pools := set.Pools()
	views := make([]poolListView, len(pools))
	for i, p := range pools {
		s := p.Stored()
		views[i] = poolListView{ID: s.ID, Name: s.Name, Members: s.Members}
	}
	if *asJSON {
		return printJSON(views)
	}
	if len(views) == 0 {
		fmt.Println("no pools configured (claudeq pool add ID --member PROVIDER --member PROVIDER)")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tMEMBERS")
	for _, v := range views {
		members := make([]string, len(v.Members))
		for i, m := range v.Members {
			members[i] = m.Provider
			if m.Weight > 0 {
				members[i] += "=" + strconv.FormatFloat(m.Weight, 'f', -1, 64)
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", v.ID, v.Name, strings.Join(members, ", "))
	}
	return w.Flush()
}

// poolShowView is what `pool show --json` prints: the pool and its members in
// the order a run started now would try them.
type poolShowView struct {
	poolListView
	Ranking []provider.MemberScore `json:"ranking"`
}

func cmdPoolShow(st *store.Store, args []string) error {
	id, rest, err := splitPositional(args, "a pool id")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("pool show", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the pool and its ranking as JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	set, err := app.Providers(st)
	if err != nil {
		return err
	}
	p, ok := set.LookupPool(id)
	if !ok {
		return fmt.Errorf("%w %q", provider.ErrUnknownPool, id)
	}
	ranking := rankPoolNow(set, p)
	s := p.Stored()
	v := poolShowView{poolListView: poolListView{ID: s.ID, Name: s.Name, Members: s.Members}, Ranking: ranking}
	if *asJSON {
		return printJSON(v)
	}
	fmt.Printf("pool %s (%s)\n\n", v.ID, v.Name)
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "#\tPROVIDER\tWEIGHT\tURGENCY\tNOTE")
	for i, r := range ranking {
		fmt.Fprintf(w, "%d\t%s\t%s\t%.2f\t%s\n", i+1, r.ProviderID,
			strconv.FormatFloat(r.Weight, 'f', -1, 64), r.Urgency, r.Note)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Println("\nA run started now goes to #1. Urgency = free % of the week x weight / hours to reset.")
	return nil
}

// rankPoolNow ranks a pool from fresh readings: each member's allowance and
// readiness are read right now, the way the daemon would just before a start.
// Runs in flight and rate-limit pauses are the daemon's to know, so they do not
// count here.
func rankPoolNow(set provider.Set, p provider.Pool) []provider.MemberScore {
	ctx, cancel := context.WithTimeout(context.Background(), providerCheckTimeout)
	defer cancel()
	members := set.PoolMembers(p)
	limits := provider.NewLimitMonitor(newProviderRegistry(), nil).Get(ctx, members, true)
	byID := make(map[string]provider.Limits, len(members))
	for i, inst := range members {
		byID[inst.ID] = limits[i]
	}
	checker := newProviderChecker()
	health := checker.CheckEach(ctx, members)
	healthByID := make(map[string]provider.Health, len(members))
	for i, inst := range members {
		healthByID[inst.ID] = health[i]
	}
	return provider.RankPool(set, p, provider.DisplayInputs(time.Now(),
		func(inst provider.Instance) (provider.Limits, bool) {
			l, ok := byID[inst.ID]
			return l, ok && len(l.Windows) > 0
		},
		func(inst provider.Instance) provider.Health { return healthByID[inst.ID] },
		nil))
}

func cmdPoolAdd(st *store.Store, args []string) error {
	id, rest, err := splitPositional(args, "a pool id")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("pool add", flag.ContinueOnError)
	name := fs.String("name", "", "display name (defaults to the id)")
	var members memberList
	fs.Var(&members, "member", "a provider in the pool, optionally with its weight: PROVIDER[=WEIGHT] (repeatable)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q", strings.Join(fs.Args(), " "))
	}
	p := provider.Pool{ID: id, Name: *name, Members: members}
	if p.Name == "" {
		p.Name = id
	}
	if err := app.AddPool(st, p); err != nil {
		return err
	}
	fmt.Printf("added pool %q\n", id)
	return nil
}

func cmdPoolEdit(st *store.Store, args []string) error {
	id, rest, err := splitPositional(args, "a pool id")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("pool edit", flag.ContinueOnError)
	name := fs.String("name", "", "display name")
	var members memberList
	fs.Var(&members, "member", "the new member list, PROVIDER[=WEIGHT] (repeatable; replaces the current one)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q", strings.Join(fs.Args(), " "))
	}
	passed := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { passed[f.Name] = true })
	if err := app.EditPool(st, id, func(p *provider.Pool) error {
		if passed["name"] {
			p.Name = orDefault(*name, id)
		}
		if passed["member"] {
			p.Members = members
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("updated pool %q\n", id)
	return nil
}
