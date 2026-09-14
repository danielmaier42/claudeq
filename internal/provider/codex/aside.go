package codex

import (
	"fmt"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// AsideCommand implements provider.Adapter. Codex does not answer claudeq's own
// questions yet, so the capability is not claimed and this reports it.
//
// The mechanics look close enough — `codex exec --json` with a read-only
// sandbox, developer instructions, and `codex exec resume` for a second turn —
// but an aside additionally needs no tools at all, a neutral directory outside
// any repository, and an answer that keeps to a schema. None of those three was
// measured in the spike, and this package does not guess at a CLI's behaviour:
// everything it does to Codex was established by running Codex. Until an aside
// is spiked the same way, prompt review and feedback stay on the providers that
// have been shown to deliver them, and say so rather than failing at the point
// of use.
func (a *Adapter) AsideCommand(provider.Instance, provider.AsideRequest) (provider.Command, error) {
	return provider.Command{}, fmt.Errorf("%w: Codex cannot answer claudeq's own questions yet", provider.ErrUnsupported)
}

// ParseAside implements provider.Adapter. Nothing produces output to parse
// while AsideCommand refuses.
func (a *Adapter) ParseAside([]byte) (provider.Aside, error) {
	return provider.Aside{}, fmt.Errorf("%w: Codex cannot answer claudeq's own questions yet", provider.ErrUnsupported)
}
