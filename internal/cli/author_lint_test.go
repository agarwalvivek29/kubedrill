package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agarwalvivek29/kubedrill/internal/author"
)

func runLint(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"author", "lint"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestAuthorLintCleanScaffold(t *testing.T) {
	// The scaffold ships one hint and a real match check — it must lint clean.
	dir, err := author.Scaffold(t.TempDir(), "lint-good")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	out, err := runLint(t, dir)
	if err != nil {
		t.Fatalf("lint on scaffold errored: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no lint findings") {
		t.Fatalf("expected clean verdict, got:\n%s", out)
	}
}

func TestAuthorLintFindingsExitNonZero(t *testing.T) {
	// Structurally valid but low-quality: a constant-true check and no hints.
	dir := t.TempDir()
	writeLintFixture(t, dir)
	out, err := runLint(t, dir)
	if err == nil {
		t.Fatalf("expected non-zero exit for lint findings, got clean:\n%s", out)
	}
	if !strings.Contains(out, "vacuous-check") || !strings.Contains(out, "min-hints") {
		t.Fatalf("expected vacuous-check and min-hints findings, got:\n%s", out)
	}
}

func TestAuthorLintJSON(t *testing.T) {
	dir := t.TempDir()
	writeLintFixture(t, dir)
	out, err := runLint(t, "-o", "json", dir)
	if err == nil {
		t.Fatal("expected non-zero exit in json mode for findings")
	}
	var res lintResult
	if jerr := json.Unmarshal([]byte(out), &res); jerr != nil {
		t.Fatalf("json stdout not parseable: %v\n%s", jerr, out)
	}
	if res.Clean || len(res.Findings) == 0 {
		t.Fatalf("expected clean=false with findings, got %+v", res)
	}
}

func TestAuthorLintLoadFailurePointsAtValidate(t *testing.T) {
	// Missing difficulty fails the loader; lint must surface that (structural
	// validity is a precondition), not a lint finding.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "challenge.yaml"), []byte(`apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata: { name: nodiff, version: "0.1.0", title: t }
environment: { cluster: {}, setup: {} }
objectives:
  - { id: o1, title: o1, points: 100, checks: [ { cel: "true", target: { kind: Deployment, name: x, namespace: y, apiVersion: apps/v1 } } ] }
solution: { script: solve.sh }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runLint(t, dir)
	if err == nil {
		t.Fatalf("expected load failure error, got clean:\n%s", out)
	}
	if !strings.Contains(err.Error(), "difficulty") || !strings.Contains(err.Error(), "author validate") {
		t.Fatalf("error should name the structural failure and point at validate, got: %v", err)
	}
}

func TestAuthorLintUnknownFormat(t *testing.T) {
	dir, err := author.Scaffold(t.TempDir(), "lint-fmt")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if _, err := runLint(t, "-o", "yaml", dir); err == nil {
		t.Fatal("unknown output format: want rejection")
	}
}

// writeLintFixture writes a structurally-valid challenge that lints dirty:
// a constant-true check (vacuous) and no hints (min-hints).
func writeLintFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "solution"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "solution", "solve.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: dirty
  version: "0.1.0"
  title: "lints dirty"
  difficulty: easy
environment:
  cluster: {}
  setup: {}
objectives:
  - id: o1
    title: "always passes"
    points: 100
    checks:
      - cel: "true"
        target: { kind: Deployment, name: x, namespace: y, apiVersion: apps/v1 }
solution:
  script: solution/solve.sh
`
	if err := os.WriteFile(filepath.Join(dir, "challenge.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}
