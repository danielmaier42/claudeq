// Package adapters wires the provider adapters claudeq ships with into one
// registry. Every entry point that runs jobs — the daemon and the CLI's
// `run-now` — builds its executor from this, so a new harness is registered in
// exactly one place and no entry point can be left behind.
package adapters

import (
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/provider/claudecode"
	"github.com/danielmaier42/claudeq/internal/provider/codex"
	"github.com/danielmaier42/claudeq/internal/provider/opencode"
)

// Default returns a registry holding every adapter this build supports.
//
// Codex and opencode are registered unconditionally. Whether Settings
// *offers* a beta kind is a presentation preference (store.Settings.
// BetaFeatures); the adapter, the API, the CLI and the scheduler never
// consult it, so a task created from the command line runs whatever the app
// is currently showing.
func Default() *provider.Registry {
	return provider.NewRegistry(claudecode.New(), codex.New(), opencode.New())
}
