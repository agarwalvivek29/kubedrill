package file_test

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/agarwalvivek29/kubedrill/internal/sources/file"
)

// minimalChallenge writes a valid, loadable challenge into dir/<name>.
func minimalChallenge(t *testing.T, dir, name string) {
	t.Helper()
	cdir := filepath.Join(dir, name)
	mustWrite(t, filepath.Join(cdir, "challenge.yaml"), `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: `+name+`
  version: "1.0.0"
  title: "t"
  difficulty: easy
environment:
  cluster: {}
  setup:
    manifests:
      - path: setup/01.yaml
objectives:
  - id: o1
    title: o1
    points: 100
    checks:
      - cel: "object.metadata.name == 'x'"
        target: { kind: ConfigMap, name: x, namespace: y, apiVersion: v1 }
solution:
  script: solution/solve.sh
`)
	mustWrite(t, filepath.Join(cdir, "setup", "01.yaml"), "apiVersion: v1\nkind: Namespace\nmetadata: { name: y }\n")
	mustWrite(t, filepath.Join(cdir, "solution", "solve.sh"), "#!/bin/sh\n")
}

func packDir(t *testing.T, name, version string, challenges ...string) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "pack.yaml"), `apiVersion: kubedrill.dev/v1alpha1
kind: Pack
metadata:
  name: `+name+`
  version: "`+version+`"
  description: "test pack"
`)
	for _, c := range challenges {
		minimalChallenge(t, dir, c)
	}
	return dir
}

func TestInstallFromDirAndResolve(t *testing.T) {
	src := packDir(t, "cka-basics", "0.1.0", "alpha", "beta")
	store := t.TempDir()

	p, err := file.Install(src, store)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if p.Name != "cka-basics" || p.Version != "0.1.0" || len(p.Challenges) != 2 {
		t.Fatalf("unexpected pack: %+v", p)
	}
	// Installed to store/<name>/<version>/.
	if want := filepath.Join(store, "cka-basics", "0.1.0"); p.Dir != want {
		t.Fatalf("installed dir = %q, want %q", p.Dir, want)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, "alpha", "challenge.yaml")); err != nil {
		t.Fatalf("challenge not installed: %v", err)
	}
	// solve.sh keeps its exec bit.
	fi, _ := os.Stat(filepath.Join(p.Dir, "alpha", "solution", "solve.sh"))
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("solve.sh lost its exec bit: %v", fi.Mode())
	}

	// Resolve finds a challenge across the store.
	dir, pack, ok := file.Resolve(store, "beta")
	if !ok || pack != "cka-basics" || filepath.Base(dir) != "beta" {
		t.Fatalf("resolve beta = (%q,%q,%v)", dir, pack, ok)
	}
	if _, _, ok := file.Resolve(store, "nope"); ok {
		t.Fatalf("resolve of unknown challenge should fail")
	}

	// Installed lists it.
	packs, err := file.Installed(store)
	if err != nil || len(packs) != 1 || packs[0].Name != "cka-basics" {
		t.Fatalf("installed: %+v err=%v", packs, err)
	}
}

func TestInstallIsAtomicReplace(t *testing.T) {
	store := t.TempDir()
	if _, err := file.Install(packDir(t, "p", "1.0.0", "a"), store); err != nil {
		t.Fatalf("install v1: %v", err)
	}
	// Reinstall the same version with a different challenge set — replaces cleanly.
	if _, err := file.Install(packDir(t, "p", "1.0.0", "a", "b"), store); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if _, _, ok := file.Resolve(store, "b"); !ok {
		t.Fatalf("replaced pack should contain b")
	}
	// No temp/old dirs left behind.
	entries, _ := os.ReadDir(filepath.Join(store, "p"))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".old" || len(e.Name()) > 0 && e.Name()[0] == '.' {
			t.Fatalf("leftover staging dir: %s", e.Name())
		}
	}
}

func TestInstallFromTarball(t *testing.T) {
	src := packDir(t, "tarpack", "2.0.0", "gamma")
	tgz := filepath.Join(t.TempDir(), "tarpack.tgz")
	writeTarball(t, src, tgz)

	store := t.TempDir()
	p, err := file.Install(tgz, store)
	if err != nil {
		t.Fatalf("install from tarball: %v", err)
	}
	if p.Name != "tarpack" || len(p.Challenges) != 1 {
		t.Fatalf("unexpected pack from tarball: %+v", p)
	}
	if _, _, ok := file.Resolve(store, "gamma"); !ok {
		t.Fatalf("tarball challenge not resolvable")
	}
}

func TestInstallRejectsBadManifest(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "pack.yaml"), "apiVersion: wrong\nkind: Pack\nmetadata: { name: x, version: \"1\" }\n")
	minimalChallenge(t, dir, "a")
	if _, err := file.Install(dir, t.TempDir()); err == nil {
		t.Fatal("expected rejection of a bad manifest apiVersion")
	}
}

func TestInstallRejectsUnloadableChallenge(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "pack.yaml"), "apiVersion: kubedrill.dev/v1alpha1\nkind: Pack\nmetadata: { name: p, version: \"1.0.0\" }\n")
	// A challenge dir that won't load (missing required fields).
	mustWrite(t, filepath.Join(dir, "broken", "challenge.yaml"), "apiVersion: kubedrill.dev/v1alpha1\nkind: Challenge\nmetadata: { name: broken }\n")
	if _, err := file.Install(dir, t.TempDir()); err == nil {
		t.Fatal("expected rejection of a pack with an unloadable challenge")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if filepath.Ext(path) == ".sh" {
		mode = 0o755
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func writeTarball(t *testing.T, srcDir, out string) {
	t.Helper()
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		b, _ := os.ReadFile(path)
		h := &tar.Header{Name: rel, Mode: int64(info.Mode().Perm()), Size: int64(len(b)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		_, err = tw.Write(b)
		return err
	})
}
