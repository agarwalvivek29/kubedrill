package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/agarwalvivek29/kubedrill/internal/challenge"
	"github.com/agarwalvivek29/kubedrill/internal/imagecache"
)

func newImagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "images",
		Short: "Manage the local image cache",
	}
	cmd.AddCommand(newImagesPullCmd())
	return cmd
}

func newImagesPullCmd() *cobra.Command {
	var k8sVersion string
	cmd := &cobra.Command{
		Use:   "pull [challenge-dir | image-ref...]",
		Short: "Pre-cache images for offline play",
		Long: "Pull images into the local cache so a challenge can run fully offline.\n" +
			"Pass a challenge directory to cache its declared images, or one or more\n" +
			"image references directly.",
		RunE: func(cmd *cobra.Command, args []string) error {
			refs, err := resolveImageRefs(args)
			if err != nil {
				return err
			}
			if k8sVersion != "" {
				// Node-image prewarm is a no-op in v0.1 (kind uses its default
				// node image); surfaced so the flag is honest, not silently ignored.
				fmt.Fprintf(cmd.ErrOrStderr(),
					"note: --k8s-version %s recorded; node-image prewarm arrives with version pinning\n", k8sVersion)
			}
			if len(refs) == 0 {
				return fmt.Errorf("nothing to pull: pass a challenge directory or image refs")
			}

			dir, err := imagecache.DefaultDir()
			if err != nil {
				return err
			}
			cache := imagecache.New(dir, imagecache.CLIDocker{})
			out := cmd.OutOrStdout()
			for _, ref := range refs {
				if cache.Has(ref) {
					fmt.Fprintf(out, "cached  %s\n", ref)
					continue
				}
				fmt.Fprintf(out, "pulling %s ...\n", ref)
				if _, err := cache.Pull(cmd.Context(), ref); err != nil {
					return err
				}
				fmt.Fprintf(out, "done    %s\n", ref)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&k8sVersion, "k8s-version", "", "Kubernetes version to prewarm the node image for")
	return cmd
}

// resolveImageRefs turns args into a concrete image-ref list: a single existing
// directory is treated as a challenge (its declared images), otherwise args are
// taken as raw image refs.
func resolveImageRefs(args []string) ([]string, error) {
	if len(args) == 1 {
		if fi, err := os.Stat(args[0]); err == nil && fi.IsDir() {
			return challengeImages(args[0])
		}
	}
	return args, nil
}

func challengeImages(dir string) ([]string, error) {
	// A bare challenge dir or a path to its challenge.yaml both work.
	if filepath.Base(dir) == "challenge.yaml" {
		dir = filepath.Dir(dir)
	}
	loaded, err := challenge.LoadDir(dir)
	if err != nil {
		return nil, err
	}
	return loaded.Challenge.Environment.Images, nil
}
