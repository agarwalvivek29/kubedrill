package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/internal/providers/kind"
	"github.com/agarwalvivek29/kubedrill/internal/store"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// resourcePressureThreshold: warn once this many sessions are already active,
// so starting another (a 3rd) is a conscious choice on default Docker VMs (NFR-4, F1).
const resourcePressureThreshold = 2

func defaultStore() (*store.Store, error) {
	dir, err := store.DefaultDir()
	if err != nil {
		return nil, err
	}
	return store.New(dir), nil
}

// resolveSession returns the target session id: the explicit arg, else the
// current session, else an error.
func resolveSession(s *store.Store, args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	cur, err := s.Current()
	if err != nil {
		return "", err
	}
	if cur == "" {
		return "", fmt.Errorf("no session specified and no current session set")
	}
	return cur, nil
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := defaultStore()
			if err != nil {
				return err
			}
			states, err := s.List()
			if err != nil {
				return err
			}
			cur, _ := s.Current()
			out := cmd.OutOrStdout()
			if len(states) == 0 {
				fmt.Fprintln(out, "No sessions. Start one with: kubedrill start <challenge>")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "SESSION\tCHALLENGE\tPHASE\tSCORE\t")
			active := 0
			for _, st := range states {
				marker := ""
				if st.ID == cur {
					marker = " *"
				}
				if st.Phase == api.PhaseCreating || st.Phase == api.PhaseRunning {
					active++
				}
				fmt.Fprintf(tw, "%s%s\t%s\t%s\t%d\t\n", st.ID, marker, st.Challenge.Name, st.Phase, st.BestScore)
			}
			tw.Flush()
			if active >= resourcePressureThreshold {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"\nnote: %d active sessions — each uses ~2 GB RAM; starting more may strain a default Docker VM\n", active)
			}
			return nil
		},
	}
}

func newEnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "env [session]",
		Short: "Print an eval-able KUBECONFIG export for a session",
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
			if _, err := s.Load(id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "export KUBECONFIG=%s\n", s.KubeconfigPath(id))
			return nil
		},
	}
}

func newShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell [session]",
		Short: "Open a subshell with KUBECONFIG set to a session",
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
			if _, err := s.Load(id); err != nil {
				return err
			}
			shell := os.Getenv("SHELL")
			if shell == "" {
				shell = "/bin/sh"
			}
			sh := exec.CommandContext(cmd.Context(), shell)
			sh.Env = append(os.Environ(), "KUBECONFIG="+s.KubeconfigPath(id), "KUBEDRILL_SESSION="+id)
			sh.Stdin, sh.Stdout, sh.Stderr = os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr()
			fmt.Fprintf(cmd.ErrOrStderr(), "kubedrill: subshell for session %s (exit to return)\n", id)
			return sh.Run()
		},
	}
}

func newStopCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "stop [session]",
		Short: "Tear down a session's cluster",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := defaultStore()
			if err != nil {
				return err
			}
			prov := kind.New()
			out := cmd.OutOrStdout()

			var ids []string
			if all {
				states, err := s.List()
				if err != nil {
					return err
				}
				for _, st := range states {
					ids = append(ids, st.ID)
				}
			} else {
				id, err := resolveSession(s, args)
				if err != nil {
					return err
				}
				ids = []string{id}
			}
			for _, id := range ids {
				if err := prov.Destroy(cmd.Context(), id); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warn: destroy cluster for %s: %v\n", id, err)
				}
				if err := s.Remove(id); err != nil {
					return err
				}
				fmt.Fprintf(out, "stopped %s\n", id)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "stop all sessions")
	return cmd
}

func newPruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Reconcile clusters against sessions and remove orphans",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := defaultStore()
			if err != nil {
				return err
			}
			prov := kind.New()
			out := cmd.OutOrStdout()

			removed := pruneOrphans(cmd.Context(), prov, s)
			for _, line := range removed {
				fmt.Fprintln(out, line)
			}
			if len(removed) == 0 {
				fmt.Fprintln(out, "nothing to prune")
			}
			return nil
		},
	}
}

// pruneOrphans destroys provider clusters with no session dir, and removes
// session dirs whose cluster is already gone. Returns human-readable lines.
func pruneOrphans(ctx context.Context, prov api.EnvProvider, s *store.Store) []string {
	var lines []string

	sessions := map[string]bool{}
	if states, err := s.List(); err == nil {
		for _, st := range states {
			sessions[st.ID] = true
		}
	}

	clusters := map[string]bool{}
	if infos, err := prov.List(ctx); err == nil {
		for _, i := range infos {
			clusters[i.ID] = true
			if !sessions[i.ID] {
				// Cluster with no session dir → orphan from a crashed start.
				if err := prov.Destroy(ctx, i.ID); err == nil {
					lines = append(lines, "destroyed orphan cluster "+i.ID)
				}
			}
		}
	}
	// Session dirs whose cluster is gone → stale state to remove.
	for id := range sessions {
		if !clusters[id] {
			if err := s.Remove(id); err == nil {
				lines = append(lines, "removed stale session "+id)
			}
		}
	}
	return lines
}
