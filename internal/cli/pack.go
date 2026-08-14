package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/challenges"
	"github.com/agarwalvivek29/kubedrill/internal/sources/file"
)

// storeRoot returns ~/.kubedrill/store, the installed-pack root (AD-13).
func storeRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kubedrill", "store"), nil
}

// newInstallCmd installs a challenge pack from a directory or tarball into the
// shared pack store (Story 4.2, FR-20).
func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <path>",
		Short: "Install a challenge pack from a directory or tarball",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := storeRoot()
			if err != nil {
				return err
			}
			p, err := file.Install(args[0], root)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Installed pack %q v%s (%d challenge(s)) to %s\n", p.Name, p.Version, len(p.Challenges), p.Dir)
			fmt.Fprintf(out, "  challenges: %v\n", p.Challenges)
			fmt.Fprintf(out, "Play one with: kubedrill start %s\n", p.Challenges[0])
			return nil
		},
	}
}

// catalogEntry is one available challenge (built-in or from an installed pack).
type catalogEntry struct {
	Name   string `json:"name"`
	Source string `json:"source"` // "builtin" or "pack:<name>@<version>"
}

// newCatalogCmd lists the challenges available to play: the embedded built-ins
// plus every challenge from an installed pack.
func newCatalogCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "List available challenges (built-ins + installed packs)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries := availableChallenges()
			if output == "json" {
				enc, err := json.MarshalIndent(entries, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(enc))
				return nil
			}
			out := cmd.OutOrStdout()
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "CHALLENGE\tSOURCE\t")
			for _, e := range entries {
				fmt.Fprintf(tw, "%s\t%s\t\n", e.Name, e.Source)
			}
			tw.Flush()
			fmt.Fprintf(cmd.ErrOrStderr(), "\n%d challenge(s). Play one with: kubedrill start <challenge>\n", len(entries))
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text|json")
	return cmd
}

// availableChallenges returns the built-ins plus installed-pack challenges,
// sorted by name (built-ins win a name collision, matching resolution order).
func availableChallenges() []catalogEntry {
	seen := map[string]bool{}
	var entries []catalogEntry
	for _, n := range challenges.Names() {
		entries = append(entries, catalogEntry{Name: n, Source: "builtin"})
		seen[n] = true
	}
	if root, err := storeRoot(); err == nil {
		if packs, err := file.Installed(root); err == nil {
			for _, p := range packs {
				for _, c := range p.Challenges {
					if seen[c] {
						continue
					}
					seen[c] = true
					entries = append(entries, catalogEntry{Name: c, Source: fmt.Sprintf("pack:%s@%s", p.Name, p.Version)})
				}
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}
