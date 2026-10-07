package api

import (
	"errors"
	"mime"
	"net"
	"net/http"
)

// uninstall opens ClaudeQ's uninstaller in the macOS Installer; the Installer
// asks for confirmation and the password. Two checks keep a web page in the
// user's browser from popping it up. A page on another origin cannot POST a
// JSON body without a CORS preflight, which this server never answers. A page
// whose own name was rebound to 127.0.0.1 (DNS rebinding) is same-origin, but
// its requests still carry that name as Host.
func (s *server) uninstall(w http.ResponseWriter, r *http.Request) {
	if !loopbackHost(r.Host) {
		writeErr(w, http.StatusForbidden, errors.New("uninstall is only accepted on 127.0.0.1"))
		return
	}
	if s.d.Uninstall == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("this build has no uninstaller; see the README"))
		return
	}
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct != "application/json" {
		writeErr(w, http.StatusUnsupportedMediaType, errors.New("expected a JSON body"))
		return
	}
	if err := s.d.Uninstall(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
