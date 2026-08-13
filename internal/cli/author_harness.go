package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/internal/author"
	"github.com/agarwalvivek29/kubedrill/internal/providers/kind"
)

// newAuthorTestCmd runs the author-test correctness harness (Story 2.4, AD-11,
// FR-15): it provisions a throwaway kind cluster and proves the challenge fails
// unsolved (per objective, ignoring dependsOn), passes once solved, and stays
// passing on a second verify. Business logic is in internal/author; this is
// wiring + printing (AD-8).
func newAuthorTestCmd() *cobra.Command {
	var output string
	var keep bool
	cmd := &cobra.Command{
		Use:   "test <dir>",
		Short: "Prove a challenge is solvable and non-vacuous on a throwaway cluster",
		Long: "Provision a throwaway kind cluster and run three phases:\n" +
			"  negative    — every objective must FAIL on the fresh environment,\n" +
			"                evaluated directly (dependsOn ignored) so a vacuous\n" +
			"                objective cannot hide behind an unmet dependency;\n" +
			"                objectives marked expectInitiallyPassing must PASS.\n" +
			"  positive    — the reference solve.sh must drive verify to 100%.\n" +
			"  idempotency — a second verify must still pass.\n\n" +
			"Requires Docker. The cluster is always torn down afterward (use --keep\n" +
			"to leave it for debugging). `-o json` emits the full report for CI/agents.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthorTest(cmd, args[0], output, keep)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text|json")
	cmd.Flags().BoolVar(&keep, "keep", false, "leave the throwaway cluster running after the run (for debugging)")
	return cmd
}

func runAuthorTest(cmd *cobra.Command, dir, output string, keep bool) error {
	if output != "" && output != "text" && output != "json" {
		return fmt.Errorf("unknown output format %q (want text|json)", output)
	}

	// Progress only in text mode (JSON must own stdout); route it to stderr.
	prog := author.ProgressFunc(func(format string, a ...any) {})
	if output == "" || output == "text" {
		prog = func(format string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), format, a...) }
	}

	report, err := author.Test(cmd.Context(), kind.New(), dir, author.TestOptions{KeepEnv: keep}, prog)
	if err != nil {
		// The harness could not run (e.g. provisioning failed) — a distinct
		// failure from "the challenge ran and failed".
		return err
	}

	if output == "json" {
		enc, merr := json.MarshalIndent(report, "", "  ")
		if merr != nil {
			return merr
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(enc))
	} else {
		printTestReport(cmd, report)
	}

	if !report.Passed {
		return errAuthorTestFailed
	}
	return nil
}

// errAuthorTestFailed makes `author test` exit non-zero when the challenge ran
// but failed a phase; the detailed report is already printed.
var errAuthorTestFailed = fmt.Errorf("author test failed")

func printTestReport(cmd *cobra.Command, r *author.TestReport) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	printPhase(cmd, r.Negative)
	printPhase(cmd, r.Positive)
	printPhase(cmd, r.Idempotency)
	fmt.Fprintln(out)
	if r.Passed {
		fmt.Fprintf(out, "✓ %s is solvable and non-vacuous\n", r.Challenge)
	} else {
		fmt.Fprintf(out, "✗ %s did not pass author test\n", r.Challenge)
	}
}

func printPhase(cmd *cobra.Command, p author.PhaseReport) {
	out := cmd.OutOrStdout()
	mark := "✓"
	switch {
	case p.Skipped:
		mark = "–"
	case !p.Passed:
		mark = "✗"
	}
	fmt.Fprintf(out, "  %s %-12s %s\n", mark, p.Name, p.Detail)
	for _, v := range p.Violations {
		fmt.Fprintf(out, "      • %s (%s): %s\n", v.ObjectiveID, v.Title, v.Reason)
	}
}
