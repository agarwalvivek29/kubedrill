package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installFixturePack writes a valid pack under $HOME/.kubedrill/store via the
// install command, using a temp HOME so the real store is untouched.
func withTempHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
}

func writePack(t *testing.T, dir, packName, challenge string) {
	t.Helper()
	write := func(p, c string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(p, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(p, []byte(c), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "pack.yaml"), "apiVersion: kubedrill.dev/v1alpha1\nkind: Pack\nmetadata:\n  name: "+packName+"\n  version: \"0.1.0\"\n")
	write(filepath.Join(dir, challenge, "challenge.yaml"), `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: `+challenge+`
  version: "1.0.0"
  title: t
  difficulty: easy
environment:
  cluster: {}
  setup:
    manifests: [{ path: setup/01.yaml }]
objectives:
  - id: o1
    title: o1
    points: 100
    checks:
      - cel: "true == true"
        target: { kind: ConfigMap, name: x, namespace: y, apiVersion: v1 }
solution:
  script: solution/solve.sh
`)
	write(filepath.Join(dir, challenge, "setup", "01.yaml"), "apiVersion: v1\nkind: Namespace\nmetadata: { name: y }\n")
	write(filepath.Join(dir, challenge, "solution", "solve.sh"), "#!/bin/sh\n")
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestInstallThenCatalogAndResolve(t *testing.T) {
	withTempHome(t)
	packSrc := t.TempDir()
	writePack(t, packSrc, "study-pack", "packonly-demo")

	// install
	if out, err := runCLI(t, "install", packSrc); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	// catalog (json) includes the pack challenge with its source.
	out, err := runCLI(t, "catalog", "-o", "json")
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	var entries []catalogEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("catalog json: %v\n%s", err, out)
	}
	var found *catalogEntry
	for i := range entries {
		if entries[i].Name == "packonly-demo" {
			found = &entries[i]
		}
	}
	if found == nil || found.Source != "pack:study-pack@0.1.0" {
		t.Fatalf("catalog missing pack challenge or wrong source: %+v", entries)
	}
	// Built-ins are also present.
	hasBuiltin := false
	for _, e := range entries {
		if e.Source == "builtin" {
			hasBuiltin = true
		}
	}
	if !hasBuiltin {
		t.Fatalf("catalog should also list built-ins")
	}

	// resolveChallengeDir finds the pack challenge.
	dir, err := resolveChallengeDir("packonly-demo")
	if err != nil {
		t.Fatalf("resolve pack challenge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "challenge.yaml")); err != nil {
		t.Fatalf("resolved dir has no challenge.yaml: %v", err)
	}
}

func TestResolveUnknownChallengeErrors(t *testing.T) {
	withTempHome(t)
	_, err := resolveChallengeDir("does-not-exist-anywhere")
	if err == nil || !strings.Contains(err.Error(), "catalog") {
		t.Fatalf("expected a helpful not-found error pointing at catalog, got: %v", err)
	}
}

func TestPackExportThenInstall(t *testing.T) {
	withTempHome(t)
	src := t.TempDir()
	writePack(t, src, "shareme", "shared-demo")
	out := filepath.Join(t.TempDir(), "shareme.tgz")

	if o, err := runCLI(t, "pack", "export", src, "-o", out); err != nil {
		t.Fatalf("pack export: %v\n%s", err, o)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("tarball not written: %v", err)
	}
	// A teammate installs the tarball and can resolve the challenge.
	if o, err := runCLI(t, "install", out); err != nil {
		t.Fatalf("install exported tarball: %v\n%s", err, o)
	}
	if _, err := resolveChallengeDir("shared-demo"); err != nil {
		t.Fatalf("exported+installed challenge not resolvable: %v", err)
	}
}
