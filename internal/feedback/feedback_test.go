package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// claude is the instance the feedback assistant is pointed at.
var claude = provider.Instance{ID: "claude", Kind: provider.KindClaudeCode, Name: "Claude", Enabled: true}

// stubAsker records the asides made and replays canned answers.
type stubAsker struct {
	answers []provider.Aside
	err     error
	reqs    []provider.AsideRequest
	insts   []provider.Instance
}

func (s *stubAsker) Ask(_ context.Context, inst provider.Instance, req provider.AsideRequest) (provider.Aside, error) {
	s.reqs = append(s.reqs, req)
	s.insts = append(s.insts, inst)
	if s.err != nil {
		return provider.Aside{}, s.err
	}
	if len(s.answers) == 0 {
		return provider.Aside{}, errors.New("stub: no answer left")
	}
	out := s.answers[0]
	s.answers = s.answers[1:]
	return out, nil
}

// validated is an answer the harness checked against the schema itself.
func validated(structured string) provider.Aside {
	return provider.Aside{Text: "ignored", Structured: json.RawMessage(structured)}
}

func TestTurnReturnsQuestionThenDraft(t *testing.T) {
	r := &stubAsker{answers: []provider.Aside{
		validated(`{"status":"ask","question":"Was genau passiert?"}`),
		validated(`{"status":"ready","title":"Notification click does nothing","body":"### What happens\nNothing.","labels":["bug"]}`),
	}}
	s := New(r)

	d, err := s.Turn(context.Background(), claude, "", "", "notifications sind kaputt")
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if d.Status != "ask" || d.Question != "Was genau passiert?" {
		t.Fatalf("first turn = %+v, want an ask", d)
	}
	if d.SessionID == "" {
		t.Fatal("first turn returned no session id")
	}
	if d.Final {
		t.Fatal("first turn must not be final")
	}

	d2, err := s.Turn(context.Background(), claude, "", d.SessionID, "klicken öffnet die App nicht")
	if err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if d2.Status != "ready" || d2.Title == "" || len(d2.Labels) != 1 {
		t.Fatalf("second turn = %+v, want a ready draft", d2)
	}
	if d2.SessionID != d.SessionID {
		t.Fatalf("session id changed: %q -> %q", d.SessionID, d2.SessionID)
	}

	// The first turn opens the session and carries the instructions; the second
	// resumes it and must not repeat them (a harness replays the recorded ones
	// anyway, so a second set could only contradict the first).
	if r.reqs[0].Resume || r.reqs[0].System == "" || r.reqs[0].SessionID != d.SessionID {
		t.Fatalf("first turn = %+v, want a new session carrying the instructions", r.reqs[0])
	}
	if !r.reqs[1].Resume || r.reqs[1].System != "" || r.reqs[1].SessionID != d.SessionID {
		t.Fatalf("second turn = %+v, want a resume of the same session", r.reqs[1])
	}
	// A conversation that may continue has to keep its session.
	if !r.reqs[0].Continues {
		t.Fatal("the first turn discarded the session it might have to resume")
	}
}

// TestTurnAdoptsAHarnessesOwnSessionID: a harness that names its own session
// reports it back, and the next turn has to continue that one.
func TestTurnAdoptsAHarnessesOwnSessionID(t *testing.T) {
	first := validated(`{"status":"ask","question":"more?"}`)
	first.SessionID = "thread-42"
	r := &stubAsker{answers: []provider.Aside{first, validated(`{"status":"ready","title":"T","body":"B"}`)}}
	s := New(r)

	d, err := s.Turn(context.Background(), claude, "", "", "hi")
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if d.SessionID != "thread-42" {
		t.Fatalf("session = %q, want the harness's own id", d.SessionID)
	}
	if _, err := s.Turn(context.Background(), claude, "", d.SessionID, "more"); err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if !r.reqs[1].Resume || r.reqs[1].SessionID != "thread-42" {
		t.Fatalf("second turn = %+v, want a resume of thread-42", r.reqs[1])
	}
}

// TestTurnAsksTheChosenProviderCheaply: a draft goes to the instance the caller
// named, and — unless Settings says otherwise — on the cheap model, not on
// whatever expensive one was picked for real work.
func TestTurnAsksTheChosenProviderCheaply(t *testing.T) {
	r := &stubAsker{answers: []provider.Aside{validated(`{"status":"ready","title":"T","body":"B"}`)}}
	if _, err := New(r).Turn(context.Background(), claude, "", "", "hi"); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if r.insts[0].ID != "claude" {
		t.Errorf("asked %q, want the named instance", r.insts[0].ID)
	}
	if r.reqs[0].Model != DefaultModel {
		t.Errorf("model = %q, want %q", r.reqs[0].Model, DefaultModel)
	}
	if r.reqs[0].Schema == "" {
		t.Error("the draft's shape must be asked for, not hoped for")
	}
	if r.reqs[0].Text != "hi" {
		t.Errorf("text = %q, want the user's message", r.reqs[0].Text)
	}

	r2 := &stubAsker{answers: []provider.Aside{validated(`{"status":"ready","title":"T","body":"B"}`)}}
	if _, err := New(r2).Turn(context.Background(), claude, "opus", "", "hi"); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if r2.reqs[0].Model != "opus" {
		t.Errorf("model = %q, want the configured one", r2.reqs[0].Model)
	}
}

func TestTurnForcesADraftOnTheLastTurn(t *testing.T) {
	var out []provider.Aside
	for i := 0; i < MaxUserTurns; i++ {
		out = append(out, validated(`{"status":"ask","question":"noch was?","title":"Partial title"}`))
	}
	r := &stubAsker{answers: out}
	s := New(r)

	id := ""
	var d Draft
	for i := 0; i < MaxUserTurns; i++ {
		var err error
		d, err = s.Turn(context.Background(), claude, "", id, "more")
		if err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
		id = d.SessionID
	}
	if d.Status != "ready" || !d.Final {
		t.Fatalf("last turn = %+v, want a final ready draft", d)
	}
	if d.Question != "" {
		t.Fatalf("last turn still carries a question: %q", d.Question)
	}
	last := r.reqs[MaxUserTurns-1]
	if !strings.Contains(last.Text, "last exchange") {
		t.Fatal("the last turn's message does not tell the model it is the last one")
	}
	if last.Continues {
		t.Fatal("the last turn has to deliver, so nothing will resume its session")
	}
}

func TestTurnRejectsEmptyInputAndAMissingHarness(t *testing.T) {
	if _, err := New(&stubAsker{}).Turn(context.Background(), claude, "", "", "   "); err == nil {
		t.Fatal("empty message was accepted")
	}
	if _, err := New(nil).Turn(context.Background(), claude, "", "", "something"); err == nil {
		t.Fatal("a service with nothing to ask accepted a turn")
	}
}

func TestTurnReportsFailures(t *testing.T) {
	tests := []struct {
		name   string
		answer provider.Aside
		err    error
	}{
		{name: "the harness could not be reached", err: errors.New("exit status 1")},
		{name: "no object in the answer", answer: provider.Aside{Text: "boom"}},
		{name: "empty draft", answer: validated(`{"status":"ask"}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New(&stubAsker{answers: []provider.Aside{tc.answer}, err: tc.err})
			if _, err := s.Turn(context.Background(), claude, "", "", "hi"); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParseFindsTheDraftInPlainText(t *testing.T) {
	// A harness that did not validate the shape itself still answered; the
	// object is pulled out of what it wrote rather than the answer discarded.
	out := provider.Aside{Text: "Here you go:\n```json\n{\"status\":\"ready\",\"title\":\"T\",\"body\":\"B\"}\n```"}
	d, err := parse(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Title != "T" || d.Body != "B" {
		t.Fatalf("parse = %+v, want the draft from the result field", d)
	}
}

func TestSanitizeDropsUnknownLabelsAndTidiesTheTitle(t *testing.T) {
	d, err := sanitize(Draft{
		Status: "ready",
		Title:  "  Multi\nline   title  ",
		Body:   " body ",
		Labels: []string{"Bug", "bug", "feature-request", "enhancement", "question"},
	})
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	if d.Title != "Multi line title" {
		t.Fatalf("title = %q", d.Title)
	}
	if got, want := strings.Join(d.Labels, ","), "bug"; got != want {
		t.Fatalf("labels = %q, want %q", got, want)
	}
}

func TestSanitizePromotesADraftWithoutAQuestion(t *testing.T) {
	// A model that fills the issue but leaves status at "ask" must not strand
	// the user in the chat.
	d, err := sanitize(Draft{Status: "ask", Body: "something happened"})
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	if d.Status != "ready" || d.Title == "" {
		t.Fatalf("sanitize = %+v, want a ready draft with a title", d)
	}
}

func TestIssueURLPrefillsTheNewIssuePage(t *testing.T) {
	u := IssueURL("owner/repo", "A title", "A body\nwith a line break", []string{"bug", "nonsense"})
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Host != "github.com" || parsed.Path != "/owner/repo/issues/new" {
		t.Fatalf("url = %q", u)
	}
	q := parsed.Query()
	if q.Get("title") != "A title" || q.Get("body") != "A body\nwith a line break" {
		t.Fatalf("query = %v", q)
	}
	if q.Get("labels") != "bug" {
		t.Fatalf("labels = %q, want the known one only", q.Get("labels"))
	}
}

func TestIssueURLStaysUnderGitHubsLengthLimit(t *testing.T) {
	// GitHub answers a longer request URI with 414, so a huge body is trimmed
	// rather than lost.
	body := strings.Repeat("Ä line that encodes to more bytes than it has runes.\n", 400)
	u := IssueURL("owner/repo", "Long one", body, nil)
	if len(u) > MaxURLLen {
		t.Fatalf("url is %d bytes, want at most %d", len(u), MaxURLLen)
	}
	got, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if !strings.HasSuffix(got.Query().Get("body"), truncMark) {
		t.Fatal("a shortened body does not say that it was shortened")
	}
	if !strings.HasPrefix(got.Query().Get("body"), "Ä line") {
		t.Fatal("the start of the report was lost")
	}
}

func TestIssueURLKeepsAShortBodyIntact(t *testing.T) {
	u := IssueURL("owner/repo", "T", "short", nil)
	if strings.Contains(u, url.QueryEscape(truncMark)) {
		t.Fatalf("short body was shortened: %q", u)
	}
}
