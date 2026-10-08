// The `claudeq pool` command group: provider pools, which spread a task's runs
// over several accounts. It is the command-line half of Settings → Pools.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
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
  claudeq pool next ID [--json]        (the member the next run goes to, and why)
  claudeq pool show ID [--json]        (the members, ranked as a run started now would try them)
  claudeq pool add  ID --member PROVIDER[=WEIGHT]... [--name N]
  claudeq pool edit ID [--name N] [--member PROVIDER[=WEIGHT]]...
                                       (--member replaces the whole member list)
  claudeq pool rm ID

WEIGHT is the plan's size relative to the others (Max 20x = 20). Leave it out
to take what the provider reports about its plan, or 1 when it reports none.

next and show ask the running daemon, as the Dashboard does, so rate-limit
pauses count. Without a daemon they read the members here.`

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
	case "next":
		return cmdPoolNext(st, rest)
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
// the order a run started now would try them. Source says where the ranking
// came from: "daemon" when the running daemon was asked, as the Dashboard
// does, or "local" when it was read here because no daemon answered.
type poolShowView struct {
	poolListView
	Source  string                 `json:"source"`
	Ranking []provider.MemberScore `json:"ranking"`
}

// poolNextView is what `pool next --json` prints: the member the next run
// would go to (nil when none can take work) and the ranking behind it.
type poolNextView struct {
	Pool    string                 `json:"pool"`
	Name    string                 `json:"name"`
	Source  string                 `json:"source"`
	Next    *provider.MemberScore  `json:"next"`
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
	v, err := poolRanking(st, id)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(v)
	}
	fmt.Printf("pool %s (%s)\n\n", v.ID, v.Name)
	if err := printRanking(v.Ranking); err != nil {
		return err
	}
	fmt.Printf("\nA run started now goes to the first member that can take work, ranked by urgency x weight.\n%s\n%s\n", urgencyNote, sourceNote(v.Source))
	return nil
}

func cmdPoolNext(st *store.Store, args []string) error {
	id, rest, err := splitPositional(args, "a pool id")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("pool next", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the chosen member and the ranking as JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	v, err := poolRanking(st, id)
	if err != nil {
		return err
	}
	out := poolNextView{Pool: v.ID, Name: v.Name, Source: v.Source, Ranking: v.Ranking}
	for i := range v.Ranking {
		if v.Ranking[i].Tier < 3 {
			out.Next = &v.Ranking[i]
			break
		}
	}
	if *asJSON {
		return printJSON(out)
	}
	if out.Next == nil {
		fmt.Printf("pool %s (%s): no member can take work right now\n\n", out.Pool, out.Name)
	} else {
		n := out.Next
		fmt.Printf("pool %s (%s): the next run goes to %s (%s)\n", out.Pool, out.Name, n.Name, n.ProviderID)
		fmt.Printf("  %s · weight %s× · urgency %.2f\n\n", n.Note, strconv.FormatFloat(n.Weight, 'f', -1, 64), n.Urgency)
	}
	if err := printRanking(out.Ranking); err != nil {
		return err
	}
	fmt.Println(sourceNote(out.Source))
	return nil
}

// printRanking prints a pool's members, best first, the way the Dashboard
// lists them: no urgency for a member that cannot take work.
func printRanking(ranking []provider.MemberScore) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "#\tPROVIDER\tWEIGHT\tURGENCY\tNOTE")
	for i, r := range ranking {
		urgency := "-"
		if r.Tier < 3 {
			urgency = strconv.FormatFloat(r.Urgency, 'f', 2, 64)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", i+1, r.ProviderID,
			strconv.FormatFloat(r.Weight, 'f', -1, 64), urgency, r.Note)
	}
	return w.Flush()
}

// urgencyNote says what the urgency figure means, wherever the CLI prints one.
const urgencyNote = "Urgency = free share of the week x its length / hours to reset, shared by the runs on it: 1 is on pace, 2 means half of what is left would expire unused."

func sourceNote(source string) string {
	if source == poolSourceDaemon {
		return "(as the daemon sees it, rate-limit pauses included)"
	}
	return "(read here: the daemon could not be asked, so rate-limit pauses are not known)"
}

const (
	poolSourceDaemon = "daemon"
	poolSourceLocal  = "local"
)

// daemonURL is where the running daemon serves its API; a variable so tests
// can put a stub server (or nothing) in its place.
var daemonURL = "http://127.0.0.1:10765"

// errNoDaemon means the daemon could not be asked: it is not running, or is
// too old to answer the question.
var errNoDaemon = errors.New("no daemon answered")

// poolRanking ranks a pool the way the Dashboard shows it. The running daemon
// is asked first: it knows the rate-limit pauses and its readings are the ones
// the scheduler goes by. Without one, the members are read here.
func poolRanking(st *store.Store, id string) (poolShowView, error) {
	v, err := daemonPool(id)
	if err == nil {
		v.Source = poolSourceDaemon
		return v, nil
	}
	if !errors.Is(err, errNoDaemon) {
		return poolShowView{}, err
	}
	set, err := app.Providers(st)
	if err != nil {
		return poolShowView{}, err
	}
	p, ok := set.LookupPool(id)
	if !ok {
		return poolShowView{}, fmt.Errorf("%w %q", provider.ErrUnknownPool, id)
	}
	s := p.Stored()
	return poolShowView{
		poolListView: poolListView{ID: s.ID, Name: s.Name, Members: s.Members},
		Source:       poolSourceLocal,
		Ranking:      rankPoolNow(set, p),
	}, nil
}

// daemonPool asks the running daemon for a pool and its ranking.
func daemonPool(id string) (poolShowView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), providerCheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, daemonURL+"/api/pools/"+url.PathEscape(id), nil)
	if err != nil {
		return poolShowView{}, fmt.Errorf("pool request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Not running, or hung past the deadline: either way it has no answer.
		return poolShowView{}, fmt.Errorf("%w: %w", errNoDaemon, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		switch {
		case json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error != "":
			return poolShowView{}, errors.New(e.Error)
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
			// A daemon from before GET /api/pools/{id}: its router answers
			// without the API's error body.
			return poolShowView{}, errNoDaemon
		default:
			return poolShowView{}, fmt.Errorf("daemon answered %s", resp.Status)
		}
	}
	// The daemon's pool view: members as {provider, weight}, like store.PoolMember.
	var body struct {
		ID      string                 `json:"id"`
		Name    string                 `json:"name"`
		Members []store.PoolMember     `json:"members"`
		Ranking []provider.MemberScore `json:"ranking"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return poolShowView{}, fmt.Errorf("read the daemon's answer: %w", err)
	}
	return poolShowView{poolListView: poolListView{ID: body.ID, Name: body.Name, Members: body.Members}, Ranking: body.Ranking}, nil
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
