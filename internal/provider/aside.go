package provider

import "encoding/json"

// An aside is a question claudeq asks a harness on its own behalf rather than
// to carry out the operator's work: the prompt review that checks a task before
// it is queued, and the feedback assistant that drafts a GitHub issue.
//
// It is not a small run. A run does the user's work in the user's directory
// with the user's tools; an aside is claudeq talking to a model about claudeq,
// so it gets no tools, no project configuration, and a directory of its own.
// Keeping the two apart is what stops a review of a prompt from being steered
// by the repository the prompt talks about.

// AsideRequest is one turn of such a conversation.
type AsideRequest struct {
	// Model is the model to answer with. Empty leaves the choice to the harness.
	Model string
	// System is the instruction set for the whole conversation. Harnesses record
	// it when a session starts and replay it on every resume, so it belongs to
	// the first turn; later turns leave it empty.
	System string
	// Text is what to say this turn.
	Text string
	// Schema is a JSON schema the answer must satisfy. Adapters that cannot
	// enforce one state the shape in System instead and parse what comes back.
	Schema string
	// SessionID identifies the conversation. Claudeq issues it when the harness
	// accepts one; otherwise the adapter reports the harness's own id in the
	// answer and the caller sends that back.
	SessionID string
	// Resume continues the named session instead of starting it.
	Resume bool
}

// Aside is what a harness answered.
type Aside struct {
	// SessionID is the conversation to send the next turn to. It may differ from
	// the requested one when the harness names its sessions itself.
	SessionID string
	// Text is the answer as the model wrote it.
	Text string
	// Structured is the answer as an object, set only when the harness itself
	// validated it against the requested schema. A caller that asked for JSON
	// still has to be able to read it out of Text.
	Structured json.RawMessage
}
