package cli

import (
	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/internal/schema"
)

// newAuthorCmd is the parent for the authoring toolchain. Only `schema --print`
// lands in Story 1.2 (it emits the frozen contract, FR-13); new/validate/lint/
// test arrive in Epic 2.
func newAuthorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "author",
		Short: "Author and validate challenges",
	}
	cmd.AddCommand(newAuthorSchemaCmd())
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
