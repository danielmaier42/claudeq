package opencode

import (
	"fmt"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// AsideCommand implements provider.Adapter. opencode does not answer
// claudeq's own questions yet, so the capability is not claimed and this
// reports it.
//
// `opencode run` has no equivalent of Claude Code's --safe-mode --tools "": no
// flag observed running the CLI directly strips tools or configuration for one
// narrow, schema-constrained turn. Until that is found or built, prompt
// review and the feedback assistant stay on the providers that have been
// shown to deliver them.
func (a *Adapter) AsideCommand(provider.Instance, provider.AsideRequest) (provider.Command, error) {
	return provider.Command{}, fmt.Errorf("%w: opencode cannot answer claudeq's own questions yet", provider.ErrUnsupported)
}

// ParseAside implements provider.Adapter. Nothing produces output to parse
// while AsideCommand refuses.
func (a *Adapter) ParseAside([]byte) (provider.Aside, error) {
	return provider.Aside{}, fmt.Errorf("%w: opencode cannot answer claudeq's own questions yet", provider.ErrUnsupported)
}
