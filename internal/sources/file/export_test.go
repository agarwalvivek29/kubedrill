package file_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agarwalvivek29/kubedrill/internal/sources/file"
)

func TestExportRoundTripsWithInstall(t *testing.T) {
	src := packDir(t, "roundtrip", "1.2.0", "one", "two")
	out := filepath.Join(t.TempDir(), "roundtrip.tgz")

	p, err := file.Export(src, "", out)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if p.Name != "roundtrip" || len(p.Challenges) != 2 {
		t.Fatalf("unexpected exported pack: %+v", p)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("tarball not written: %v", err)
	}

	// Install the exported tarball into a fresh store — challenges survive.
	store := t.TempDir()
	ip, err := file.Install(out, store)
	if err != nil {
		t.Fatalf("install exported tarball: %v", err)
	}
	if ip.Name != "roundtrip" || len(ip.Challenges) != 2 {
		t.Fatalf("round-trip lost challenges: %+v", ip)
	}
	if _, _, ok := file.Resolve(store, "two"); !ok {
		t.Fatalf("round-tripped challenge not resolvable")
	}
}

func TestExportInstalledPackByName(t *testing.T) {
	store := t.TempDir()
	if _, err := file.Install(packDir(t, "named", "0.3.0", "x"), store); err != nil {
		t.Fatalf("install: %v", err)
	}
	out := filepath.Join(t.TempDir(), "named.tgz")
	if _, err := file.Export("named", store, out); err != nil {
		t.Fatalf("export installed pack by name: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("tarball missing: %v", err)
	}
}
