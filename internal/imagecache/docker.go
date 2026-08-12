package imagecache

import (
	"context"
	"fmt"
	"os/exec"
)

// CLIDocker implements Docker by shelling out to the `docker` CLI. Shelling
// out (rather than the Docker SDK) keeps the dependency surface small and the
// behavior identical to what a user would run by hand.
type CLIDocker struct{}

// Pull runs `docker pull <ref>`.
func (CLIDocker) Pull(ctx context.Context, ref string) error {
	return run(ctx, "docker", "pull", ref)
}

// Save runs `docker save <ref> -o <destPath>`.
func (CLIDocker) Save(ctx context.Context, ref, destPath string) error {
	return run(ctx, "docker", "save", ref, "-o", destPath)
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, string(out))
	}
	return nil
}
