package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// AsideCommand implements provider.Adapter. It is the narrowest invocation
// dispatchq makes of Codex, and its shape was established by running Codex 0.162:
// --ignore-user-config and --ignore-rules leave the operator's MCP servers,
// profiles and exec policies out so the answer cannot be steered by them (auth
// still comes from the instance's directory), a read-only sandbox in the empty
// directory the runner provides leaves the agent nothing to touch, and --json
// returns the event stream the run parser already reads.
//
// Codex names its sessions itself: the first turn ignores dispatchq's id and the
// answer reports the thread id, which a later turn resumes with `codex exec
// resume`. The system prompt goes in as developer instructions on the first turn
// only; the thread records it and replays it on resume, which the spike
// confirmed. A one-question aside adds --ephemeral, so it leaves nothing in the
// operator's session history.
//
// Codex enforces a schema only through a file and only in OpenAI's strict
// dialect, so the schema is stated in the instructions instead and the callers
// pull the object out of the answer, as they do for any harness.
func (a *Adapter) AsideCommand(inst provider.Instance, req provider.AsideRequest) (provider.Command, error) {
	if strings.TrimSpace(req.Text) == "" {
		return provider.Command{}, errors.New("codex: an aside needs something to say")
	}
	if req.Resume && req.SessionID == "" {
		return provider.Command{}, errors.New("codex: continuing an aside needs its session id")
	}

	args := []string{"exec"}
	if req.Resume {
		args = append(args, "resume")
	}
	args = append(args, "--json", "--skip-git-repo-check", "--ignore-user-config", "--ignore-rules")
	// `codex exec resume` does not take --sandbox (see Command).
	if req.Resume {
		args = append(args, "-c", sandboxModeKey+"="+tomlString("read-only"))
	} else {
		args = append(args, "--sandbox", "read-only")
		if !req.Continues {
			args = append(args, "--ephemeral")
		}
		if instructions := asideInstructions(req.System, req.Schema); instructions != "" {
			args = append(args, "-c", developerInstructionsKey+"="+tomlString(instructions))
		}
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Resume {
		args = append(args, req.SessionID)
	}
	args = append(args, "-")

	return provider.Command{
		Path:  a.binaryOr(inst),
		Args:  args,
		Env:   a.env(inst),
		Stdin: req.Text,
	}, nil
}

// asideInstructions is the system prompt plus, when the caller wants a shape,
// the schema the answer has to keep to.
func asideInstructions(system, schema string) string {
	system = strings.TrimSpace(system)
	if schema = strings.TrimSpace(schema); schema == "" {
		return system
	}
	shape := "Answer with exactly one JSON object that satisfies this JSON schema, and nothing else:\n" + schema
	if system == "" {
		return shape
	}
	return system + "\n\n" + shape
}

// ParseAside implements provider.Adapter. It reads the same JSONL stream a run
// produces: the thread id from thread.started (absent on a resume, where the
// caller keeps the id it sent), the last agent message as the answer, and a
// failed turn or a stream error as the reason there is none.
func (a *Adapter) ParseAside(out []byte) (provider.Aside, error) {
	var (
		res       provider.Aside
		failure   string
		completed bool
	)
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var ev streamEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "thread.started":
			res.SessionID = ev.ThreadID
		case "item.completed":
			if ev.Item != nil && ev.Item.Type == "agent_message" && strings.TrimSpace(ev.Item.Text) != "" {
				res.Text = strings.TrimSpace(ev.Item.Text)
			}
		case "error":
			if msg := strings.TrimSpace(ev.Message); msg != "" {
				failure = msg
			}
		case "turn.failed":
			if ev.Error != nil && strings.TrimSpace(ev.Error.Message) != "" {
				failure = strings.TrimSpace(ev.Error.Message)
			}
			if failure == "" {
				failure = "the turn failed"
			}
			return provider.Aside{}, fmt.Errorf("codex reported an error: %s", failure)
		case "turn.completed":
			completed = true
		}
	}
	if err := sc.Err(); err != nil {
		return provider.Aside{}, fmt.Errorf("read codex output: %w", err)
	}
	if !completed || res.Text == "" {
		if failure != "" {
			return provider.Aside{}, fmt.Errorf("codex reported an error: %s", failure)
		}
		return provider.Aside{}, errors.New("codex returned no answer")
	}
	return res, nil
}
