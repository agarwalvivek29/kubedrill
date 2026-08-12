//go:build e2e

package imagecache

import (
	"context"
	"os"
	"testing"
)

// TestPullRealImageE2E pulls a tiny real image and asserts a non-empty tarball
// lands in the cache. Requires Docker. Run with: go test -tags e2e ./...
func TestPullRealImageE2E(t *testing.T) {
	c := New(t.TempDir(), CLIDocker{})
	ctx := context.Background()

	p, err := c.Pull(ctx, "busybox:1.36")
	if err != nil {
		t.Fatalf("pull busybox: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("cached tarball missing: %v", err)
	}
	if info.Size() < 1024 {
		t.Fatalf("cached tarball implausibly small: %d bytes", info.Size())
	}
	// Second pull must hit the cache (no error, same path).
	p2, err := c.Pull(ctx, "busybox:1.36")
	if err != nil || p2 != p {
		t.Fatalf("cached re-pull: path=%q err=%v", p2, err)
	}
}
