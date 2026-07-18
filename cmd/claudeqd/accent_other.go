//go:build !darwin

package main

// startAccentNotifications is a no-op off macOS (the daemon targets macOS; this
// keeps non-darwin builds compiling).
func startAccentNotifications(func()) {}
