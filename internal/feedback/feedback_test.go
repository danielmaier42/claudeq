package feedback

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

// stubRunner records the invocations and replays canned CLI output.
type stubRunner struct {
	out  [][]byte
	err  error
	argv [][]string
	dirs []string
}

func (s *stubRunner) Run(_ context.Context, dir string, argv []string) ([]byte, error) {
	s.argv = append(s.argv, argv)
	s.dirs = append(s.dirs, dir)
	if s.err != nil {
		return nil, s.err
	}
	if len(s.out) == 0 {
		return nil, errors.New("stub: no output left")
	}
	out := s.out[0]
	s.out = s.out[1:]
	return out, nil
}

// cliJSON wraps a draft the way `claude -p --output-format json` reports it.
func cliJSON(structured string) []byte {
	return []byte(`{"is_error":false,"subtype":"success","result":"ignored","structured_output":` + structured + `}`)
}

func TestTurnReturnsQuestionThenDraft(t *testing.T) {
	r := &stubRunner{out: [][]byte{
		cliJSON(`{"status":"ask","question":"Was genau passiert?"}`),
		cliJSON(`{"status":"ready","title":"Notification click does nothing","body":"### What happens\nNothing.","labels":["bug"]}`),
	}}
	s := New(r)

	d, err := s.Turn(context.Background(), "/bin/claude", "", "notifications sind kaputt")
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

	d2, err := s.Turn(context.Background(), "/bin/claude", d.SessionID, "klicken öffnet die App nicht")
	if err != nil {
		t.Fatalf("second turn: %v", err)
	}
	if d2.Status != "ready" || d2.Title == "" || len(d2.Labels) != 1 {
		t.Fatalf("second turn = %+v, want a ready draft", d2)
	}
	if d2.SessionID != d.SessionID {
		t.Fatalf("session id changed: %q -> %q", d.SessionID, d2.SessionID)
	}

	// First call opens the session and carries the system prompt; the second
	// resumes it and must not repeat the prompt (the CLI replays the recorded
	// one anyway).
	if !hasFlag(r.argv[0], "--session-id", d.SessionID) || !hasFlagName(r.argv[0], "--system-prompt") {
		t.Fatalf("first argv = %v, want --session-id and --system-prompt", r.argv[0])
	}
	if !hasFlag(r.argv[1], "--resume", d.SessionID) || hasFlagName(r.argv[1], "--session-id") {
		t.Fatalf("second argv = %v, want --resume only", r.argv[1])
	}
	// Both turns run in the same throwaway directory, or the CLI would not find
	// the session to resume.
	if r.dirs[0] != r.dirs[1] || r.dirs[0] == "" {
		t.Fatalf("session dirs = %q, %q, want one stable directory", r.dirs[0], r.dirs[1])
	}
}

func TestTurnLocksDownTheSession(t *testing.T) {
	r := &stubRunner{out: [][]byte{cliJSON(`{"status":"ready","title":"T","body":"B"}`)}}
	if _, err := New(r).Turn(context.Background(), "/bin/claude", "", "hi"); err != nil {
		t.Fatalf("turn: %v", err)
	}
	argv := r.argv[0]
	for _, want := range []string{"-p", "--strict-mcp-config", "--disable-slash-commands", "--safe-mode"} {
		if !hasFlagName(argv, want) {
			t.Errorf("argv %v is missing %s", argv, want)
		}
	}
	if !hasFlag(argv, "--model", Model) {
		t.Errorf("argv %v does not pin the model to %s", argv, Model)
	}
	if !hasFlag(argv, "--tools", "") {
		t.Errorf("argv %v does not disable tools", argv)
	}
	if !hasFlag(argv, "--output-format", "json") {
		t.Errorf("argv %v does not ask for json output", argv)
	}
	if argv[len(argv)-1] != "hi" {
		t.Errorf("argv %v does not end with the user's message", argv)
	}
}

func TestTurnForcesADraftOnTheLastTurn(t *testing.T) {
	var out [][]byte
	for i := 0; i < MaxUserTurns; i++ {
		out = append(out, cliJSON(`{"status":"ask","question":"noch was?","title":"Partial title"}`))
	}
	r := &stubRunner{out: out}
	s := New(r)

	id := ""
	var d Draft
	for i := 0; i < MaxUserTurns; i++ {
		var err error
		d, err = s.Turn(context.Background(), "/bin/claude", id, "more")
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
	if !strings.Contains(r.argv[MaxUserTurns-1][len(r.argv[MaxUserTurns-1])-1], "last exchange") {
		t.Fatal("the last turn's message does not tell the model it is the last one")
	}
}

func TestTurnRejectsEmptyInputAndMissingBinary(t *testing.T) {
	s := New(&stubRunner{})
	if _, err := s.Turn(context.Background(), "/bin/claude", "", "   "); err == nil {
		t.Fatal("empty message was accepted")
	}
	if _, err := s.Turn(context.Background(), "", "", "something"); err == nil {
		t.Fatal("missing binary was accepted")
	}
}

func TestTurnReportsCLIFailures(t *testing.T) {
	tests := []struct {
		name string
		out  []byte
		err  error
	}{
		{name: "process failed", err: errors.New("exit status 1")},
		{name: "not json", out: []byte("boom")},
		{name: "error result", out: []byte(`{"is_error":true,"subtype":"error_during_execution","result":"rate limit"}`)},
		{name: "empty draft", out: cliJSON(`{"status":"ask"}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New(&stubRunner{out: [][]byte{tc.out}, err: tc.err})
			if _, err := s.Turn(context.Background(), "/bin/claude", "", "hi"); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParseFallsBackToTheResultField(t *testing.T) {
	// Some CLI versions report the structured answer only as the result string.
	out := []byte(`{"is_error":false,"result":"{\"status\":\"ready\",\"title\":\"T\",\"body\":\"B\"}"}`)
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
	if got, want := strings.Join(d.Labels, ","), "bug,enhancement"; got != want {
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

func hasFlagName(argv []string, name string) bool {
	for _, a := range argv {
		if a == name {
			return true
		}
	}
	return false
}

func hasFlag(argv []string, name, value string) bool {
	for i, a := range argv {
		if a == name && i+1 < len(argv) && argv[i+1] == value {
			return true
		}
	}
	return false
}
