package api

// Model is a selectable Claude model.
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// knownModels is the curated list offered in the UI. Update as new models ship.
// An empty ID elsewhere means "use the global / Claude default".
var knownModels = []Model{
	{ID: "claude-opus-4-8", Label: "Opus 4.8 — most capable"},
	{ID: "claude-sonnet-5", Label: "Sonnet 5 — balanced"},
	{ID: "claude-haiku-4-5-20251001", Label: "Haiku 4.5 — fastest"},
	{ID: "claude-fable-5", Label: "Fable 5"},
}
