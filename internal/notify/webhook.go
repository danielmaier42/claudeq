package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// DefaultWebhookTemplate is the body sent when no template is configured: a
// small, self-describing JSON object. Services that expect a particular shape
// (Slack's "text", Discord's "content", a Home Assistant automation) get it by
// setting a template instead.
const DefaultWebhookTemplate = `{"title":"{{title}}","message":"{{message}}","url":"{{url}}","source":"claudeq"}`

// webhookFields are the placeholders a template may use. Anything else is
// refused when the template is saved: a mistyped {{titel}} would otherwise be
// posted verbatim for months without anyone noticing.
var webhookFields = []string{"title", "message", "url"}

// webhookPlaceholder matches any {{...}} in a template, so unknown fields can
// be named in the error rather than silently passed through. The capture
// group is deliberately wider than the three known field names (letters,
// digits, dot and hyphen) so a mistyped {{title2}} or {{run-id}} is still
// caught as unknown instead of slipping past this pattern uncaught and being
// posted to the endpoint verbatim.
var webhookPlaceholder = regexp.MustCompile(`{{\s*([A-Za-z0-9_.-]*)\s*}}`)

// Webhook posts notifications as JSON to any URL. It is the one channel that
// covers services claudeq does not know about: Slack, Discord and Teams
// incoming webhooks, Home Assistant, n8n, or a script behind a reverse proxy.
type Webhook struct {
	// URL is the endpoint to post to.
	URL string
	// Template is the request body, with {{title}}, {{message}} and {{url}}
	// substituted. Empty uses DefaultWebhookTemplate.
	Template string
	// Client overrides the HTTP client (for tests). Nil uses a 10s client.
	Client *http.Client
}

// Configured reports whether the channel can deliver. Delegates to
// ValidateWebhook rather than restating the rule, so a template with an
// unknown placeholder or broken JSON (a hand-edited config.toml) is skipped
// instead of posting garbage on every send.
func (w Webhook) Configured() bool { return ValidateWebhook(w.URL, w.Template) == nil }

// Notify posts the rendered template to the endpoint.
func (w Webhook) Notify(ctx context.Context, n Notification) error {
	if !w.Configured() {
		return fmt.Errorf("webhook not configured")
	}
	body := RenderWebhookBody(w.Template, n)
	return post(ctx, w.Client, "webhook", strings.TrimSpace(w.URL), "application/json", body, nil)
}

// RenderWebhookBody substitutes a notification into a template. Values are
// escaped as JSON string content, so a message containing quotes or newlines
// cannot break the body it is placed into.
func RenderWebhookBody(template string, n Notification) string {
	tpl := strings.TrimSpace(template)
	if tpl == "" {
		tpl = DefaultWebhookTemplate
	}
	values := map[string]string{"title": n.Title, "message": n.Message, "url": n.URL}
	return webhookPlaceholder.ReplaceAllStringFunc(tpl, func(match string) string {
		name := webhookPlaceholder.FindStringSubmatch(match)[1]
		v, ok := values[name]
		if !ok {
			return match
		}
		return jsonEscape(v)
	})
}

// ValidateWebhook checks a URL and template before they are saved. The template
// is rendered with values that contain everything awkward — a quote, a
// backslash, a newline, a non-ASCII character — and the result must be valid
// JSON, which is what catches a missing brace or a placeholder left outside its
// string.
func ValidateWebhook(rawURL, template string) error {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return fmt.Errorf("webhook URL: required")
	}
	if !IsWebURL(u) {
		return fmt.Errorf("webhook URL: %q is not an http(s) URL", rawURL)
	}
	if unknown := unknownWebhookFields(template); len(unknown) > 0 {
		return fmt.Errorf("webhook body: unknown placeholder %s (known: {{%s}})",
			strings.Join(unknown, ", "), strings.Join(webhookFields, "}}, {{"))
	}
	sample := Notification{
		Title:   `ClaudeQ "test"\1`,
		Message: "line one\nline two — ümlaut",
		URL:     "https://example.com/a?b=1&c=2",
	}
	if body := RenderWebhookBody(template, sample); !json.Valid([]byte(body)) {
		return fmt.Errorf("webhook body: the template does not produce valid JSON")
	}
	return nil
}

// unknownWebhookFields lists the placeholders in a template that claudeq cannot
// fill, quoted as they were written.
func unknownWebhookFields(template string) []string {
	var unknown []string
	seen := map[string]bool{}
	for _, m := range webhookPlaceholder.FindAllStringSubmatch(template, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		known := false
		for _, f := range webhookFields {
			if f == name {
				known = true
				break
			}
		}
		if !known {
			unknown = append(unknown, m[0])
		}
	}
	return unknown
}

// jsonEscape renders a string as JSON string content, without the quotes, so it
// can be substituted into a template that supplies them.
func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(b[1 : len(b)-1])
}
