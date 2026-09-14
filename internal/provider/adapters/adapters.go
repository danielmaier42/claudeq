// Package adapters wires the provider adapters claudeq ships with into one
// registry. Every entry point that runs jobs — the daemon and the CLI's
// `run-now` — builds its executor from this, so a new harness is registered in
// exactly one place and no entry point can be left behind.
package adapters

import (
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/provider/claudecode"
	"github.com/danielmaier42/claudeq/internal/provider/codex"
)

// Default returns a registry holding every adapter this build supports.
//
// Codex is registered unconditionally. Whether Settings *offers* it is a
// presentation preference (store.Settings.ShowCodexBeta); the adapter, the API,
// the CLI and the scheduler never consult it, so a Codex task created from the
// command line runs whatever the app is currently showing.
func Default() *provider.Registry {
	return provider.NewRegistry(claudecode.New(), codex.New())
}
