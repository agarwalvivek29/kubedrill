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

// runValidate executes `author validate` with the given args, returning stdout
// and the command error (which Execute would map to a non-zero exit).
func runValidate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"author", "validate"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestAuthorValidateValidText(t *testing.T) {
	dir, err := author.Scaffold(t.TempDir(), "good-drill")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	out, err := runValidate(t, dir)
	if err != nil {
		t.Fatalf("validate on scaffold errored: %v", err)
	}
	if !strings.Contains(out, "good-drill is valid") {
		t.Fatalf("summary missing valid verdict:\n%s", out)
	}
}

func TestAuthorValidateValidJSON(t *testing.T) {
	dir, err := author.Scaffold(t.TempDir(), "json-drill")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	out, err := runValidate(t, "-o", "json", dir)
	if err != nil {
		t.Fatalf("validate -o json errored: %v", err)
	}
	var res validateResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out)
	}
	if !res.Valid {
		t.Fatalf("valid=false for a good scaffold: %+v", res)
	}
	if res.Name != "json-drill" || res.Objectives != 1 || res.Faults != 1 {
		t.Fatalf("unexpected counts: %+v", res)
	}
}

func TestAuthorValidateBrokenFails(t *testing.T) {
	// A challenge that decodes but fails semantic validation: missing difficulty.
	dir := t.TempDir()
	writeChallenge(t, dir, `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: broken
  version: "0.1.0"
  title: "missing difficulty"
environment:
  cluster: {}
  setup: {}
objectives:
  - id: o1
    title: "o1"
    points: 100
    checks:
      - cel: "true"
        target: { kind: Deployment, name: x, namespace: y, apiVersion: apps/v1 }
solution:
  script: solution/solve.sh
`)
	// solution script referenced must exist for us to reach the difficulty error,
	// but here validation fails earlier on difficulty regardless — assert failure.
	out, err := runValidate(t, dir)
	if err == nil {
		t.Fatalf("expected validation error, got success:\n%s", out)
	}
	if !strings.Contains(err.Error(), "difficulty") {
		t.Fatalf("error should name the offending field (difficulty), got: %v", err)
	}
}

func TestAuthorValidateBrokenJSONExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	writeChallenge(t, dir, `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: broken
  version: "0.1.0"
  title: "no objectives"
  difficulty: easy
environment:
  cluster: {}
  setup: {}
objectives: []
solution:
  script: solution/solve.sh
`)
	out, err := runValidate(t, "-o", "json", dir)
	if err == nil {
		t.Fatal("expected non-nil error (non-zero exit) for invalid challenge in json mode")
	}
	// stdout must still be clean, parseable JSON with valid:false.
	var res validateResult
	if jerr := json.Unmarshal([]byte(out), &res); jerr != nil {
		t.Fatalf("json-mode stdout not parseable on failure: %v\n%s", jerr, out)
	}
	if res.Valid || res.Error == "" {
		t.Fatalf("expected valid=false with an error message: %+v", res)
	}
}

func TestAuthorValidateUnknownFormat(t *testing.T) {
	dir, err := author.Scaffold(t.TempDir(), "fmt-drill")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if _, err := runValidate(t, "-o", "yaml", dir); err == nil {
		t.Fatal("unknown output format: got nil error, want rejection")
	}
}

func writeChallenge(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "challenge.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write challenge.yaml: %v", err)
	}
}
