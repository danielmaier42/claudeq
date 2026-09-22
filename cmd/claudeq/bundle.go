// Sharing tasks as files: `claudeq export` writes a task to a .claudeq bundle
// (a zip with task.json + prompt.md), `claudeq import` adds one from such a
// file — settings taken as they are, to be adjusted with `claudeq edit`.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/bundle"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func cmdExport(st *store.Store, args []string) error {
	id, rest, err := splitPositional(args, "a task id")
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	out := flags.String("out", "", "file or directory to write to (default: ./ID.claudeq)")
	force := flags.Bool("force", false, "overwrite an existing file")
	if err := flags.Parse(rest); err != nil {
		return err
	}

	var buf bytes.Buffer
	t, err := app.ExportTask(st, id, &buf, time.Now())
	if err != nil {
		return err
	}
	path := exportPath(*out, bundle.FileName(t))
	if err := bundle.Save(path, buf.Bytes(), *force); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		}
		return err
	}
	fmt.Printf("exported task %q to %s\n", t.ID, path)
	return nil
}

// exportPath resolves --out: empty means the default name in the current
// directory, an existing directory gets the default name inside it, anything
// else is the file itself (with the .claudeq extension added if missing).
func exportPath(out, defaultName string) string {
	if out == "" {
		return defaultName
	}
	if fi, err := os.Stat(out); err == nil && fi.IsDir() {
		return filepath.Join(out, defaultName)
	}
	p, _ := bundle.EnsureExt(out)
	return p
}

// applyImportProvider decides what the imported task runs on: the operator's
// override, otherwise the local instance the file's hint resolves to.
//
// A hint that matches nothing here, or matches several accounts, is not guessed
// at — importing a task onto the wrong account spends the wrong allowance, and
// may be the wrong employer's. The command says what the file wants and how to
// answer it, rather than queueing something that cannot run as intended.
func applyImportProvider(st *store.Store, t *task.Task, hint bundle.ProviderHint, providerOverride, modelOverride string) error {
	if t.IsScript() {
		if providerOverride != "" || modelOverride != "" {
			return fmt.Errorf("this file holds a script job: it runs no model, so --provider and --model do not apply")
		}
		return nil
	}
	switch {
	case providerOverride != "":
		t.Provider = providerOverride
		// A model from the file was chosen for another harness; without an
		// explicit one the new provider's default is the honest answer.
		t.Model = modelOverride
		return nil
	case modelOverride != "":
		t.Model = modelOverride
	}
	match, ok := app.ResolveHint(st, hint)
	if !ok {
		return fmt.Errorf("this task was exported for %s, which does not match exactly one provider configured here; "+
			"name one with --provider ID (claudeq provider list)", hintName(hint))
	}
	t.Provider = match.Provider
	if modelOverride == "" && match.Model != "" {
		t.Model = match.Model
	}
	return nil
}

// hintName is how an unresolved provider hint is named on the command line.
func hintName(hint bundle.ProviderHint) string {
	if hint.ProviderName != "" && hint.ProviderName != hint.Kind {
		return fmt.Sprintf("%q (%s)", hint.ProviderName, hint.Kind)
	}
	return hint.Kind
}

func cmdImport(st *store.Store, args []string) error {
	path, rest, err := splitPositional(args, "the path of a .claudeq file")
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	idOverride := flags.String("id", "", "use this id instead of the one in the file")
	providerOverride := flags.String("provider", "", "run the imported task on this provider instance")
	modelOverride := flags.String("model", "", "run the imported task with this model")
	if err := flags.Parse(rest); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("usage: claudeq import PATH [--id ID] [--provider ID] [--model NAME]")
	}
	t, hint, err := bundle.Load(path)
	if err != nil {
		return err
	}
	wanted := t.ID
	if *idOverride != "" {
		wanted = *idOverride
		t.ID = wanted
	}
	if err := applyImportProvider(st, &t, hint, *providerOverride, *modelOverride); err != nil {
		return err
	}
	// Whatever the task ended up pointing at still has to be able to run it.
	if !t.IsScript() {
		if err := ensureRunnable(st, t.Provider); err != nil {
			return err
		}
	}
	t, err = app.ImportTask(st, t)
	if err != nil {
		return err
	}
	if wanted != "" && t.ID != wanted {
		fmt.Printf("imported task %q (id %q was already taken)\n", t.ID, wanted)
	} else {
		fmt.Printf("imported task %q\n", t.ID)
	}
	// The working directory comes from the machine the task was exported on, so
	// it may not exist here — the task would fail at run time without a hint.
	if !app.DirExists(t.WorkingDir) {
		fmt.Fprintf(os.Stderr, "warning: working directory %q does not exist here; set it with: claudeq edit %s --dir PATH\n", t.WorkingDir, t.ID)
	}
	return nil
}
