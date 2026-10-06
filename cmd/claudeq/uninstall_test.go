package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/uninstall"
)

type fakeMac struct {
	plan     uninstall.Plan
	planErr  error
	runErr   error
	purge    bool
	ran      bool
	planned  bool
	runInput uninstall.Plan
}

func (f *fakeMac) Plan(_ context.Context, purge bool) (uninstall.Plan, error) {
	f.planned, f.purge = true, purge
	return f.plan, f.planErr
}

func (f *fakeMac) Run(_ context.Context, p uninstall.Plan) error {
	f.ran, f.runInput = true, p
	return f.runErr
}

type alertCall struct {
	title, message string
	critical       bool
}

func newUninstallEnv(mac *fakeMac, stdin string) (uninstallEnv, *bytes.Buffer, *[]alertCall) {
	out := &bytes.Buffer{}
	alerts := &[]alertCall{}
	return uninstallEnv{
		mac: mac, dataDir: "/data", uid: 501, in: strings.NewReader(stdin), out: out,
		alert: func(title, message string, critical bool) error {
			*alerts = append(*alerts, alertCall{title, message, critical})
			return nil
		},
	}, out, alerts
}

func TestUninstallAsksFirst(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		run   bool
	}{
		{"no answer", nil, "", false},
		{"no", nil, "n\n", false},
		{"yes", nil, "y\n", true},
		{"--yes skips the question", []string{"--yes"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mac := &fakeMac{plan: uninstall.Plan{Apps: []string{"/Applications/ClaudeQ.app"}}}
			env, out, _ := newUninstallEnv(mac, tc.stdin)
			if err := runUninstall(tc.args, env); err != nil {
				t.Fatal(err)
			}
			if mac.ran != tc.run {
				t.Fatalf("ran = %v, want %v\n%s", mac.ran, tc.run, out)
			}
			if !strings.Contains(out.String(), "/Applications/ClaudeQ.app") {
				t.Fatalf("plan not shown:\n%s", out)
			}
		})
	}
}

func TestUninstallPurgeFlag(t *testing.T) {
	mac := &fakeMac{}
	env, out, _ := newUninstallEnv(mac, "")
	if err := runUninstall([]string{"--purge", "--yes"}, env); err != nil {
		t.Fatal(err)
	}
	if !mac.purge {
		t.Fatal("--purge was not passed on")
	}
	if !strings.Contains(out.String(), "deleted too") {
		t.Fatalf("output does not say the data went:\n%s", out)
	}
}

func TestUninstallRefusesRoot(t *testing.T) {
	mac := &fakeMac{}
	env, _, _ := newUninstallEnv(mac, "")
	env.uid = 0
	if err := runUninstall([]string{"--yes"}, env); err == nil || !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("err = %v, want a refusal that mentions sudo", err)
	}
	if mac.planned {
		t.Fatal("planned an uninstall as root")
	}
}

func TestUninstallCancelledIsNotAnError(t *testing.T) {
	mac := &fakeMac{runErr: uninstall.ErrCancelled}
	env, out, _ := newUninstallEnv(mac, "")
	if err := runUninstall([]string{"--yes"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Nothing was removed") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestUninstallGUIReportsInAlerts(t *testing.T) {
	tests := []struct {
		name     string
		mac      fakeMac
		wantErr  bool
		alert    string // "" = no alert
		critical bool
	}{
		{"success", fakeMac{}, false, "ClaudeQ was removed", false},
		{"password prompt dismissed", fakeMac{runErr: uninstall.ErrCancelled}, false, "", false},
		{"removal fails", fakeMac{runErr: errors.New("rm: denied")}, true, "ClaudeQ could not be removed", true},
		{"plan fails", fakeMac{planErr: errors.New("refusing")}, true, "ClaudeQ could not be removed", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mac := tc.mac
			// No stdin: the GUI mode must never wait for an answer.
			env, out, alerts := newUninstallEnv(&mac, "")
			err := runUninstall([]string{"--gui"}, env)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error=%v", err, tc.wantErr)
			}
			if tc.alert == "" {
				if len(*alerts) != 0 {
					t.Fatalf("unexpected alert %+v", *alerts)
				}
				return
			}
			if len(*alerts) != 1 || (*alerts)[0].title != tc.alert || (*alerts)[0].critical != tc.critical {
				t.Fatalf("alerts = %+v, want one %q (critical=%v)", *alerts, tc.alert, tc.critical)
			}
			if out.Len() != 0 {
				t.Fatalf("GUI mode wrote to the terminal:\n%s", out)
			}
		})
	}
}

// The uninstall must not open the store: opening it creates the data directory
// that a purge is about to delete.
func TestUninstallDoesNotCreateDataDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "claudeq")
	t.Setenv(store.EnvHome, home)
	_ = run([]string{"uninstall", "--no-such-flag"})
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("data directory was created (stat err = %v)", err)
	}
}
