// Package cli wires the kubedrill command tree (AD-8: wiring + printing only).
//
// Commands constructed here delegate to the engine; they hold no business
// logic and no package-global engine state. Every command that produces
// structured results will support `-o json`, serialized from engine result
// types — the reserved future API surface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newRootCmd builds the root command tree. Subcommands are registered here as
// they land in later stories (start, verify, hint, reset, stop, author, ...).
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "kubedrill",
		Short:         "Local-first Kubernetes practice labs",
		Long:          "kubedrill turns any machine with Docker into a Kubernetes practice lab.\nChallenges are declarative manifests; the tool provisions a throwaway kind\ncluster, injects a fault, and grades your solution.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCmd())
	root.AddCommand(newAuthorCmd())
	root.AddCommand(newImagesCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newEnvCmd())
	root.AddCommand(newShellCmd())
	root.AddCommand(newStopCmd())
	root.AddCommand(newPruneCmd())
	return root
}

// Execute runs the root command and returns a process exit code.
//
// SilenceErrors/SilenceUsage are set on the root so cobra doesn't print
// errors itself; we print them here (once, to stderr) and map to an exit code.
// Exit codes are part of the CLI contract (LLD §9): 0 ok, 2 usage/general
// error. Richer codes (1 objectives failing, 3 environment, 4 capability
// refusal, 5 rule fail) are introduced by the commands that can produce them.
func Execute() int {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 2
	}
	return 0
}
