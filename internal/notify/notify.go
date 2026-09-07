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
	// ArtifactID, when set, makes the macOS notification open that artifact in
	// the ClaudeQ window on click (see cmd/claudeqapp, notifyclick_cocoa.m).
	// Channels that cannot carry a click target ignore it.
	ArtifactID string
	// URL, when set, is the notification's link: Pushover shows it as the
	// message's supplementary URL, and clicking the macOS notification opens it
	// (again via cmd/claudeqapp). Channels that cannot carry a link ignore it.
	URL string
}

// IsWebURL reports whether s is an absolute http or https URL — the only kind
// of link a notification carries, because it is opened on click without
// further ado.
func IsWebURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
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

// Notify displays a native notification. When claudeq runs from its app bundle
// it posts through UNUserNotificationCenter so the notification carries the app
// icon; otherwise (or if that fails) it falls back to osascript, whose
// notifications show a generic script icon but always work from a LaunchAgent.
func (m Mac) Notify(ctx context.Context, n Notification) error {
	if nativeNotifyAvailable() {
		if err := postNativeNotification(n.Title, n.Message, n.ArtifactID, n.URL); err == nil {
			return nil
		}
		// fall through to osascript on failure
	}
	script := fmt.Sprintf("display notification %s with title %s",
		asString(n.Message), asString(n.Title))
	if out, err := m.Runner.Run(ctx, "osascript", "-e", script); err != nil {
		return fmt.Errorf("osascript: %w (%s)", err, string(out))
	}
	return nil
}

// RequestMacAuthorization asks for notification permission up front (the one-time
// system prompt), so later notifications can be delivered with the app icon. A
// no-op when not running from the app bundle.
func RequestMacAuthorization() {
	if nativeNotifyAvailable() {
		requestNativeAuth()
	}
}

// Authorization states reported by [MacAuthorization]. Only AuthorizationAllowed
// (and AuthorizationProvisional) actually put a notification on screen: macOS
// accepts and stores the others, then presents them as nothing.
const (
	AuthorizationAllowed       = "authorized"
	AuthorizationDenied        = "denied"
	AuthorizationNotDetermined = "not_determined"
	AuthorizationProvisional   = "provisional"
	AuthorizationUnknown       = "unknown"
	// AuthorizationUnsupported means the question cannot be asked: not macOS, or
	// not running from the app bundle (a bare dev binary uses osascript, which
	// has no authorization of its own).
	AuthorizationUnsupported = "unsupported"
)

// MacAuthorization reports whether macOS will show this app's notifications, so
// the UI can point out that alerts are being swallowed instead of leaving the
// operator to wonder why nothing arrives.
func MacAuthorization() string {
	if !nativeNotifyAvailable() {
		return AuthorizationUnsupported
	}
	return nativeAuthorizationStatus()
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
	if n.URL != "" {
		form.Set("url", n.URL)
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
