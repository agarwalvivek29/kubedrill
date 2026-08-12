package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/challenge"
)

// newAuthorValidateCmd runs the full offline loader over a challenge directory
// (Story 2.2, FR-14): strict decode, semantic validation (unique ids, acyclic
// dependsOn, exactly-one-of unions), referential checks (referenced files
// exist), and match/CEL compilation — with no Docker and no cluster. The loader
// (challenge.LoadDir) is the single validation path; this command is thin
// wiring + printing (AD-8).
func newAuthorValidateCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "validate <dir>",
		Short: "Validate a challenge directory without a cluster",
		Long: "Run strict decode, semantic validation (unique ids, acyclic dependsOn,\n" +
			"exactly-one-of unions), referential checks (referenced files exist), and\n" +
			"match/CEL compilation over a challenge directory. Needs no Docker and no\n" +
			"cluster, so it is fast enough to run on every save. Failures name the\n" +
			"offending file and field; `-o json` emits a machine-readable verdict on\n" +
			"stdout.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthorValidate(cmd, args[0], output)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text|json")
	return cmd
}

// validateResult is the machine-readable verdict emitted by `-o json`.
type validateResult struct {
	Dir        string `json:"dir"`
	Valid      bool   `json:"valid"`
	Name       string `json:"name,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Objectives int    `json:"objectives"`
	Checks     int    `json:"checks"`
	Faults     int    `json:"faults"`
	Hints      int    `json:"hints"`
	Error      string `json:"error,omitempty"`
}

func runAuthorValidate(cmd *cobra.Command, dir, output string) error {
	loaded, loadErr := challenge.LoadDir(dir)

	switch output {
	case "", "text":
		if loadErr != nil {
			// Returned to Execute, which prints "Error: …" to stderr and exits 2.
			return loadErr
		}
		printValidateSummary(cmd, dir, loaded.Challenge)
		return nil
	case "json":
		// Data always goes to stdout so scripts can parse it; on failure the
		// error is *also* returned so stderr carries a human message and the
		// process exits non-zero. stdout stays clean, parseable JSON either way.
		res := buildValidateResult(dir, loaded, loadErr)
		enc, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(enc))
		return loadErr
	default:
		return fmt.Errorf("unknown output format %q (want text|json)", output)
	}
}

func printValidateSummary(cmd *cobra.Command, dir string, c *v1alpha1.Challenge) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "✓ %s is valid (%s)\n", c.Metadata.Name, c.APIVersion)
	fmt.Fprintf(out, "  %s · %s · %s · %s · no cluster needed\n",
		plural(len(c.Objectives), "objective"),
		plural(countChecks(c), "check"),
		plural(len(c.Environment.Setup.Faults), "fault"),
		plural(len(c.Hints), "hint"),
	)
}

func buildValidateResult(dir string, loaded *challenge.Loaded, loadErr error) validateResult {
	res := validateResult{Dir: dir, Valid: loadErr == nil}
	if loadErr != nil {
		res.Error = loadErr.Error()
		return res
	}
	c := loaded.Challenge
	res.Name = c.Metadata.Name
	res.APIVersion = c.APIVersion
	res.Objectives = len(c.Objectives)
	res.Checks = countChecks(c)
	res.Faults = len(c.Environment.Setup.Faults)
	res.Hints = len(c.Hints)
	return res
}

// countChecks counts the checks an author wrote across all objectives. anyOf
// leaves are not double-counted — an anyOf is one check with alternatives.
func countChecks(c *v1alpha1.Challenge) int {
	n := 0
	for _, o := range c.Objectives {
		n += len(o.Checks)
	}
	return n
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
