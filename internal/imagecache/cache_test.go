package imagecache

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeDocker records calls and "saves" by writing a stub archive.
type fakeDocker struct {
	pulls  int64
	saves  int64
	onSave func(dest string) error
}

func (f *fakeDocker) Pull(_ context.Context, _ string) error {
	atomic.AddInt64(&f.pulls, 1)
	return nil
}

func (f *fakeDocker) Save(_ context.Context, _, destPath string) error {
	atomic.AddInt64(&f.saves, 1)
	if f.onSave != nil {
		if err := f.onSave(destPath); err != nil {
			return err
		}
	}
	return os.WriteFile(destPath, []byte("TARBALL"), 0o644)
}

func TestPullCachesAndSkipsSecondTime(t *testing.T) {
	d := &fakeDocker{}
	c := New(t.TempDir(), d)
	ctx := context.Background()

	p1, err := c.Pull(ctx, "nginx:1.27-alpine")
	if err != nil {
		t.Fatalf("first pull: %v", err)
	}
	if _, err := os.Stat(p1); err != nil {
		t.Fatalf("tarball not written: %v", err)
	}
	if !c.Has("nginx:1.27-alpine") {
		t.Fatal("Has should be true after pull")
	}

	p2, err := c.Pull(ctx, "nginx:1.27-alpine")
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("path not stable: %q vs %q", p1, p2)
	}
	if got := atomic.LoadInt64(&d.pulls); got != 1 {
		t.Fatalf("docker pull called %d times, want 1 (second should hit cache)", got)
	}
}

func TestPathForDistinctRefsDoNotCollide(t *testing.T) {
	c := New(t.TempDir(), &fakeDocker{})
	a := c.pathFor("registry.k8s.io/pause:3.9")
	b := c.pathFor("registry.k8s.io/pause:3.10")
	if a == b {
		t.Fatal("distinct refs produced the same cache path")
	}
	if !strings.HasSuffix(a, ".tar") {
		t.Fatalf("cache path should end in .tar: %q", a)
	}
}

func TestConcurrentPullsPullOnce(t *testing.T) {
	d := &fakeDocker{}
	c := New(t.TempDir(), d)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Pull(ctx, "busybox:1.36"); err != nil {
				t.Errorf("concurrent pull: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&d.pulls); got != 1 {
		t.Fatalf("docker pull called %d times under concurrency, want exactly 1", got)
	}
	// No temp files left behind.
	entries, _ := os.ReadDir(c.root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func TestSaveFailureLeavesNoPartialCache(t *testing.T) {
	d := &fakeDocker{onSave: func(dest string) error {
		// Simulate a save that writes a partial file then fails.
		_ = os.WriteFile(dest, []byte("PARTIAL"), 0o644)
		return errDiskFull
	}}
	c := New(t.TempDir(), d)

	_, err := c.Pull(context.Background(), "nginx:1.27-alpine")
	if err == nil {
		t.Fatal("expected error when save fails")
	}
	if c.Has("nginx:1.27-alpine") {
		t.Fatal("failed save must not leave a cached entry (no atomic rename)")
	}
	entries, _ := os.ReadDir(c.root)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tar" || strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("partial/temp artifact left behind: %s", e.Name())
		}
	}
}

type constErr string

func (e constErr) Error() string { return string(e) }

const errDiskFull = constErr("simulated disk full")
