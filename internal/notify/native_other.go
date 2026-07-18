//go:build !darwin

package notify

import "errors"

// Native notifications are macOS-only; elsewhere callers fall back to osascript
// (which is itself a no-op off macOS, but keeps the interface uniform).
func nativeNotifyAvailable() bool { return false }

func requestNativeAuth() {}

func postNativeNotification(_, _ string) error { return errors.New("native notifications unsupported") }
