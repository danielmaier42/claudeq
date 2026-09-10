package review

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// envelope builds what `claude -p --output-format json` prints, with body as
// the model's answer.
func envelope(t *testing.T, body string) []byte {
	t.Helper()
	out, err := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": body})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// capture records the invocation a review made and answers with body.
type capture struct {
	bin, dir string
	args     []string
	calls    int
}

func (c *capture) runner(body []byte, err error) Runner {
	return func(_ context.Context, bin, dir string, args []string) ([]byte, error) {
		c.calls++
		c.bin, c.dir, c.args = bin, dir, args
		return body, err
	}
}

func TestArgsIsMinimalAndToolFree(t *testing.T) {
	args := Args("opus", "SYSTEM", "MESSAGE")
	joined := strings.Join(args, " ")
	for _, want := range []string{"-p", "--output-format json", "--safe-mode", "--no-session-persistence", "--model opus"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
	// --tools with an empty value removes every tool; the review must never be
	// able to touch the machine itself.
	i := indexOf(args, "--tools")
	if i < 0 || args[i+1] != "" {
		t.Errorf("args %q should disable all tools", args)
	}
	if args[len(args)-1] != "MESSAGE" || args[len(args)-2] != "SYSTEM" {
		t.Errorf("system prompt and message should be the last two args: %q", args)
	}
}

func TestArgsWithoutModel(t *testing.T) {
	if indexOf(Args("", "s", "m"), "--model") >= 0 {
		t.Error("an empty model must not be passed to the CLI at all")
	}
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func TestReviewEmptyPromptCostsNothing(t *testing.T) {
	var c capture
	r := &Reviewer{Bin: "claude", Run: c.runner(nil, nil)}
	res, err := r.Review(context.Background(), Request{Kind: KindTask, Prompt: "   \n  "})
	if err != nil || !res.OK {
		t.Fatalf("got (%+v, %v), want an OK result", res, err)
	}
	if c.calls != 0 {
		t.Errorf("an empty prompt must not start a model call, got %d", c.calls)
	}
}

func TestReviewWithoutBinary(t *testing.T) {
	r := &Reviewer{}
	if _, err := r.Review(context.Background(), Request{Prompt: "do it"}); !errors.Is(err, ErrNoBinary) {
		t.Errorf("got %v, want ErrNoBinary", err)
	}
}

func TestReviewUsesRequestBinaryAndModel(t *testing.T) {
	var c capture
	r := &Reviewer{Bin: "fallback", Home: t.TempDir(), Run: c.runner(envelope(t, `{"ok":true}`), nil)}
	if _, err := r.Review(context.Background(), Request{Prompt: "do it", Bin: "/opt/claude", Model: "haiku"}); err != nil {
		t.Fatal(err)
	}
	if c.bin != "/opt/claude" {
		t.Errorf("bin = %q, want the per-request override", c.bin)
	}
	if i := indexOf(c.args, "--model"); i < 0 || c.args[i+1] != "haiku" {
		t.Errorf("args %q should carry the requested model", c.args)
	}
}

func TestReviewRunsInANeutralDirectory(t *testing.T) {
	home := t.TempDir()
	var c capture
	r := &Reviewer{Bin: "claude", Home: home, Run: c.runner(envelope(t, `{"ok":true}`), nil)}
	// The task's own directory may not exist yet, and the review reads nothing
	// there anyway, so the process must not be started in it.
	if _, err := r.Review(context.Background(), Request{Prompt: "x", WorkingDir: "/nope/missing"}); err != nil {
		t.Fatal(err)
	}
	if c.dir != home {
		t.Errorf("dir = %q, want the home directory %q", c.dir, home)
	}
}

func TestReviewParsesAnswers(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantOK      bool
		wantMessage string
		wantRevised string
	}{
		{"clean", `{"ok":true}`, true, "", ""},
		{"fenced", "```json\n{\"ok\":false,\"message\":\"m\",\"revised_prompt\":\"r\"}\n```", false, "m", "r"},
		{"chatty", "Here you go: {\"ok\":false,\"message\":\"m\"} — hope that helps.", false, "m", ""},
		{"finding without text is no finding", `{"ok":false,"message":"  "}`, true, "", ""},
		{"a revision is ignored when the answer is ok", `{"ok":true,"revised_prompt":"r"}`, true, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c capture
			r := &Reviewer{Bin: "claude", Home: t.TempDir(), Run: c.runner(envelope(t, tc.body), nil)}
			res, err := r.Review(context.Background(), Request{Prompt: "do it"})
			if err != nil {
				t.Fatalf("Review: %v", err)
			}
			if res.OK != tc.wantOK || res.Message != tc.wantMessage || res.RevisedPrompt != tc.wantRevised {
				t.Errorf("got %+v, want ok=%v message=%q revised=%q", res, tc.wantOK, tc.wantMessage, tc.wantRevised)
			}
		})
	}
}

func TestReviewDropsARevisionThatChangesNothing(t *testing.T) {
	var c capture
	body := `{"ok":false,"message":"m","revised_prompt":"do it"}`
	r := &Reviewer{Bin: "claude", Home: t.TempDir(), Run: c.runner(envelope(t, body), nil)}
	res, err := r.Review(context.Background(), Request{Prompt: "do it\n"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RevisedPrompt != "" {
		t.Errorf("revised = %q, want it dropped as a no-op", res.RevisedPrompt)
	}
	if res.Message != "m" {
		t.Errorf("the finding itself should survive, got %q", res.Message)
	}
}

func TestReviewTruncatesALongMessage(t *testing.T) {
	var c capture
	long, _ := json.Marshal(strings.Repeat("word ", 400))
	body := `{"ok":false,"message":` + string(long) + `}`
	r := &Reviewer{Bin: "claude", Home: t.TempDir(), Run: c.runner(envelope(t, body), nil)}
	res, err := r.Review(context.Background(), Request{Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Message) > maxMessage+4 {
		t.Errorf("message is %d chars, want it cut to about %d", len(res.Message), maxMessage)
	}
}

func TestReviewReportsBadOutput(t *testing.T) {
	tests := []struct {
		name string
		out  []byte
	}{
		{"not json at all", []byte("claude: command failed")},
		{"no object in the answer", envelope(t, "sure, looks fine to me")},
		{"broken object", envelope(t, `{"ok":`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c capture
			r := &Reviewer{Bin: "claude", Home: t.TempDir(), Run: c.runner(tc.out, nil)}
			if _, err := r.Review(context.Background(), Request{Prompt: "do it"}); err == nil {
				t.Error("want an error, got none")
			}
		})
	}
}

func TestReviewReportsACLIError(t *testing.T) {
	out, err := json.Marshal(map[string]any{"is_error": true, "subtype": "error_during_execution", "result": "boom"})
	if err != nil {
		t.Fatal(err)
	}
	var c capture
	r := &Reviewer{Bin: "claude", Home: t.TempDir(), Run: c.runner(out, nil)}
	if _, err := r.Review(context.Background(), Request{Prompt: "do it"}); err == nil ||
		!strings.Contains(err.Error(), "error_during_execution") {
		t.Errorf("got %v, want the CLI's error reported", err)
	}
}

func TestReviewHonoursItsTimeout(t *testing.T) {
	r := &Reviewer{Bin: "claude", Home: t.TempDir(), Timeout: 20 * time.Millisecond,
		Run: func(ctx context.Context, _, _ string, _ []string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
	start := time.Now()
	if _, err := r.Review(context.Background(), Request{Prompt: "do it"}); err == nil {
		t.Fatal("want an error when the review runs out of time")
	}
	if time.Since(start) > time.Second {
		t.Errorf("the timeout did not cut the run short (took %s)", time.Since(start))
	}
}
