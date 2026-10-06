package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os/exec"
)

// Uninstaller starts removing ClaudeQ from this Mac; purge deletes the user's
// tasks, history and settings too. It returns once the removal is under way.
type Uninstaller func(purge bool) error

// DetachedUninstaller runs `claudeq uninstall --gui` from the CLI at cliPath in
// a session of its own. The uninstall boots out the LaunchAgent, which ends
// this daemon and everything else in its job; detached, it lives on to finish
// and to report the outcome in a macOS alert. Nil when cliPath is empty.
func DetachedUninstaller(cliPath string) Uninstaller {
	if cliPath == "" {
		return nil
	}
	return func(purge bool) error {
		args := []string{"uninstall", "--gui"}
		if purge {
			args = append(args, "--purge")
		}
		cmd := exec.Command(cliPath, args...)
		detach(cmd)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start %s uninstall: %w", cliPath, err)
		}
		// Reap it if this process happens to outlive the uninstall (the user
		// dismissed the password prompt).
		go func() { _ = cmd.Wait() }()
		return nil
	}
}

// uninstall starts removing ClaudeQ. Two checks keep a web page in the user's
// browser from triggering it. A page on another origin cannot POST a JSON body
// without a CORS preflight, which this server never answers. A page whose own
// name was rebound to 127.0.0.1 (DNS rebinding) is same-origin, but its
// requests still carry that name as Host.
func (s *server) uninstall(w http.ResponseWriter, r *http.Request) {
	if !loopbackHost(r.Host) {
		writeErr(w, http.StatusForbidden, errors.New("uninstall is only accepted on 127.0.0.1"))
		return
	}
	if s.d.Uninstall == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("this build cannot uninstall itself; see the README"))
		return
	}
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct != "application/json" {
		writeErr(w, http.StatusUnsupportedMediaType, errors.New("expected a JSON body"))
		return
	}
	var in struct {
		Purge bool `json:"purge"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	if err := s.d.Uninstall(in.Purge); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"purge": in.Purge})
}

// loopbackHost reports whether a request's Host names this machine's loopback
// interface, the only address the dashboard is served on.
func loopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}
