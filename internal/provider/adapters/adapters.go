// Package adapters wires the provider adapters claudeq ships with into one
// registry. Every entry point that runs jobs — the daemon and the CLI's
// `run-now` — builds its executor from this, so a new harness is registered in
// exactly one place and no entry point can be left behind.
package adapters

import (
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/provider/claudecode"
)

// Default returns a registry holding every adapter this build supports.
func Default() *provider.Registry {
	return provider.NewRegistry(claudecode.New())
}
