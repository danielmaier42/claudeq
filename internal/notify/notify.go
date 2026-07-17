// Package notify delivers claudeq notifications to native macOS Notification
// Center (via osascript) and to Pushover (FA-39/40/41). Delivery is best-effort:
// a failing channel never blocks the caller's flow.
package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/system"
)

// Notification is a single message to deliver.
type Notification struct {
	Title   string
	Message string
}

// Notifier delivers a notification.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
}

// Mac posts native macOS notifications via `osascript`. It works from a user
// LaunchAgent session (PLAN.md V3).
type Mac struct {
	Runner system.Runner
}

// Notify displays a native notification.
func (m Mac) Notify(ctx context.Context, n Notification) error {
	script := fmt.Sprintf("display notification %s with title %s",
		asString(n.Message), asString(n.Title))
	if out, err := m.Runner.Run(ctx, "osascript", "-e", script); err != nil {
		return fmt.Errorf("osascript: %w (%s)", err, string(out))
	}
	return nil
}

// asString renders a Go string as an AppleScript double-quoted string literal.
func asString(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return "\"" + r.Replace(s) + "\""
}

// DefaultPushoverURL is the Pushover messages endpoint.
const DefaultPushoverURL = "https://api.pushover.net/1/messages.json"

// Pushover delivers notifications to the Pushover mobile service.
type Pushover struct {
	Token   string
	UserKey string
	// URL overrides the endpoint (for tests). Empty uses DefaultPushoverURL.
	URL string
	// Client overrides the HTTP client (for tests). Empty uses a 10s client.
	Client *http.Client
}

// Configured reports whether credentials are present.
func (p Pushover) Configured() bool { return p.Token != "" && p.UserKey != "" }

// Notify posts the message to Pushover.
func (p Pushover) Notify(ctx context.Context, n Notification) error {
	if !p.Configured() {
		return fmt.Errorf("pushover not configured")
	}
	endpoint := p.URL
	if endpoint == "" {
		endpoint = DefaultPushoverURL
	}
	form := url.Values{
		"token":   {p.Token},
		"user":    {p.UserKey},
		"title":   {n.Title},
		"message": {n.Message},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build pushover request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("pushover request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("pushover returned status %d", resp.StatusCode)
	}
	return nil
}

// Multi fans a notification out to several notifiers, best-effort: it attempts
// all of them and joins any errors.
type Multi struct {
	Notifiers []Notifier
}

// Notify delivers to every configured notifier.
func (m Multi) Notify(ctx context.Context, n Notification) error {
	var errs []string
	for _, notifier := range m.Notifiers {
		if notifier == nil {
			continue
		}
		if err := notifier.Notify(ctx, n); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notify: %s", strings.Join(errs, "; "))
	}
	return nil
}
