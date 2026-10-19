package codex

import (
	"slices"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// TestCodexClaimsAsides: the capability and the method have to agree, or a
// provider would be offered for prompt review and then fail every time.
func TestCodexClaimsAsides(t *testing.T) {
	a := New()
	if !a.Capabilities().Asides {
		t.Fatal("Codex builds aside commands but does not claim the capability")
	}
	if _, err := a.AsideCommand(provider.Instance{}, provider.AsideRequest{Text: "hi"}); err != nil {
		t.Fatalf("AsideCommand: %v", err)
	}
}

func TestAsideCommand(t *testing.T) {
	a := &Adapter{detect: func() string { return "/bin/codex" }}
	inst := provider.Instance{ConfigDir: "/acct"}

	tests := []struct {
		name    string
		req     provider.AsideRequest
		want    []string
		without []string
	}{
		{
			name: "one question leaves no session and states the schema",
			req:  provider.AsideRequest{Text: "check", System: "be brief", Schema: `{"type":"object"}`, Model: "gpt-x", SessionID: "ours"},
			want: []string{"exec", "--json", "--skip-git-repo-check", "--ignore-user-config", "--ignore-rules",
				"--sandbox", "read-only", "--ephemeral", "--model", "gpt-x", "-"},
			without: []string{"resume", "ours"},
		},
		{
			name:    "a conversation that continues is kept",
			req:     provider.AsideRequest{Text: "first", System: "s", Continues: true},
			want:    []string{"--sandbox", "read-only"},
			without: []string{"--ephemeral", "resume"},
		},
		{
			name:    "a resume sends the thread id and no instructions",
			req:     provider.AsideRequest{Text: "next", System: "ignored", SessionID: "thread-1", Resume: true, Continues: true},
			want:    []string{"exec", "resume", "-c", `sandbox_mode="read-only"`, "thread-1", "-"},
			without: []string{"--sandbox", "--ephemeral"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := a.AsideCommand(inst, tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Path != "/bin/codex" || cmd.Stdin != tt.req.Text {
				t.Errorf("path %q stdin %q", cmd.Path, cmd.Stdin)
			}
			if !slices.Contains(cmd.Env, ConfigDirEnv+"=/acct") {
				t.Errorf("env %v lacks the instance's account directory", cmd.Env)
			}
			for _, w := range tt.want {
				if !slices.Contains(cmd.Args, w) {
					t.Errorf("args %v lack %q", cmd.Args, w)
				}
			}
			for _, w := range tt.without {
				if slices.Contains(cmd.Args, w) {
					t.Errorf("args %v contain %q", cmd.Args, w)
				}
			}
			instr := ""
			for i, arg := range cmd.Args {
				if arg == "-c" && strings.HasPrefix(cmd.Args[i+1], developerInstructionsKey+"=") {
					instr = cmd.Args[i+1]
				}
			}
			if tt.req.Resume != (instr == "") {
				t.Errorf("developer instructions %q on resume=%v", instr, tt.req.Resume)
			}
			if tt.req.Schema != "" && !strings.Contains(instr, "JSON schema") {
				t.Errorf("instructions %q do not state the schema", instr)
			}
		})
	}
}

func TestAsideCommandRefuses(t *testing.T) {
	a := New()
	if _, err := a.AsideCommand(provider.Instance{}, provider.AsideRequest{Text: "  "}); err == nil {
		t.Error("an empty question must be refused")
	}
	if _, err := a.AsideCommand(provider.Instance{}, provider.AsideRequest{Text: "x", Resume: true}); err == nil {
		t.Error("a resume without a thread id must be refused")
	}
}

// The streams below are what Codex 0.162 printed for a real aside.
func TestParseAside(t *testing.T) {
	first := `{"type":"thread.started","thread_id":"01a1"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"{\"word\":\"banana\"}"}}
{"type":"turn.completed","usage":{"input_tokens":25160,"output_tokens":9}}
`
	resumed := `{"type":"turn.started"}
{"type":"error","message":"model metadata not found"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"second"}}
{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}
`
	failed := `{"type":"thread.started","thread_id":"01a2"}
{"type":"error","message":"stream disconnected"}
{"type":"turn.failed","error":{"message":"usage limit reached"}}
`
	a := New()

	got, err := a.ParseAside([]byte(first))
	if err != nil || got.SessionID != "01a1" || got.Text != `{"word":"banana"}` {
		t.Errorf("first turn = %+v, %v", got, err)
	}
	got, err = a.ParseAside([]byte(resumed))
	if err != nil || got.SessionID != "" || got.Text != "second" {
		t.Errorf("resumed turn = %+v, %v; a warning before the answer is not a failure", got, err)
	}
	if _, err := a.ParseAside([]byte(failed)); err == nil || !strings.Contains(err.Error(), "usage limit reached") {
		t.Errorf("failed turn error = %v", err)
	}
	if _, err := a.ParseAside([]byte("not json\n")); err == nil {
		t.Error("output without an answer must be an error")
	}
}
