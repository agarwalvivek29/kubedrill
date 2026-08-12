package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveImageRefsRawRefs(t *testing.T) {
	got, err := resolveImageRefs([]string{"nginx:1.27-alpine", "busybox:1.36"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "nginx:1.27-alpine" {
		t.Fatalf("raw refs mishandled: %v", got)
	}
}

func TestResolveImageRefsFromChallengeDir(t *testing.T) {
	dir := t.TempDir()
	sol := filepath.Join(dir, "solution")
	_ = os.MkdirAll(sol, 0o755)
	_ = os.WriteFile(filepath.Join(sol, "solve.sh"), []byte("#!/usr/bin/env bash\n"), 0o644)
	yaml := `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata: { name: t, version: "1.0.0", title: t, difficulty: easy }
environment:
  cluster: {}
  images: [nginx:1.27-alpine, busybox:1.36]
  setup: {}
objectives:
  - id: a
    title: a
    points: 1
    checks: [{cel: "object.x == 1"}]
solution:
  script: solution/solve.sh
`
	if err := os.WriteFile(filepath.Join(dir, "challenge.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveImageRefs([]string{dir})
	if err != nil {
		t.Fatalf("resolve from challenge dir: %v", err)
	}
	if len(got) != 2 || got[1] != "busybox:1.36" {
		t.Fatalf("challenge images mishandled: %v", got)
	}
}
