package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/internal/engine"
)

func newResetCmd() *cobra.Command {
	var hard, clock, hints bool
	cmd := &cobra.Command{
		Use:   "reset [session]",
		Short: "Restore the challenge to its broken starting state",
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
			prog := func(format string, a ...any) {
				fmt.Fprintf(cmd.ErrOrStderr(), "  … "+format+"\n", a...)
			}
			didHard, err := newEngine(s).Reset(cmd.Context(), id,
				engine.ResetOpts{Hard: hard, Clock: clock, Hints: hints}, prog)
			if err != nil {
				return err
			}
			mode := "fast"
			if didHard {
				mode = "hard (cluster recreated)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "reset %s — %s; challenge is broken again, keep going.\n", id, mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&hard, "hard", false, "recreate the cluster instead of resetting in place")
	cmd.Flags().BoolVar(&clock, "clock", false, "restart the timer")
	cmd.Flags().BoolVar(&hints, "hints", false, "clear revealed hints (practice mode)")
	return cmd
}
