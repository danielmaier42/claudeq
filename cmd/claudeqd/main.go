// Command claudeqd is the claudeq background daemon.
//
// This is the bootstrap skeleton (build phase 0): it wires the entry point,
// version reporting, and the quality toolchain. The scheduler, executor, and
// reactive limit gate arrive in build phase 2 — see PLAN.md.
package main

import (
	"flag"
	"fmt"

	"github.com/danielmaier42/claudeq/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the claudeqd version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	// The daemon runtime is not implemented yet (see PLAN.md build phases).
	fmt.Println("claudeqd " + version.String() + ": runtime not implemented yet (see PLAN.md)")
}
