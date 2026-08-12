package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/internal/author"
	"github.com/agarwalvivek29/kubedrill/internal/schema"
)

// newAuthorCmd is the parent for the authoring toolchain. `schema --print`
// landed in Story 1.2 (it emits the frozen contract, FR-13), `new` scaffolds a
// challenge (Story 2.1) and `validate` checks one without a cluster (Story 2.2);
// lint/test arrive in later Epic 2 stories.
func newAuthorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "author",
		Short: "Author and validate challenges",
	}
	cmd.AddCommand(newAuthorSchemaCmd())
	cmd.AddCommand(newAuthorNewCmd())
	cmd.AddCommand(newAuthorValidateCmd())
	return cmd
}

// newAuthorNewCmd scaffolds a challenge skeleton from the template (Story 2.1).
func newAuthorNewCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a new challenge directory from the template",
		Long: "Scaffold a complete, loadable challenge skeleton (challenge.yaml,\n" +
			"setup/, probes/, solution/ with SOLUTION.md and a solve.sh stub). The\n" +
			"name is used verbatim as the directory and namespace, so it must be a\n" +
			"DNS-1123 label. Edit the generated files, then validate with `author\n" +
			"validate` (Story 2.2).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			created, err := author.Scaffold(dir, args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Scaffolded challenge %q at %s\n", args[0], created)
			fmt.Fprintf(out, "Next: edit challenge.yaml and solution/solve.sh, then run `kubedrill author validate %s`.\n", created)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "parent directory to create the challenge in")
	return cmd
}

func newAuthorSchemaCmd() *cobra.Command {
	var print bool
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the challenge JSON Schema (kubedrill.dev/v1alpha1)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write(schema.ChallengeV1Alpha1())
			return err
		},
	}
	// --print is accepted for the documented `author schema --print` form; the
	// command prints regardless (it has no other mode yet).
	cmd.Flags().BoolVar(&print, "print", true, "print the embedded JSON Schema to stdout")
	return cmd
}
