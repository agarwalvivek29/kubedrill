package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/internal/author"
	"github.com/agarwalvivek29/kubedrill/internal/challenge"
)

// newAuthorLintCmd runs opinionated quality/safety rules over a challenge
// (Story 2.3, FR-14, AD-5/AD-11). It first loads the directory (the full
// offline validation of `author validate`) — a challenge must be structurally
// valid to be linted — then applies author.Lint. Business logic lives in
// internal/author; this is wiring + printing (AD-8).
func newAuthorLintCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "lint <dir>",
		Short: "Lint a challenge for quality and safety",
		Long: "Check a challenge against opinionated quality/safety rules: at least\n" +
			"one hint, no trivially-vacuous checks, enforce rules are scoped, and no\n" +
			"field-level require on Secrets. lint first runs the full offline\n" +
			"validation (`author validate`) — a structurally invalid challenge (e.g.\n" +
			"missing difficulty, or enforce:true on a require rule) fails there\n" +
			"before the quality rules run. `-o json` emits findings on stdout.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthorLint(cmd, args[0], output)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text|json")
	return cmd
}

// lintResult is the machine-readable verdict emitted by `-o json`.
type lintResult struct {
	Dir      string           `json:"dir"`
	Clean    bool             `json:"clean"`
	Findings []author.Finding `json:"findings"`
	Error    string           `json:"error,omitempty"`
}

func runAuthorLint(cmd *cobra.Command, dir, output string) error {
	if output != "" && output != "text" && output != "json" {
		return fmt.Errorf("unknown output format %q (want text|json)", output)
	}

	loaded, loadErr := challenge.LoadDir(dir)
	var findings []author.Finding
	if loadErr == nil {
		findings = author.Lint(loaded.Challenge)
	}

	if output == "json" {
		res := lintResult{Dir: dir, Clean: loadErr == nil && len(findings) == 0, Findings: findings}
		if loadErr != nil {
			res.Error = loadErr.Error()
		}
		enc, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(enc))
		return lintExitErr(loadErr, findings)
	}

	// Text mode.
	if loadErr != nil {
		// Structural failure — surface it and point at validate.
		return fmt.Errorf("%w\n(challenge must be structurally valid to lint; run `kubedrill author validate %s`)", loadErr, dir)
	}
	out := cmd.OutOrStdout()
	if len(findings) == 0 {
		fmt.Fprintf(out, "✓ %s: no lint findings\n", loaded.Challenge.Metadata.Name)
		return nil
	}
	for _, f := range findings {
		fmt.Fprintf(out, "✗ [%s] %s\n", f.Rule, f.Message)
	}
	return lintExitErr(nil, findings)
}

// lintExitErr maps load/lint outcome to the command error (non-zero exit). The
// detailed findings are already printed; the returned error is a concise
// stderr summary.
func lintExitErr(loadErr error, findings []author.Finding) error {
	if loadErr != nil {
		return loadErr
	}
	if len(findings) > 0 {
		return fmt.Errorf("%d lint finding(s)", len(findings))
	}
	return nil
}
