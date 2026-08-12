// Package imagecache owns the shared image tarball cache under
// ~/.kubedrill/cache/images (AD-13). It is the sole writer of that directory:
// pulls serialize on a cache-scoped file lock and land via write-to-temp +
// atomic rename, so concurrent invocations can never tear the cache.
package imagecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// Docker is the slice of the container runtime the cache needs. Injectable so
// unit tests run without Docker.
type Docker interface {
	// Pull fetches an image ref into the local daemon.
	Pull(ctx context.Context, ref string) error
	// Save writes the image ref as an OCI/docker archive to destPath.
	Save(ctx context.Context, ref, destPath string) error
}

// Cache is a content-addressed image tarball store.
type Cache struct {
	root   string
	docker Docker
}

// DefaultDir returns ~/.kubedrill/cache/images.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".kubedrill", "cache", "images"), nil
}

// New builds a cache rooted at dir with the given Docker runtime.
func New(dir string, docker Docker) *Cache {
	return &Cache{root: dir, docker: docker}
}

// pathFor returns the deterministic tarball path for an image ref. The name is
// a readable slug plus a short content hash of the ref, so distinct refs never
// collide and the file is still greppable by eye.
func (c *Cache) pathFor(ref string) string {
	slug := ref
	for _, r := range []string{"/", ":", "@", " "} {
		slug = strings.ReplaceAll(slug, r, "_")
	}
	sum := sha256.Sum256([]byte(ref))
	return filepath.Join(c.root, fmt.Sprintf("%s.%s.tar", slug, hex.EncodeToString(sum[:])[:12]))
}

// Has reports whether ref is already cached.
func (c *Cache) Has(ref string) bool {
	_, err := os.Stat(c.pathFor(ref))
	return err == nil
}

// TarballsFor returns cached tarball paths for the given refs, pulling any that
// are missing first. The returned slice is aligned to refs order.
func (c *Cache) TarballsFor(ctx context.Context, refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		p, err := c.Pull(ctx, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Pull ensures ref is cached and returns its tarball path. It is idempotent
// and concurrency-safe: it takes the cache lock, re-checks presence, then
// pulls + saves to a temp file and atomically renames into place. A ref that
// is already cached returns immediately without touching Docker.
func (c *Cache) Pull(ctx context.Context, ref string) (string, error) {
	dest := c.pathFor(ref)
	if c.Has(ref) {
		return dest, nil
	}
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		return "", fmt.Errorf("imagecache: create dir: %w", err)
	}

	lock := flock.New(filepath.Join(c.root, ".lock"))
	if _, err := lock.TryLockContext(ctx, 50*time.Millisecond); err != nil {
		return "", fmt.Errorf("imagecache: acquire lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// Double-check under the lock: another process may have cached it while we
	// waited.
	if c.Has(ref) {
		return dest, nil
	}

	if err := c.docker.Pull(ctx, ref); err != nil {
		return "", fmt.Errorf("imagecache: pull %q: %w", ref, err)
	}

	tmp, err := os.CreateTemp(c.root, ".tmp-*.tar")
	if err != nil {
		return "", fmt.Errorf("imagecache: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	// Save writes the archive; clean up the temp on any failure.
	if err := c.docker.Save(ctx, ref, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("imagecache: save %q: %w", ref, err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("imagecache: commit %q: %w", ref, err)
	}
	return dest, nil
}
