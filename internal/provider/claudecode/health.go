package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// ResolveBinary implements provider.Adapter: the instance's configured path
// first, then detection, then "" when the CLI cannot be located at all.
// Command falls back to the bare name so an exec-time lookup still gets a
// chance; a readiness check must not, or a missing CLI would read as ready.
func (a *Adapter) ResolveBinary(inst provider.Instance) string {
	if inst.BinaryPath != "" {
		return inst.BinaryPath
	}
	return a.DetectBinary()
}

// authStatus is the part of `claude auth status --json` claudeq reads. The
// command also reports the account's email and organisation; those are
// deliberately not parsed, so they cannot end up in a log or an API response.
type authStatus struct {
	LoggedIn   bool   `json:"loggedIn"`
	AuthMethod string `json:"authMethod"`
}

// CheckHealth implements provider.Adapter. It costs no model usage: it looks at
// the filesystem, asks the CLI for its version, and asks whether anyone is
// logged in.
func (a *Adapter) CheckHealth(ctx context.Context, inst provider.Instance, p provider.Prober) provider.Health {
	if h, ok := a.checkConfigDir(inst); !ok {
		return h
	}

	bin := a.ResolveBinary(inst)
	if bin == "" {
		return provider.Health{
			State: provider.HealthNotInstalled,
			Reason: "Claude Code was not found. Install the CLI, or set its path in " +
				"Settings → Providers.",
		}
	}
	if err := executableFile(bin); err != nil {
		return provider.Health{
			State:  provider.HealthNotInstalled,
			Binary: bin,
			Reason: fmt.Sprintf("Claude Code at %s cannot be run: %s.", bin, err),
		}
	}

	version, err := p.Probe(ctx, a.probeCommand(inst, bin, "--version"))
	if err != nil {
		return provider.Health{
			State:  provider.HealthNotInstalled,
			Binary: bin,
			Reason: fmt.Sprintf("Claude Code at %s did not answer --version (%s).", bin, redact(err.Error())),
		}
	}

	out, err := p.Probe(ctx, a.probeCommand(inst, bin, "auth", "status", "--json"))
	if err != nil {
		return provider.Health{
			State:  provider.HealthCheckFailed,
			Binary: bin,
			Detail: firstLine(string(version)),
			Reason: fmt.Sprintf("Claude Code at %s could not report its login status (%s).", bin, redact(err.Error())),
		}
	}
	var st authStatus
	if err := json.Unmarshal(jsonBody(out), &st); err != nil {
		return provider.Health{
			State:  provider.HealthCheckFailed,
			Binary: bin,
			Detail: firstLine(string(version)),
			Reason: "Claude Code reported its login status in a format claudeq does not understand.",
		}
	}
	if !st.LoggedIn {
		return provider.Health{
			State:  provider.HealthNotAuthenticated,
			Binary: bin,
			Detail: firstLine(string(version)),
			Reason: "Claude Code is not logged in. Run `claude auth login` in a terminal.",
		}
	}
	return provider.Health{
		State:  provider.HealthReady,
		Binary: bin,
		Detail: strings.TrimSpace(firstLine(string(version)) + " · " + st.AuthMethod),
	}
}

// checkConfigDir reports the instance's own settings as invalid when the
// configuration directory it names cannot be used. An unreadable account
// directory is a configuration problem, not a missing installation, and saying
// so is what tells the operator where to look.
func (a *Adapter) checkConfigDir(inst provider.Instance) (provider.Health, bool) {
	if inst.ConfigDir == "" {
		return provider.Health{}, true
	}
	fi, err := os.Stat(inst.ConfigDir)
	switch {
	case err != nil:
		return provider.Health{
			State:  provider.HealthInvalidConfiguration,
			Reason: fmt.Sprintf("The configuration directory %s is not accessible: %s.", inst.ConfigDir, redact(err.Error())),
		}, false
	case !fi.IsDir():
		return provider.Health{
			State:  provider.HealthInvalidConfiguration,
			Reason: fmt.Sprintf("The configuration directory %s is not a directory.", inst.ConfigDir),
		}, false
	}
	return provider.Health{}, true
}

// probeCommand builds a readiness probe for this instance — same binary, same
// configuration directory as a run, so the answer is about the account the jobs
// would actually use.
func (a *Adapter) probeCommand(inst provider.Instance, bin string, args ...string) provider.Command {
	cmd := provider.Command{Path: bin, Args: args}
	if inst.ConfigDir != "" {
		cmd.Env = append(cmd.Env, ConfigDirEnv+"="+inst.ConfigDir)
	}
	return cmd
}

// executableFile reports why a path cannot be run, or nil when it can.
func executableFile(p string) error {
	fi, err := os.Stat(p)
	switch {
	case err != nil:
		return fmt.Errorf("%s", redact(err.Error()))
	case fi.IsDir():
		return fmt.Errorf("it is a directory")
	case fi.Mode()&0o111 == 0:
		return fmt.Errorf("it is not executable")
	}
	return nil
}

// jsonBody takes the JSON document out of a CLI's output, which may be preceded
// by a line of noise (an update notice, a warning).
func jsonBody(out []byte) []byte {
	if i := strings.IndexAny(string(out), "{"); i > 0 {
		return out[i:]
	}
	return out
}

// firstLine is the first non-empty line of s, for the short version detail
// shown on the provider card.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// redact keeps a probe's own words out of the reason when they could carry
// account data: only the first line is kept, and it is cut short. The CLI's
// login status names an email address and an organisation, and a reason string
// is written to the daemon log and served over the API.
func redact(s string) string {
	line := firstLine(s)
	const limit = 160
	if r := []rune(line); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return line
}
