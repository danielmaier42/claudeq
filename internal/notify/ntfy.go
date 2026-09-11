package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// DefaultNtfyServer is the public ntfy instance, used when no server is set.
// A self-hosted instance is the point of ntfy for many people, so the server is
// configurable and only defaults here.
const DefaultNtfyServer = "https://ntfy.sh"

// ntfyTopicPattern is what an ntfy topic may contain. Rejecting anything else
// up front keeps a mistyped topic from becoming a channel that silently
// publishes into a topic nobody is subscribed to.
var ntfyTopicPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Ntfy publishes notifications to an ntfy server (ntfy.sh or self-hosted).
// Messages are published as JSON to the server root rather than as headers on
// POST /<topic>, because a JSON body carries a title with umlauts or emoji
// without the header encoding ntfy would otherwise require.
type Ntfy struct {
	// Server is the instance's base URL. Empty uses DefaultNtfyServer.
	Server string
	// Topic is the topic to publish to (subscribers listen on it).
	Topic string
	// Token is an optional access token for a protected topic.
	Token string
	// Client overrides the HTTP client (for tests). Nil uses a 10s client.
	Client *http.Client
}

// Configured reports whether the channel can publish. Delegates to
// ValidateNtfy rather than restating the rule, so a server address that
// cannot be turned into an endpoint (a hand-edited config.toml, a config
// copied from another machine) is skipped instead of failing on every send.
func (t Ntfy) Configured() bool { return ValidateNtfy(t.Server, t.Topic) == nil }

// Notify publishes the notification to the topic. A link is sent as ntfy's
// click action, so tapping the notification on the phone opens it.
func (t Ntfy) Notify(ctx context.Context, n Notification) error {
	if !t.Configured() {
		return fmt.Errorf("ntfy not configured")
	}
	endpoint, err := NtfyEndpoint(t.Server)
	if err != nil {
		return fmt.Errorf("ntfy: %w", err)
	}
	body, err := json.Marshal(struct {
		Topic   string `json:"topic"`
		Title   string `json:"title"`
		Message string `json:"message"`
		Click   string `json:"click,omitempty"`
	}{
		Topic:   strings.TrimSpace(t.Topic),
		Title:   n.Title,
		Message: n.Message,
		Click:   n.URL,
	})
	if err != nil {
		return fmt.Errorf("build ntfy payload: %w", err)
	}
	header := http.Header{}
	if token := strings.TrimSpace(t.Token); token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	return post(ctx, t.Client, "ntfy", endpoint, "application/json", string(body), header)
}

// NtfyEndpoint normalizes a configured server into the URL messages are posted
// to. A bare host is read as https, and a trailing slash is dropped, so what
// someone types into the settings field ("ntfy.example.com") works as typed.
func NtfyEndpoint(server string) (string, error) {
	s := strings.TrimSpace(server)
	if s == "" {
		return DefaultNtfyServer, nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%q is not a URL", server)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%q is not an http(s) URL", server)
	}
	// A path here is almost always the topic pasted into the wrong field (ntfy's
	// own web UI shows a subscriber the full "https://ntfy.sh/<topic>" URL): kept
	// as-is, messages would publish as JSON POSTs to that path instead of the
	// server root, which ntfy answers 2xx while showing the raw JSON as the
	// message body instead of a proper title/click notification.
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("%q has a path (%q) — that belongs in the topic field, not the server", server, u.Path)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// ValidateNtfy checks a server/topic pair before it is saved, so a typo is
// refused while the operator is watching instead of swallowing every
// notification at three in the morning.
func ValidateNtfy(server, topic string) error {
	if _, err := NtfyEndpoint(server); err != nil {
		return fmt.Errorf("ntfy server: %w", err)
	}
	t := strings.TrimSpace(topic)
	if t == "" {
		return fmt.Errorf("ntfy topic: required (the name subscribers listen on)")
	}
	if strings.Contains(t, "/") || strings.Contains(t, "://") {
		return fmt.Errorf("ntfy topic: %q looks like a URL — the topic is only the last part of it, the rest belongs in the server field", topic)
	}
	if !ntfyTopicPattern.MatchString(t) {
		return fmt.Errorf("ntfy topic: %q is not a valid topic (letters, digits, - and _, up to 64 characters)", topic)
	}
	return nil
}
