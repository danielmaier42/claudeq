package app

import (
	"testing"

	"github.com/danielmaier42/claudeq/internal/store"
)

func TestValidateNotificationsIgnoresDisabledChannels(t *testing.T) {
	// Every channel is switched off and half-filled in (a token with no user
	// key, a topic-less server, a template with no URL): none of that may block
	// saving an unrelated setting.
	s := store.Settings{
		Pushover: store.Pushover{Token: "tok"},
		Ntfy:     store.Ntfy{Server: "ntfy.example.com"},
		Webhook:  store.Webhook{Template: `{"text":"{{title}}"}`},
	}
	if err := ValidateNotifications(s); err != nil {
		t.Fatalf("disabled channels must not be validated: %v", err)
	}
}

func TestValidateNotificationsRejectsIncompleteEnabledChannel(t *testing.T) {
	cases := []struct {
		name string
		s    store.Settings
	}{
		{"pushover missing user key", store.Settings{Pushover: store.Pushover{Enabled: true, Token: "tok"}}},
		{"ntfy missing topic", store.Settings{Ntfy: store.Ntfy{Enabled: true, Server: "https://ntfy.sh"}}},
		{"webhook missing url", store.Settings{Webhook: store.Webhook{Enabled: true}}},
		{"webhook unknown placeholder", store.Settings{Webhook: store.Webhook{
			Enabled: true, URL: "https://example.com/hook", Template: `{"x":"{{titel}}"}`,
		}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateNotifications(c.s); err == nil {
				t.Fatal("expected an error for an enabled but incomplete channel")
			}
		})
	}
}

func TestValidateNotificationsAcceptsCompleteEnabledChannels(t *testing.T) {
	s := store.Settings{
		Pushover: store.Pushover{Enabled: true, Token: "tok", UserKey: "usr"},
		Ntfy:     store.Ntfy{Enabled: true, Topic: "claudeq"},
		Webhook:  store.Webhook{Enabled: true, URL: "https://hooks.example.com/x"},
	}
	if err := ValidateNotifications(s); err != nil {
		t.Fatalf("fully configured channels must pass: %v", err)
	}
}
