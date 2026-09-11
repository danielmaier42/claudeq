package app

import (
	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/store"
)

// ValidateNotifications checks every switched-on channel's configuration. A
// switched-off channel is not checked: its fields are inert, and half-filled
// notes someone left behind must not block saving an unrelated setting. The
// check runs again when the channel is switched on, because that is a settings
// write too.
func ValidateNotifications(s store.Settings) error {
	if s.Pushover.Enabled {
		if err := notify.ValidatePushover(s.Pushover.Token, s.Pushover.UserKey); err != nil {
			return err
		}
	}
	if s.Ntfy.Enabled {
		if err := notify.ValidateNtfy(s.Ntfy.Server, s.Ntfy.Topic); err != nil {
			return err
		}
	}
	if s.Webhook.Enabled {
		if err := notify.ValidateWebhook(s.Webhook.URL, s.Webhook.Template); err != nil {
			return err
		}
	}
	return nil
}
