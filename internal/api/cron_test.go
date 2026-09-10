package api

import (
	"strings"
	"testing"
	"time"
)

func TestCheckCronEndpoint(t *testing.T) {
	srv, _ := newServer(t, nil)

	r := do(t, srv, "GET", "/api/cron/check?expr=0+20+*+*+*", nil)
	if r.Status != 200 {
		t.Fatalf("status = %d, want 200 (%s)", r.Status, r.Body)
	}
	var ok cronCheck
	r.into(t, &ok)
	if !ok.Valid || ok.Error != "" {
		t.Fatalf("valid expression rejected: %+v", ok)
	}
	if len(ok.Next) != cronPreviewRuns {
		t.Fatalf("got %d upcoming runs, want %d (%v)", len(ok.Next), cronPreviewRuns, ok.Next)
	}
	for i, n := range ok.Next {
		if !n.After(time.Now()) {
			t.Errorf("upcoming run %d (%v) is not in the future", i, n)
		}
		if n.Minute() != 0 || n.Hour() != 20 {
			t.Errorf("upcoming run %d = %v, want 20:00", i, n)
		}
	}
}

// A bad expression is an answer, not a failed request: the sheet asks on every
// keystroke and must be able to show the reason inline.
func TestCheckCronEndpointRejects(t *testing.T) {
	srv, _ := newServer(t, nil)
	for _, tc := range []struct{ name, query, want string }{
		{"out of range", "expr=0+99+*+*+*", "hour"},
		{"too few fields", "expr=0+20+*+*", "exactly 5 fields"},
		{"empty", "", "missing cron schedule"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := do(t, srv, "GET", "/api/cron/check?"+tc.query, nil)
			if r.Status != 200 {
				t.Fatalf("status = %d, want 200 (%s)", r.Status, r.Body)
			}
			var got cronCheck
			r.into(t, &got)
			if got.Valid {
				t.Fatalf("expression accepted, want rejected: %+v", got)
			}
			if !strings.Contains(got.Error, tc.want) {
				t.Errorf("error = %q, want it to mention %q", got.Error, tc.want)
			}
			if len(got.Next) != 0 {
				t.Errorf("rejected expression previews runs: %v", got.Next)
			}
		})
	}
}

// The task endpoints must reject a bad schedule too — the sheet's live check is
// convenience, not the gate.
func TestAddTaskRejectsBadCron(t *testing.T) {
	srv, _ := newServer(t, nil)
	bad := sampleTask("nightly")
	bad.Trigger = "cron"
	bad.Cron = "0 99 * * *"
	r := do(t, srv, "POST", "/api/tasks", bad)
	if r.Status != 400 {
		t.Fatalf("status = %d, want 400 (%s)", r.Status, r.Body)
	}
	if !strings.Contains(string(r.Body), "hour") {
		t.Errorf("error does not name the field: %s", r.Body)
	}

	good := bad
	good.Cron = "0 20 * * *"
	if r := do(t, srv, "POST", "/api/tasks", good); r.Status != 201 {
		t.Fatalf("valid cron task rejected: %d (%s)", r.Status, r.Body)
	}
	upd := good
	upd.Cron = "0 20 * *"
	if r := do(t, srv, "PUT", "/api/tasks/"+good.ID, upd); r.Status != 400 {
		t.Fatalf("update with a bad cron accepted: %d (%s)", r.Status, r.Body)
	}
}
