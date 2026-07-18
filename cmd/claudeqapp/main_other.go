//go:build !darwin

package main

import (
	"fmt"
	"os"
)

// The native window targets macOS. This stub keeps non-darwin builds compiling.
func main() {
	fmt.Fprintln(os.Stderr, "claudeqapp is macOS-only")
	os.Exit(1)
}
