package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/internal/sources/file"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// newPackCmd is the parent for pack subcommands (export; install is top-level).
func newPackCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "pack", Short: "Work with challenge packs"}
	cmd.AddCommand(newPackExportCmd())
	return cmd
}

// newPackExportCmd exports a pack (a directory or an installed pack name) to a
// portable tarball a teammate can `kubedrill install` (Story 4.2, FR-20).
func newPackExportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "export <dir|pack-name>",
		Short: "Export a pack to a portable tarball",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := storeRoot()
			if err != nil {
				return err
			}
			if out == "" {
				out = args[0] + ".tgz"
			}
			p, err := file.Export(args[0], root, out)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Exported pack %q v%s (%d challenge(s)) to %s\n", p.Name, p.Version, len(p.Challenges), out)
			fmt.Fprintf(cmd.OutOrStdout(), "Share it; a teammate installs with: kubedrill install %s\n", out)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "output tarball path (default <name>.tgz)")
	return cmd
}

// newSessionCmd is the parent for session subcommands (export).
func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "session", Short: "Inspect and export sessions"}
	cmd.AddCommand(newSessionExportCmd())
	return cmd
}

// sessionResult is the stable, diffable export of a session's outcome.
type sessionResult struct {
	Challenge        string          `json:"challenge"`
	ChallengeVersion string          `json:"challengeVersion,omitempty"`
	BestScore        int             `json:"bestScore"`
	AttemptCount     int             `json:"attemptCount"`
	HintsUsed        []string        `json:"hintsUsed,omitempty"`
	FinalObjectives  map[string]bool `json:"finalObjectives,omitempty"`
}

// newSessionExportCmd emits a session's result as JSON teammates can diff (score
// + per-objective outcome), independent of session id / timestamps.
func newSessionExportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "export [session]",
		Short: "Export a session's result as diffable JSON",
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
			res := sessionResult{
				Challenge:        st.Challenge.Name,
				ChallengeVersion: st.Challenge.Version,
				BestScore:        st.BestScore,
				AttemptCount:     len(st.Attempts),
				HintsUsed:        st.HintsUsed,
				FinalObjectives:  finalObjectives(st.Attempts),
			}
			enc, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return err
			}
			if out == "" {
				fmt.Fprintln(cmd.OutOrStdout(), string(enc))
				return nil
			}
			if err := os.WriteFile(out, append(enc, '\n'), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s (challenge %s, score %d/%d objectives). Diff it against a teammate's.\n",
				out, res.Challenge, countTrue(res.FinalObjectives), len(res.FinalObjectives))
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "write JSON to this file (default stdout)")
	return cmd
}

// finalObjectives returns the per-objective outcome from the last attempt.
func finalObjectives(attempts []api.Attempt) map[string]bool {
	if len(attempts) == 0 {
		return nil
	}
	return attempts[len(attempts)-1].Objectives
}

func countTrue(m map[string]bool) int {
	n := 0
	for _, v := range m {
		if v {
			n++
		}
	}
	return n
}
