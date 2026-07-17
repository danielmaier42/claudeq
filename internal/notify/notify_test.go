package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordRunner struct {
	args []string
	err  error
}

func (r *recordRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.args = append([]string{name}, args...)
	return nil, r.err
}

func TestMacBuildsEscapedOsascript(t *testing.T) {
	r := &recordRunner{}
	m := Mac{Runner: r}
	err := m.Notify(context.Background(), Notification{Title: `cq "run"`, Message: `it failed: "boom"`})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if r.args[0] != "osascript" || r.args[1] != "-e" {
		t.Fatalf("expected osascript -e, got %v", r.args)
	}
	script := r.args[2]
	if !strings.Contains(script, `display notification`) || !strings.Contains(script, `with title`) {
		t.Fatalf("unexpected script: %s", script)
	}
	if !strings.Contains(script, `\"boom\"`) {
		t.Fatalf("quotes not escaped in message: %s", script)
	}
}

func TestMacPropagatesRunnerError(t *testing.T) {
	m := Mac{Runner: &recordRunner{err: errors.New("no gui")}}
	if err := m.Notify(context.Background(), Notification{Title: "t", Message: "m"}); err == nil {
		t.Fatal("expected error from runner")
	}
}

func TestPushoverPostsExpectedForm(t *testing.T) {
	var gotForm string
	var gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form.Encode()
		gotType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := Pushover{Token: "tok", UserKey: "usr", URL: srv.URL, Client: srv.Client()}
	if err := p.Notify(context.Background(), Notification{Title: "T", Message: "M"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	for _, want := range []string{"token=tok", "user=usr", "title=T", "message=M"} {
		if !strings.Contains(gotForm, want) {
			t.Fatalf("form %q missing %q", gotForm, want)
		}
	}
	if !strings.HasPrefix(gotType, "application/x-www-form-urlencoded") {
		t.Fatalf("content-type = %q", gotType)
	}
}

func TestPushoverErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	p := Pushover{Token: "t", UserKey: "u", URL: srv.URL, Client: srv.Client()}
	if err := p.Notify(context.Background(), Notification{Title: "T", Message: "M"}); err == nil {
		t.Fatal("expected error on 400")
	}
}

func TestPushoverNotConfigured(t *testing.T) {
	if (Pushover{}).Configured() {
		t.Fatal("empty pushover should not be configured")
	}
	if err := (Pushover{}).Notify(context.Background(), Notification{}); err == nil {
		t.Fatal("expected error when not configured")
	}
}

type stubNotifier struct {
	called int
	err    error
}

func (s *stubNotifier) Notify(context.Context, Notification) error {
	s.called++
	return s.err
}

func TestMultiFansOutAndJoinsErrors(t *testing.T) {
	ok := &stubNotifier{}
	bad := &stubNotifier{err: errors.New("boom")}
	m := Multi{Notifiers: []Notifier{ok, bad, nil}}
	err := m.Notify(context.Background(), Notification{Title: "t", Message: "m"})
	if ok.called != 1 || bad.called != 1 {
		t.Fatalf("both notifiers should be attempted: ok=%d bad=%d", ok.called, bad.called)
	}
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected joined error, got %v", err)
	}
}
