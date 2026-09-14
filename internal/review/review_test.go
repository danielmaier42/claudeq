package review

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// capture records the aside a review made and answers with text.
type capture struct {
	inst  provider.Instance
	req   provider.AsideRequest
	calls int
	text  string
	err   error
}

func (c *capture) Ask(_ context.Context, inst provider.Instance, req provider.AsideRequest) (provider.Aside, error) {
	c.calls++
	c.inst, c.req = inst, req
	return provider.Aside{Text: c.text}, c.err
}

// reviewer returns a Reviewer answering with body, and the capture behind it.
func reviewer(t *testing.T, body string) (*Reviewer, *capture) {
	t.Helper()
	c := &capture{text: body}
	return &Reviewer{Home: t.TempDir(), Ask: c}, c
}

// claude is the instance a review is pointed at.
var claude = provider.Instance{ID: "claude", Kind: provider.KindClaudeCode, Name: "Claude", Enabled: true}

func TestReviewEmptyPromptCostsNothing(t *testing.T) {
	r, c := reviewer(t, `{"ok":true}`)
	res, err := r.Review(context.Background(), Request{Kind: KindTask, Prompt: "   \n  ", Provider: claude})
	if err != nil || !res.OK {
		t.Fatalf("got (%+v, %v), want an OK result", res, err)
	}
	if c.calls != 0 {
		t.Errorf("an empty prompt must not start a model call, got %d", c.calls)
	}
}

// TestReviewWithoutAProvider: no provider means no review, reported as
// unavailable rather than as a finding about the prompt.
func TestReviewWithoutAProvider(t *testing.T) {
	r, _ := reviewer(t, "")
	if _, err := r.Review(context.Background(), Request{Prompt: "do it"}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
	bare := &Reviewer{}
	if _, err := bare.Review(context.Background(), Request{Prompt: "do it", Provider: claude}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
}

// TestReviewAsksTheChosenProvider: the instance and model come from the caller,
// and the question is a one-off — it keeps no session to be resumed.
func TestReviewAsksTheChosenProvider(t *testing.T) {
	r, c := reviewer(t, `{"ok":true}`)
	other := provider.Instance{ID: "claude-work", Kind: provider.KindClaudeCode, Enabled: true}
	if _, err := r.Review(context.Background(), Request{Prompt: "do it", Model: "haiku", Provider: other}); err != nil {
		t.Fatal(err)
	}
	if c.inst.ID != "claude-work" {
		t.Errorf("asked %q, want the requested instance", c.inst.ID)
	}
	if c.req.Model != "haiku" {
		t.Errorf("model = %q, want haiku", c.req.Model)
	}
	if c.req.Continues || c.req.Resume {
		t.Error("a review is one question; it must not open a conversation")
	}
	if c.req.System == "" || !strings.Contains(c.req.Text, "do it") {
		t.Errorf("the review must send its instructions and the prompt: %+v", c.req)
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
			r, _ := reviewer(t, tc.body)
			res, err := r.Review(context.Background(), Request{Prompt: "do it", Provider: claude})
			if err != nil {
				t.Fatalf("Review: %v", err)
			}
			if res.OK != tc.wantOK || res.Message != tc.wantMessage || res.RevisedPrompt != tc.wantRevised {
				t.Errorf("got %+v, want ok=%v message=%q revised=%q", res, tc.wantOK, tc.wantMessage, tc.wantRevised)
			}
		})
	}
}

// TestReviewPrefersAValidatedAnswer: when the harness checked the shape itself,
// that is the answer — not whatever prose surrounds it.
func TestReviewPrefersAValidatedAnswer(t *testing.T) {
	r := &Reviewer{Home: t.TempDir(), Ask: asideFunc(func() provider.Aside {
		return provider.Aside{
			Text:       "ignore me {\"ok\":true}",
			Structured: json.RawMessage(`{"ok":false,"message":"real finding"}`),
		}
	})}
	res, err := r.Review(context.Background(), Request{Prompt: "do it", Provider: claude})
	if err != nil {
		t.Fatal(err)
	}
	if res.Message != "real finding" {
		t.Errorf("message = %q, want the validated answer", res.Message)
	}
}

// asideFunc adapts a plain function to Asker.
type asideFunc func() provider.Aside

func (f asideFunc) Ask(context.Context, provider.Instance, provider.AsideRequest) (provider.Aside, error) {
	return f(), nil
}

func TestReviewDropsARevisionThatChangesNothing(t *testing.T) {
	r, _ := reviewer(t, `{"ok":false,"message":"m","revised_prompt":"do it"}`)
	res, err := r.Review(context.Background(), Request{Prompt: "do it\n", Provider: claude})
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
	long, _ := json.Marshal(strings.Repeat("word ", 400))
	r, _ := reviewer(t, `{"ok":false,"message":`+string(long)+`}`)
	res, err := r.Review(context.Background(), Request{Prompt: "do it", Provider: claude})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Message) > maxMessage+4 {
		t.Errorf("message is %d chars, want it cut to about %d", len(res.Message), maxMessage)
	}
}

func TestReviewReportsBadOutput(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"no object in the answer", "sure, looks fine to me"},
		{"broken object", `{"ok":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := reviewer(t, tc.text)
			if _, err := r.Review(context.Background(), Request{Prompt: "do it", Provider: claude}); err == nil {
				t.Error("want an error, got none")
			}
		})
	}
}

// TestReviewReportsTheHarnessesError: what the harness said went wrong is what
// the operator is told.
func TestReviewReportsTheHarnessesError(t *testing.T) {
	r, c := reviewer(t, "")
	c.err = errors.New("claude reported an error: boom")
	if _, err := r.Review(context.Background(), Request{Prompt: "do it", Provider: claude}); err == nil ||
		!strings.Contains(err.Error(), "boom") {
		t.Errorf("got %v, want the harness's error reported", err)
	}
}

func TestReviewHonoursItsTimeout(t *testing.T) {
	r := &Reviewer{Home: t.TempDir(), Timeout: 20 * time.Millisecond, Ask: blockingAsker{}}
	start := time.Now()
	if _, err := r.Review(context.Background(), Request{Prompt: "do it", Provider: claude}); err == nil {
		t.Fatal("want an error when the review runs out of time")
	}
	if time.Since(start) > time.Second {
		t.Errorf("the timeout did not cut the run short (took %s)", time.Since(start))
	}
}

type blockingAsker struct{}

func (blockingAsker) Ask(ctx context.Context, _ provider.Instance, _ provider.AsideRequest) (provider.Aside, error) {
	<-ctx.Done()
	return provider.Aside{}, ctx.Err()
}
