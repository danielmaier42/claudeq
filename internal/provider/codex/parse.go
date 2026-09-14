package codex

import (
	"encoding/json"
	"strings"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// NewParser implements provider.Adapter.
func (a *Adapter) NewParser() provider.Parser { return &parser{} }

// streamEvent is one line of `codex exec --json`. The stream is a sequence of
// thread/turn/item records; claudeq reads the few that decide the outcome and
// ignores the rest, so a new record type the CLI adds later is harmless.
type streamEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id"`
	Message  string      `json:"message"`
	Item     *streamItem `json:"item"`
	Usage    *usage      `json:"usage"`
	Error    *streamErr  `json:"error"`
}

// streamItem is the payload of item.started / item.completed. `agent_message`
// carries what the run has to say; `error` carries a problem, which is not
// always terminal (a model-metadata warning arrives this way); `collab_tool_call`
// is Codex coordinating its own subagents, which claudeq deliberately ignores.
type streamItem struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Message string `json:"message"`
}

type streamErr struct {
	Message string `json:"message"`
}

// usage is what Codex reports for a completed turn. It has no monetary cost, and
// claudeq does not invent one.
type usage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	ReasoningTokens   int `json:"reasoning_output_tokens"`
}

// parser translates Codex's JSONL stream into normalized events. It is per-run
// because the outcome is spread across several lines: the last agent message is
// the run's answer, and the failure that ends a turn is usually explained by an
// `error` record that arrived earlier.
type parser struct {
	lastMessage string
	failure     string
}

// Parse implements provider.Parser. A line that is not JSON is stderr noise or a
// partial write and is ignored; the run log keeps it verbatim either way.
func (p *parser) Parse(line []byte) []provider.Event {
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil
	}

	switch ev.Type {
	case "thread.started":
		if ev.ThreadID == "" {
			return nil
		}
		return []provider.Event{{Type: provider.EventSessionStarted, SessionID: ev.ThreadID}}

	case "item.started", "item.completed":
		p.noteItem(ev.Item)
		return nil

	case "error":
		p.noteFailure(ev.Message)
		return nil

	case "turn.completed":
		return []provider.Event{{
			Type:        provider.EventCompleted,
			FinalOutput: p.lastMessage,
			Metrics:     metricsOf(ev.Usage),
		}}

	case "turn.failed":
		if ev.Error != nil {
			p.noteFailure(ev.Error.Message)
		}
		return []provider.Event{p.terminalFailure()}
	}
	return nil
}

// noteItem records what an item contributes: the agent's own words, or a problem
// it reported. A collaboration record means Codex is talking to its own
// subagents — that is the harness's business, not claudeq's.
func (p *parser) noteItem(item *streamItem) {
	if item == nil {
		return
	}
	switch item.Type {
	case "agent_message":
		if text := strings.TrimSpace(item.Text); text != "" {
			p.lastMessage = text
		}
	case "error":
		p.noteFailure(item.Message)
	}
}

// noteFailure keeps the *last* problem reported. Codex narrates a failure as it
// unfolds — a transport warning, then a retry, then the status that actually
// ended it — so the latest line is the one that explains the outcome, unlike a
// terminal event which merely repeats it.
func (p *parser) noteFailure(msg string) {
	if msg = strings.TrimSpace(msg); msg != "" {
		p.failure = msg
	}
}

// terminalFailure classifies the reason a turn ended badly. The order matters:
// an authentication problem outranks everything else, and a rate limit outranks
// a plain failure because the run can be picked up again later.
func (p *parser) terminalFailure() provider.Event {
	detail := p.failure
	switch {
	case isAuthFailure(detail):
		return provider.Event{Type: provider.EventAuthFailed, Detail: summarize(detail)}
	case isRateLimit(detail):
		// The 429 the spike captured carried neither a reset time nor a
		// Retry-After value, so no timing is reported and the engine falls back
		// to its own backoff for this provider instance.
		return provider.Event{Type: provider.EventRateLimited, Detail: summarize(detail)}
	case isModelRejected(detail):
		return provider.Event{Type: provider.EventModelRejected, Detail: summarize(detail)}
	default:
		return provider.Event{Type: provider.EventFailed, Detail: summarize(detail)}
	}
}

func isAuthFailure(msg string) bool {
	return strings.Contains(msg, "401") || strings.Contains(strings.ToLower(msg), "unauthorized")
}

func isRateLimit(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(msg, "429") || strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "rate_limit_exceeded")
}

// isModelRejected recognises the shape the spike captured: a terminal 400 whose
// message says the model is not supported. A 400 on its own is not enough —
// plenty of other requests can be rejected — so both halves must be present.
func isModelRejected(msg string) bool {
	if !strings.Contains(msg, "400") {
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "model") &&
		(strings.Contains(lower, "not supported") || strings.Contains(lower, "invalid_request_error"))
}

// summarize cuts a Codex error down to something an operator can read in a
// notification. The full text stays in the run log.
func summarize(msg string) string {
	msg = strings.Join(strings.Fields(msg), " ")
	const limit = 300
	if r := []rune(msg); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return msg
}

// metricsOf maps Codex's token counts onto claudeq's. Cached and reasoning
// tokens have no field of their own; cached input is part of the input total
// Codex reports, and reasoning output is counted with the output, so neither is
// added twice.
func metricsOf(u *usage) *provider.Metrics {
	if u == nil {
		return nil
	}
	return &provider.Metrics{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
	}
}
