package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultTimeout bounds every outbound channel. A notification is best-effort:
// a channel that stops answering must never hold up the daemon's send.
const defaultTimeout = 10 * time.Second

// errBodyLimit is how much of a rejected reply is quoted back in the error.
// Enough for the one-line reason these services return, not enough to fill the
// daemon log with an HTML error page.
const errBodyLimit = 200

// post sends one request to a channel's endpoint and turns a non-2xx reply into
// an error naming the channel and quoting its reason, so a channel that stops
// accepting notifications says why in the daemon's log instead of failing
// silently. The reply is never parsed: none of the channels tells us anything
// useful on success.
func post(ctx context.Context, client *http.Client, channel, endpoint, contentType, body string, header http.Header) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", channel, err)
	}
	req.Header.Set("Content-Type", contentType)
	for name, values := range header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", channel, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s returned status %d%s", channel, resp.StatusCode, reason(resp.Body))
	}
	return nil
}

// reason renders a failed reply's first line as a parenthesised suffix, or
// nothing when the channel said nothing.
func reason(r io.Reader) string {
	b, err := io.ReadAll(io.LimitReader(r, errBodyLimit))
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(strings.ReplaceAll(string(b), "\n", " "))
	if text == "" {
		return ""
	}
	return " (" + text + ")"
}
