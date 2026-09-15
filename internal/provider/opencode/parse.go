package opencode

import (
	"encoding/json"
	"strings"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// NewParser implements provider.Adapter.
func (a *Adapter) NewParser() provider.Parser { return &parser{} }

// streamEvent is one line of `opencode run --format json`. The stream is a
// sequence of step/text/tool-use records for one turn, repeated for however
// many turns the run took; claudeq reads the few fields that decide the
// outcome and ignores the rest, so a new record type the CLI adds later is
// harmless.
type streamEvent struct {
	Type      string     `json:"type"`
	SessionID string     `json:"sessionID"`
	Part      *eventPart `json:"part"`
	Error     *streamErr `json:"error"`
}

// eventPart covers `text` (the model's own words) and `step_finish` (which
// ends a turn: "stop" is the run's actual answer, "tool-calls" means another
// step follows). Nothing about tool_use is read — claudeq does not need the
// per-call detail, only the run's final outcome.
type eventPart struct {
	Type   string      `json:"type"`
	Text   string      `json:"text"`
	Reason string      `json:"reason"`
	Tokens *tokenUsage `json:"tokens"`
	Cost   *float64    `json:"cost"`
}

type tokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

type streamErr struct {
	Name string `json:"name"`
	Data struct {
		Message string `json:"message"`
	} `json:"data"`
}

// parser translates opencode's JSONL stream into normalized events. It is
// per-run because the answer and its usage figures arrive on separate lines
// (the last `text` before the terminal `step_finish`), and because a run can
// take several turns before the one that actually ends it.
type parser struct {
	lastMessage string
	metrics     *provider.Metrics
}

// Parse implements provider.Parser. A line that is not JSON is stderr noise or
// a partial write and is ignored; the run log keeps it verbatim either way.
//
// opencode has no event that marks the whole run complete, only a
// `step_finish` per turn — its own `reason` field says whether another turn
// follows ("tool-calls") or the run is actually done ("stop"), observed
// running the CLI directly. There is no explicit success/failure flag on it;
// a `step_finish` with reason "stop" is read as success because nothing else
// in what was observed ever contradicted that, and a run that instead failed
// exits non-zero after an `error` line, which the collector treats as failed
// regardless of whether a completion was also seen.
func (p *parser) Parse(line []byte) []provider.Event {
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil
	}

	var out []provider.Event
	if ev.SessionID != "" {
		out = append(out, provider.Event{Type: provider.EventSessionStarted, SessionID: ev.SessionID})
	}

	switch ev.Type {
	case "text":
		if ev.Part != nil {
			if t := strings.TrimSpace(ev.Part.Text); t != "" {
				p.lastMessage = t
			}
		}
	case "step_finish":
		if ev.Part == nil {
			break
		}
		p.noteUsage(ev.Part)
		if ev.Part.Reason == "stop" {
			out = append(out, provider.Event{
				Type:        provider.EventCompleted,
				FinalOutput: p.lastMessage,
				Metrics:     p.metrics,
			})
		}
	case "error":
		// Every failure observed running the CLI directly — an unknown model,
		// an unauthenticated cloud provider — came back in this same generic
		// shape, with nothing in the payload that told one apart from another.
		// So unlike Codex or Claude Code, this adapter cannot classify a
		// failure as an authentication problem or a rate limit; every one of
		// them is reported as a plain failure.
		out = append(out, provider.Event{Type: provider.EventFailed, Detail: summarize(errorDetail(ev.Error))})
	}
	return out
}

// noteUsage keeps the latest usage figures a step_finish reported. A run that
// takes several turns updates this on each one; the last update before the
// terminal step_finish is what claudeq reports for the whole run, because
// opencode does not sum turns itself.
func (p *parser) noteUsage(part *eventPart) {
	if part.Tokens == nil && part.Cost == nil {
		return
	}
	m := &provider.Metrics{}
	if p.metrics != nil {
		m = &provider.Metrics{InputTokens: p.metrics.InputTokens, OutputTokens: p.metrics.OutputTokens, CostUSD: p.metrics.CostUSD}
	}
	if part.Tokens != nil {
		m.InputTokens, m.OutputTokens = part.Tokens.Input, part.Tokens.Output
	}
	if part.Cost != nil {
		m.CostUSD = *part.Cost
	}
	p.metrics = m
}

func errorDetail(e *streamErr) string {
	if e == nil {
		return ""
	}
	if msg := strings.TrimSpace(e.Data.Message); msg != "" {
		return msg
	}
	return e.Name
}

// summarize cuts an opencode error down to something an operator can read in a
// notification. The full text stays in the run log.
func summarize(msg string) string {
	msg = strings.Join(strings.Fields(msg), " ")
	const limit = 300
	if r := []rune(msg); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return msg
}
