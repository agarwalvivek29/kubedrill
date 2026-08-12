package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newHintCmd() *cobra.Command {
	var peek bool
	cmd := &cobra.Command{
		Use:   "hint [session]",
		Short: "Reveal the next hint (or peek at its cost)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := defaultStore()
			if err != nil {
				return err
			}
			id, err := resolveSession(s, args)
			if err != nil {
				return err
			}
			eng := newEngine(s)
			out := cmd.OutOrStdout()

			if peek {
				h, remaining, ok, err := eng.NextHint(id)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "No more hints for this challenge.")
					return nil
				}
				fmt.Fprintf(out, "Next hint (%s) costs %d points; %d hint(s) left.\n", h.ID, h.Penalty, remaining)
				fmt.Fprintln(out, "Reveal it with: kubedrill hint")
				return nil
			}

			h, err := eng.RevealHint(id)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Hint %s (−%d points):\n  %s\n", h.ID, h.Penalty, h.Text)
			return nil
		},
	}
	cmd.Flags().BoolVar(&peek, "peek", false, "show the next hint's penalty without revealing it")
	return cmd
}
