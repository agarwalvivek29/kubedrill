// Command kubedrill is the entry point for the kubedrill CLI.
//
// Per AD-8 (CLI is a thin adapter), this package does wiring only: it
// constructs the cobra command tree and delegates all behavior to the engine.
// No business logic lives here.
package main

import (
	"os"

	"github.com/agarwalvivek29/kubedrill/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
