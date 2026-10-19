// Package version reports version information for the dispatchq binaries.
package version

// Version is the dispatchq release version. It defaults to "dev" for local and
// CI builds and is overridden at release build time via
// -ldflags "-X github.com/danielmaier42/claudeq/internal/version.Version=vX.Y.Z".
var Version = "dev"

// String returns the human-readable version string.
func String() string {
	return Version
}
