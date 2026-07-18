package api

import (
	"context"
	"strings"
	"sync"

	"github.com/danielmaier42/claudeq/internal/system"
)

// ThemeHub broadcasts accent-color changes to Server-Sent Events subscribers, so
// the dashboard updates live without polling.
type ThemeHub struct {
	mu      sync.Mutex
	subs    map[chan string]struct{}
	current string
}

// NewThemeHub creates a hub seeded with the initial accent.
func NewThemeHub(initial string) *ThemeHub {
	return &ThemeHub{subs: map[chan string]struct{}{}, current: initial}
}

// Publish updates the current accent and notifies subscribers if it changed.
func (h *ThemeHub) Publish(accent string) {
	h.mu.Lock()
	if accent == h.current {
		h.mu.Unlock()
		return
	}
	h.current = accent
	subs := make([]chan string, 0, len(h.subs))
	for c := range h.subs {
		subs = append(subs, c)
	}
	h.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- accent:
		default: // subscriber is slow; it will get the next update
		}
	}
}

func (h *ThemeHub) subscribe() (<-chan string, func()) {
	c := make(chan string, 1)
	h.mu.Lock()
	h.subs[c] = struct{}{}
	c <- h.current // deliver the current value immediately
	h.mu.Unlock()
	return c, func() {
		h.mu.Lock()
		delete(h.subs, c)
		h.mu.Unlock()
	}
}

// macAccentHex maps macOS AppleAccentColor indices to hex colors matching the
// system accent palette.
var macAccentHex = map[string]string{
	"-1": "#8e8e93", // graphite
	"0":  "#ff5257", // red
	"1":  "#f7821b", // orange
	"2":  "#ffc600", // yellow
	"3":  "#62ba46", // green
	"4":  "#007aff", // blue
	"5":  "#8944ab", // purple
	"6":  "#f74f9e", // pink
}

// MacAccent returns a function that reads the user's macOS accent color
// (AppleAccentColor) and maps it to a hex color, or "" when it's unset/default
// (multicolor/blue) or unavailable — reliable across browsers, unlike the CSS
// AccentColor system color.
func MacAccent(r system.Runner) func() string {
	return func() string {
		out, err := r.Run(context.Background(), "defaults", "read", "-g", "AppleAccentColor")
		if err != nil {
			return ""
		}
		return macAccentHex[strings.TrimSpace(string(out))]
	}
}
