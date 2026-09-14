// The `claudeq provider` command group: inspecting and configuring the harness
// instances tasks run on. It is the command-line half of Settings → Providers.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/provider/adapters"
	"github.com/danielmaier42/claudeq/internal/store"
)

const providerUsage = `claudeq provider - the harnesses tasks run on

Usage:
  claudeq provider list [--json]
  claudeq provider show ID [--json]
  claudeq provider check ID [--json]   (probe it now, ignoring the cached verdict)
  claudeq provider add  ID --kind KIND [--name N] [--path PATH]
                        [--config-dir PATH] [--default-model MODEL]
  claudeq provider edit ID [--name N] [--path PATH]
                        [--config-dir PATH] [--default-model MODEL]
  claudeq provider enable ID | claudeq provider disable ID
  claudeq provider default ID          (run tasks that name no provider on it)
  claudeq provider rm ID`

// providerCheckTimeout bounds a whole `provider list` pass: every configured
// instance is probed, and a CLI that hangs must not hang the command.
const providerCheckTimeout = 60 * time.Second

func cmdProvider(st *store.Store, args []string) error {
	if len(args) == 0 {
		fmt.Println(providerUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return cmdProviderList(st, rest)
	case "show":
		return cmdProviderShow(st, rest, false)
	case "check":
		return cmdProviderShow(st, rest, true)
	case "add":
		return cmdProviderAdd(st, rest)
	case "edit":
		return cmdProviderEdit(st, rest)
	case "enable":
		return withProviderID(rest, "enabled", func(id string) error { return app.SetProviderEnabled(st, id, true) })
	case "disable":
		return withProviderID(rest, "disabled", func(id string) error { return app.SetProviderEnabled(st, id, false) })
	case "default":
		return withProviderID(rest, "is now the default provider;", func(id string) error { return app.SetDefaultProvider(st, id) })
	case "rm":
		return withProviderID(rest, "removed", func(id string) error { return app.RemoveProvider(st, id) })
	default:
		fmt.Println(providerUsage)
		return fmt.Errorf("unknown provider command %q", sub)
	}
}

// withProviderID runs a one-argument provider command and reports what it did,
// so the confirmation says which change landed rather than a generic "updated".
func withProviderID(args []string, done string, fn func(string) error) error {
	if len(args) != 1 {
		return fmt.Errorf("expected exactly one provider id")
	}
	if err := fn(args[0]); err != nil {
		return err
	}
	fmt.Printf("provider %q %s\n", args[0], done)
	return nil
}

// providerView is what `provider list --json` and `provider show --json` print:
// the configured instance, whether it is the default, and its readiness. A
// running job reads this to find out which providers can take work.
type providerView struct {
	provider.Instance
	Default bool            `json:"default"`
	Health  provider.Health `json:"health"`
}

// checkProviders probes every instance in set. fresh is what `provider check`
// passes; the other commands run in a fresh process, where the cache is empty
// anyway, so it makes no difference to them.
func checkProviders(set provider.Set, only string, fresh bool) ([]providerView, error) {
	checker := provider.NewChecker(adapters.Default())
	ctx, cancel := context.WithTimeout(context.Background(), providerCheckTimeout)
	defer cancel()

	var out []providerView
	for _, inst := range set.All() {
		if only != "" && inst.ID != only {
			continue
		}
		h := checker.CheckMaybeFresh(ctx, inst, fresh)
		out = append(out, providerView{Instance: inst, Default: inst.ID == set.DefaultID(), Health: h})
	}
	if only != "" && len(out) == 0 {
		return nil, fmt.Errorf("%w %q", provider.ErrUnknownProvider, only)
	}
	return out, nil
}

func cmdProviderList(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("provider list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the providers as JSON, readiness included")
	if err := fs.Parse(args); err != nil {
		return err
	}
	set, err := app.Providers(st)
	if err != nil {
		return err
	}
	views, err := checkProviders(set, "", false)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(views)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "\tID\tKIND\tNAME\tDEFAULT MODEL\tSTATUS")
	for _, v := range views {
		mark := " "
		if v.Default {
			mark = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			mark, v.ID, v.Kind, v.Name, orDefault(v.DefaultModel, "(provider default)"), v.Health.State)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Println("\n* the provider a task runs on when it names none")
	return nil
}

func cmdProviderShow(st *store.Store, args []string, fresh bool) error {
	id, rest, err := splitPositional(args, "a provider id")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("provider show", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the provider as JSON")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	set, err := app.Providers(st)
	if err != nil {
		return err
	}
	views, err := checkProviders(set, id, fresh)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(views[0])
	}
	printProvider(views[0])
	return nil
}

func printProvider(v providerView) {
	fmt.Printf("id:             %s\n", v.ID)
	fmt.Printf("kind:           %s\n", v.Kind)
	fmt.Printf("name:           %s\n", v.Name)
	fmt.Printf("enabled:        %t\n", v.Enabled)
	fmt.Printf("default:        %t\n", v.Default)
	fmt.Printf("binary_path:    %s\n", orDefault(v.BinaryPath, "(auto-detect)"))
	fmt.Printf("config_dir:     %s\n", orDefault(v.ConfigDir, "(the CLI's own)"))
	fmt.Printf("default_model:  %s\n", orDefault(v.DefaultModel, "(the provider's own default)"))
	fmt.Printf("status:         %s\n", v.Health.State)
	if v.Health.Binary != "" {
		fmt.Printf("resolved:       %s\n", v.Health.Binary)
	}
	if v.Health.Detail != "" {
		fmt.Printf("detail:         %s\n", v.Health.Detail)
	}
	if v.Health.Reason != "" {
		fmt.Printf("reason:         %s\n", v.Health.Reason)
	}
}

// providerPatch is a parsed set of `provider add`/`provider edit` flags: the
// values plus which of them the caller passed, so an edit changes only the
// fields it names.
type providerPatch struct {
	set          map[string]bool
	kind         string
	name         string
	path         string
	configDir    string
	defaultModel string
}

func (p *providerPatch) register(fs *flag.FlagSet, withKind bool) {
	if withKind {
		fs.StringVar(&p.kind, "kind", "", "adapter kind the instance runs on (required)")
	}
	fs.StringVar(&p.name, "name", "", "display name (defaults to the id)")
	fs.StringVar(&p.path, "path", "", "absolute path to the harness CLI (empty = auto-detect)")
	fs.StringVar(&p.configDir, "config-dir", "", "the CLI's configuration directory, which selects the account (empty = its own default)")
	fs.StringVar(&p.defaultModel, "default-model", "", "model used when neither the task nor the caller names one")
}

func (p providerPatch) apply(inst *provider.Instance) {
	if p.set["name"] {
		inst.Name = p.name
	}
	if p.set["path"] {
		inst.BinaryPath = p.path
	}
	if p.set["config-dir"] {
		inst.ConfigDir = p.configDir
	}
	if p.set["default-model"] {
		inst.DefaultModel = p.defaultModel
	}
}

func cmdProviderAdd(st *store.Store, args []string) error {
	id, rest, err := splitPositional(args, "a provider id")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("provider add", flag.ContinueOnError)
	var p providerPatch
	p.register(fs, true)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if p.set, err = passedFlags(fs); err != nil {
		return err
	}
	if p.kind == "" {
		return fmt.Errorf("--kind is required")
	}
	inst := provider.Instance{ID: id, Kind: provider.Kind(p.kind), Name: id, Enabled: true}
	p.apply(&inst)
	if inst.Name == "" {
		inst.Name = id
	}
	if err := app.AddProvider(st, adapters.Default(), inst); err != nil {
		return err
	}
	fmt.Printf("added provider %q\n", id)
	return nil
}

func cmdProviderEdit(st *store.Store, args []string) error {
	id, rest, err := splitPositional(args, "a provider id")
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("provider edit", flag.ContinueOnError)
	var p providerPatch
	p.register(fs, false)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if p.set, err = passedFlags(fs); err != nil {
		return err
	}
	if len(p.set) == 0 {
		return fmt.Errorf("no changes given")
	}
	if err := app.EditProvider(st, adapters.Default(), id, func(inst *provider.Instance) error {
		p.apply(inst)
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("updated provider %q\n", id)
	return nil
}
