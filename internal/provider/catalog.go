package provider

import "sync"

// Catalog remembers the model list of each provider instance.
//
// It is keyed by the instance's own settings rather than by its kind, because
// two instances of one harness can be two different installations: a second
// account with its own configuration directory, or a path pointing at another
// build. One answer for all of them would show the wrong models for the second.
//
// Only a real answer is kept. A discovery that fell back — the CLI was not
// installed yet, the debug command failed — is asked again next time, so
// installing the harness while the daemon runs does not leave it showing the
// fallback list until the next restart.
type Catalog struct {
	mu sync.Mutex
	by map[string][]Model
}

// Lookup returns the remembered list for inst, asking discover when there is
// none. discover reports whether its answer is worth keeping.
func (c *Catalog) Lookup(inst Instance, discover func() ([]Model, bool)) []Model {
	key := fingerprint(inst)
	c.mu.Lock()
	if cached, ok := c.by[key]; ok {
		c.mu.Unlock()
		return cached
	}
	c.mu.Unlock()

	models, keep := discover()
	if !keep {
		return models
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.by == nil {
		c.by = map[string][]Model{}
	}
	c.by[key] = models
	return models
}
