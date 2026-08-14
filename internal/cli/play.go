package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/challenges"
	"github.com/agarwalvivek29/kubedrill/internal/engine"
	"github.com/agarwalvivek29/kubedrill/internal/providers/kind"
	"github.com/agarwalvivek29/kubedrill/internal/sources/file"
	"github.com/agarwalvivek29/kubedrill/internal/store"
	"github.com/agarwalvivek29/kubedrill/internal/verify"
)

func newEngine(s *store.Store) *engine.Engine {
	return engine.New(kind.New(), s)
}

func newSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// resolveChallengeDir returns a real directory for a challenge arg: an on-disk
// path if it exists, otherwise a built-in challenge materialized under
// ~/.kubedrill/challenges.
func resolveChallengeDir(arg string) (string, error) {
	if fi, err := os.Stat(arg); err == nil && fi.IsDir() {
		return arg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// Built-ins win (curated), then installed packs (Story 4.2).
	if challenges.Has(arg) {
		return challenges.Materialize(arg, filepath.Join(home, ".kubedrill", "challenges"))
	}
	if dir, _, ok := file.Resolve(filepath.Join(home, ".kubedrill", "store"), arg); ok {
		return dir, nil
	}
	// Not found — produce a helpful error listing what IS available.
	names := make([]string, 0, len(availableChallenges()))
	for _, e := range availableChallenges() {
		names = append(names, e.Name)
	}
	return "", fmt.Errorf("no challenge %q found (have: %v). See `kubedrill catalog`", arg, names)
}

func newStartCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "start <challenge>",
		Short: "Provision a challenge and hand you the cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := defaultStore()
			if err != nil {
				return err
			}
			if live := s.LiveCount(); live >= resourcePressureThreshold && !force {
				return fmt.Errorf("%d sessions already have running clusters (each ~2 GB RAM); stop one, or re-run with --force", live)
			}
			dir, err := resolveChallengeDir(args[0])
			if err != nil {
				return err
			}
			id := newSessionID()
			eng := newEngine(s)
			prog := func(format string, a ...any) {
				fmt.Fprintf(cmd.ErrOrStderr(), "  … "+format+"\n", a...)
			}
			res, err := eng.Start(cmd.Context(), dir, id, prog)
			if err != nil {
				return err
			}
			printObjectives(cmd, res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "start even when other sessions are active")
	return cmd
}

func printObjectives(cmd *cobra.Command, res *engine.StartResult) {
	out := cmd.OutOrStdout()
	ch := res.Challenge
	fmt.Fprintf(out, "\n%s  [%s]\n", ch.Metadata.Title, ch.Metadata.Difficulty)
	if ch.Metadata.Description != "" {
		fmt.Fprintf(out, "\n%s\n", ch.Metadata.Description)
	}
	fmt.Fprintf(out, "Objectives:\n")
	for _, o := range ch.Objectives {
		fmt.Fprintf(out, "  • [%d pts] %s\n", o.Points, o.Title)
	}
	if res.Deadline != nil {
		fmt.Fprintf(out, "\nTime limit: %s (deadline %s)\n",
			ch.Metadata.TimeLimit, res.Deadline.Local().Format(time.Kitchen))
	}
	fmt.Fprintf(out, "\nStart solving:\n  export KUBECONFIG=%s\n", res.KubeconfigPath)
	fmt.Fprintf(out, "Then check your work:\n  kubedrill verify -s %s\n", res.SessionID)
}

func newVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify [session]",
		Short: "Grade your solution",
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
			card, late, err := newEngine(s).Verify(cmd.Context(), id)
			if err != nil {
				return err
			}
			printScorecard(cmd, card, late)
			if !card.AllPassed || (card.Failed && !card.Advisory) {
				// Exit 1: objectives still failing, or a fail-penalty/tamper rule
				// tripped (advisory rules are informational and never fail).
				return errObjectivesFailing
			}
			return nil
		},
	}
	return cmd
}

func printScorecard(cmd *cobra.Command, card *verify.Scorecard, late bool) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	for _, o := range card.Objectives {
		mark := "✗"
		switch {
		case o.Passed:
			mark = "✓"
		case o.Errored:
			mark = "!"
		}
		line := fmt.Sprintf("  %s [%d pts] %s", mark, o.Points, o.Title)
		if !o.Passed && o.Reason != "" {
			line += "  — " + o.Reason
		}
		fmt.Fprintln(out, line)
	}
	// Rule violations (Epic 3): shown with the evidence that triggered them.
	if len(card.RuleViolations) > 0 {
		heading := "\nRule violations:"
		if card.Advisory {
			heading = "\nRule violations (advisory — node access; informational only):"
		}
		fmt.Fprintln(out, heading)
		for _, v := range card.RuleViolations {
			penalty := fmt.Sprintf("−%d", v.Points)
			if v.Fail {
				penalty = "FAIL"
			}
			fmt.Fprintf(out, "  ✗ [%s] %s (%s)  — %s\n", v.Type, v.RuleID, penalty, v.Message)
			for _, ev := range v.Evidence {
				where := ev.Resource
				if ev.Namespace != "" {
					where += " " + ev.Namespace + "/" + ev.Name
				} else if ev.Name != "" {
					where += " " + ev.Name
				}
				fmt.Fprintf(out, "      • %s %s (as %s)\n", ev.Verb, where, ev.User)
			}
		}
	}

	fmt.Fprintf(out, "\nObjectives: %d/%d", card.Score, card.MaxScore)
	if card.HintPenalty > 0 {
		fmt.Fprintf(out, "   Hint penalty: −%d", card.HintPenalty)
	}
	if card.RulePenalty > 0 {
		fmt.Fprintf(out, "   Rule penalty: −%d", card.RulePenalty)
	}
	fmt.Fprintf(out, "\nScore: %d/%d", card.NetScore(), card.MaxScore)
	if late {
		fmt.Fprint(out, "  (late — past the deadline; recorded score unaffected)")
	}
	fmt.Fprintln(out)
	switch {
	case card.Failed && !card.Advisory:
		fmt.Fprintln(out, "Challenge failed: an integrity/fail rule was violated. 🚫")
	case card.AllPassed:
		fmt.Fprintln(out, "All objectives passed. 🎉")
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [session]",
		Short: "Show timer and progress for a session",
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
			st, err := s.Load(id)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Session:   %s\n", st.ID)
			fmt.Fprintf(out, "Challenge: %s\n", st.Challenge.Name)
			fmt.Fprintf(out, "Phase:     %s\n", st.Phase)
			fmt.Fprintf(out, "Best score: %d\n", st.BestScore)
			if st.Deadline != nil {
				rem := time.Until(*st.Deadline).Round(time.Second)
				if rem < 0 {
					fmt.Fprintf(out, "Time:      expired\n")
				} else {
					fmt.Fprintf(out, "Time left: %s\n", rem)
				}
			}
			if len(st.HintsUsed) > 0 {
				fmt.Fprintf(out, "Hints used: %d (%v)\n", len(st.HintsUsed), st.HintsUsed)
			}
			if n := len(st.Attempts); n > 0 {
				fmt.Fprintf(out, "Attempts:  %d (last score %d)\n", n, st.Attempts[n-1].Score)
			}
			return nil
		},
	}
}

// errObjectivesFailing makes `verify` exit non-zero while still printing the
// scorecard. Execute() maps any error to exit 2 today; a dedicated code for
// this case is refined with the exit-code taxonomy.
var errObjectivesFailing = fmt.Errorf("objectives still failing")
