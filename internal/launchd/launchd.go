// Package launchd installs claudeqd as a user LaunchAgent so it starts at login
// and is restarted if it exits (PLAN.md §5.1, NFA-03).
package launchd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/danielmaier42/claudeq/internal/system"
)

// DefaultLabel is the LaunchAgent label / reverse-DNS identifier.
const DefaultLabel = "de.maierdaniel.claudeq"

// Config parameterizes the generated plist.
type Config struct {
	Label      string
	BinPath    string   // absolute path to claudeqd
	Args       []string // arguments (e.g. ["run"])
	StdoutPath string
	StderrPath string
	// AssociatedBundleID ties the agent to an app bundle so System Settings →
	// Login Items shows the app's name and icon instead of a bare "claudeqd
	// (unknown developer)". Empty omits the key (e.g. dev runs outside a bundle).
	AssociatedBundleID string
}

var plistTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.BinPath}}</string>
{{- range .Args}}
		<string>{{.}}</string>
{{- end}}
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
{{- if .AssociatedBundleID}}
	<key>AssociatedBundleIdentifiers</key>
	<array>
		<string>{{.AssociatedBundleID}}</string>
	</array>
{{- end}}
	<key>StandardOutPath</key>
	<string>{{.StdoutPath}}</string>
	<key>StandardErrorPath</key>
	<string>{{.StderrPath}}</string>
</dict>
</plist>
`))

// Plist renders the LaunchAgent plist XML for the config.
func Plist(c Config) (string, error) {
	var b strings.Builder
	if err := plistTemplate.Execute(&b, c); err != nil {
		return "", fmt.Errorf("render plist: %w", err)
	}
	return b.String(), nil
}

// Agent installs and removes the LaunchAgent. Dir is the LaunchAgents directory
// (e.g. ~/Library/LaunchAgents); UID is the target GUI domain user id.
type Agent struct {
	Runner system.Runner
	Dir    string
	Label  string
	UID    int
}

func (a Agent) label() string {
	if a.Label != "" {
		return a.Label
	}
	return DefaultLabel
}

func (a Agent) plistPath() string {
	return filepath.Join(a.Dir, a.label()+".plist")
}

// Install writes the plist and (re)bootstraps it into the user's GUI domain.
func (a Agent) Install(ctx context.Context, plist string) error {
	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	path := a.plistPath()
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}
	domain := fmt.Sprintf("gui/%d", a.UID)
	// Remove any prior instance so bootstrap does not fail on "already loaded".
	_, _ = a.Runner.Run(ctx, "launchctl", "bootout", domain+"/"+a.label())
	if out, err := a.Runner.Run(ctx, "launchctl", "bootstrap", domain, path); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w (%s)", err, string(out))
	}
	return nil
}

// Uninstall boots out the agent and removes its plist.
func (a Agent) Uninstall(ctx context.Context) error {
	domain := fmt.Sprintf("gui/%d", a.UID)
	_, _ = a.Runner.Run(ctx, "launchctl", "bootout", domain+"/"+a.label())
	if err := os.Remove(a.plistPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist: %w", err)
	}
	return nil
}
