package provider

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/danielmaier42/claudeq/internal/store"
)

// modelScanLines bounds how far into a run log the backfill looks for the
// harness naming its model. It does so on its opening lines; a log that has not
// by then never will, and reading every log whole would slow each start.
const modelScanLines = 64

// BackfillReportedModels fills in RunProvider.ReportedModel for finished runs
// recorded before dispatchq kept it, by reading the model back out of each run's
// log with its harness's own parser. It is meant to run at startup, before the
// engine appends anything. A run whose log names no model is left alone and
// looked at again next start, which costs a few lines of reading. A log that
// cannot be read is skipped and reported in the returned error. Returns how
// many runs it filled in.
func BackfillReportedModels(st *store.Store, reg *Registry) (int, error) {
	runs, err := st.Runs()
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, r := range runs {
		if r.Provider.ReportedModel != "" || !r.Status.Terminal() || r.LogPath == "" {
			continue
		}
		a, err := reg.Lookup(Kind(r.Provider.Kind))
		if err != nil {
			continue // a script run, or a harness this build no longer has
		}
		model, err := reportedModel(r.LogPath, a.NewParser())
		if err != nil {
			// One unreadable log must not keep the runs after it unfilled; it
			// is reported with the rest once the pass is through.
			errs = append(errs, fmt.Errorf("read model of run %s: %w", r.RunID, err))
			continue
		}
		if model == "" {
			continue
		}
		r.Provider.ReportedModel = model
		if err := st.AppendRun(r); err != nil {
			return n, err
		}
		n++
	}
	return n, errors.Join(errs...)
}

// reportedModel returns the model the run log's opening lines name, or "" when
// they name none or the log is gone.
func reportedModel(path string, p Parser) (string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	return scanModel(f, p)
}

func scanModel(r io.Reader, p Parser) (string, error) {
	c := NewCollector("", "")
	sc := bufio.NewScanner(r)
	// The init line lists every tool and skill, so it can run long.
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for i := 0; i < modelScanLines && sc.Scan(); i++ {
		c.AddAll(p.Parse(sc.Bytes()))
		if c.model != "" {
			return c.model, nil
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return "", err
	}
	return "", nil
}
