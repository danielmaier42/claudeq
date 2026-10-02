package claudecode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// UsageURL is the endpoint Claude Code's own `/usage` screen reads the plan
// limits from. It is not a documented API: it answers with the account's
// windows and costs no model usage, and if Anthropic changes it the dashboard
// says the limits are unavailable and nothing else is affected.
const UsageURL = "https://api.anthropic.com/api/oauth/usage"

// usageBeta is the beta header the endpoint requires for OAuth callers.
const usageBeta = "oauth-2025-04-20"

// keychainService is the macOS keychain item Claude Code keeps its login in.
// An instance with its own configuration directory gets an item of its own,
// suffixed with the start of the directory's SHA-256 — the same name the CLI
// derives, so claudeq reads exactly the account the instance's runs use.
func keychainService(configDir string) string {
	const base = "Claude Code-credentials"
	if configDir == "" {
		return base
	}
	sum := sha256.Sum256([]byte(configDir))
	return base + "-" + hex.EncodeToString(sum[:])[:8]
}

// credentials is the part of Claude Code's stored login claudeq reads. The
// refresh token is deliberately not parsed: claudeq never renews a login
// itself, because a renewal rotates the token and would log the CLI out.
type credentials struct {
	OAuth *struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"` // milliseconds since the epoch
	} `json:"claudeAiOauth"`
}

// usageWindow is one window of the usage endpoint's answer.
type usageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

// usageReport is the part of the usage endpoint's answer claudeq shows: the
// five-hour session window and the weekly window across all models.
type usageReport struct {
	FiveHour *usageWindow `json:"five_hour"`
	SevenDay *usageWindow `json:"seven_day"`
}

// ReadLimits implements provider.LimitReader: the account's five-hour and
// weekly windows, read with the login the CLI stored for this instance.
func (a *Adapter) ReadLimits(ctx context.Context, inst provider.Instance) ([]provider.LimitWindow, error) {
	token, err := a.accessToken(ctx, inst)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.usageURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("build usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", usageBeta)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		// The client's error names the URL, never the header, so it is safe to
		// pass on; it is cut to a line all the same.
		return nil, fmt.Errorf("the Claude usage endpoint could not be reached (%s)", redact(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read Claude's usage answer: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, errors.New("the stored Claude login was refused; it is renewed on the next run on this account")
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("the Claude usage endpoint asked claudeq to slow down; the next reading tries again")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("the Claude usage endpoint answered %s", resp.Status)
	}
	return parseUsage(body)
}

// parseUsage turns the endpoint's answer into the two windows the dashboard
// shows. A window the account does not have is left out; an answer with
// neither is an error, because "no limits" is not something Claude says.
func parseUsage(body []byte) ([]provider.LimitWindow, error) {
	var rep usageReport
	if err := json.Unmarshal(body, &rep); err != nil {
		return nil, fmt.Errorf("the Claude usage answer is not in a format claudeq understands: %w", err)
	}
	var out []provider.LimitWindow
	for _, w := range []struct {
		id, label string
		src       *usageWindow
	}{
		{"five_hour", "5 hours", rep.FiveHour},
		{"week", "Week", rep.SevenDay},
	} {
		if w.src == nil || w.src.Utilization == nil {
			continue
		}
		lw := provider.LimitWindow{ID: w.id, Label: w.label, UsedPercent: *w.src.Utilization}
		if at, err := time.Parse(time.RFC3339Nano, w.src.ResetsAt); err == nil {
			lw.ResetsAt = &at
		}
		out = append(out, lw)
	}
	if len(out) == 0 {
		return nil, errors.New("the Claude usage answer named no limit windows")
	}
	return out, nil
}

// accessToken returns the instance's stored OAuth access token: from the
// keychain where macOS Claude Code keeps it, else from the credentials file the
// CLI writes where there is no keychain. The token is never logged or wrapped
// into an error.
func (a *Adapter) accessToken(ctx context.Context, inst provider.Instance) (string, error) {
	raw, err := a.readKeychain(ctx, keychainService(inst.ConfigDir))
	if err != nil {
		dir := inst.ConfigDir
		if dir == "" {
			home, herr := os.UserHomeDir()
			if herr != nil {
				return "", fmt.Errorf("locate Claude's configuration directory: %w", herr)
			}
			dir = filepath.Join(home, ".claude")
		}
		var ferr error
		raw, ferr = os.ReadFile(filepath.Join(dir, ".credentials.json")) // #nosec G304 — the instance's own configuration directory
		if ferr != nil {
			return "", errors.New("no Claude login was found for this account; run `claude auth login`")
		}
	}
	var c credentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", errors.New("the stored Claude login is not in a format claudeq understands")
	}
	if c.OAuth == nil || c.OAuth.AccessToken == "" {
		return "", errors.New("this account is not logged in with a Claude plan, so it has no plan limits to show")
	}
	if c.OAuth.ExpiresAt > 0 && a.now().After(time.UnixMilli(c.OAuth.ExpiresAt)) {
		return "", errors.New("the stored Claude login has expired; it is renewed on the next run on this account")
	}
	return c.OAuth.AccessToken, nil
}

// readKeychain returns the secret of a generic keychain item, through the same
// `security` tool Claude Code itself uses to store it — which is why reading it
// raises no keychain prompt.
func (a *Adapter) readKeychain(ctx context.Context, service string) ([]byte, error) {
	if a.keychain != nil {
		return a.keychain(ctx, service)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", service, "-w")
	// Only stdout: it holds the secret and nothing else, and stderr is not
	// needed to know the item is missing.
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("keychain item %q: %w", service, err)
	}
	return out, nil
}

func (a *Adapter) usageURL() string {
	if a.usageEndpoint != "" {
		return a.usageEndpoint
	}
	return UsageURL
}

func (a *Adapter) httpClient() *http.Client {
	if a.http != nil {
		return a.http
	}
	return &http.Client{Timeout: provider.LimitsTimeout}
}

func (a *Adapter) now() time.Time {
	if a.clock != nil {
		return a.clock()
	}
	return time.Now()
}
