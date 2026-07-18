package api

import (
	"context"
	"strings"

	"github.com/danielmaier42/claudeq/internal/system"
)

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
