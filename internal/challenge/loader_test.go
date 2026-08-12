package challenge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDirValid(t *testing.T) {
	got, err := LoadDir("testdata/valid")
	if err != nil {
		t.Fatalf("valid challenge failed to load: %v", err)
	}
	if got.Challenge.Metadata.Name != "fix-crashloop" {
		t.Fatalf("name = %q, want fix-crashloop", got.Challenge.Metadata.Name)
	}
	if len(got.Challenge.Objectives) != 2 {
		t.Fatalf("objectives = %d, want 2", len(got.Challenge.Objectives))
	}
	// penalty: fail round-trips into the sentinel.
	if !got.Challenge.Rules[0].Penalty.Fail {
		t.Fatalf("rule penalty should be fail sentinel")
	}
}

// writeChallenge lays down a challenge dir from a yaml string plus the standard
// referenced files, so negative cases isolate the one thing under test.
func writeChallenge(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "challenge.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	must := filepath.Join(dir, "solution")
	_ = os.MkdirAll(must, 0o755)
	_ = os.WriteFile(filepath.Join(must, "solve.sh"), []byte("#!/usr/bin/env bash\n"), 0o644)
	return dir
}

const header = `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: t
  version: "1.0.0"
  title: t
  difficulty: medium
environment:
  cluster: {}
  setup: {}
solution:
  script: solution/solve.sh
`

func TestLoadDirNegativeTable(t *testing.T) {
	cases := []struct {
		name   string
		yaml   string
		errHas string
	}{
		{
			name: "unknown field rejected",
			yaml: header + `objectives:
  - id: a
    title: a
    points: 1
    wat: nope
    checks:
      - cel: "object.x == 1"
`,
			errHas: "strict decode failed",
		},
		{
			name: "wrong apiVersion",
			yaml: strings.Replace(header, "kubedrill.dev/v1alpha1", "example.com/v1", 1) + `objectives:
  - id: a
    title: a
    points: 1
    checks:
      - cel: "object.x == 1"
`,
			errHas: "unsupported document",
		},
		{
			name: "duplicate objective id",
			yaml: header + `objectives:
  - id: dup
    title: a
    points: 1
    checks: [{cel: "object.x == 1"}]
  - id: dup
    title: b
    points: 1
    checks: [{cel: "object.y == 1"}]
`,
			errHas: "duplicate id",
		},
		{
			name: "dependsOn cycle",
			yaml: header + `objectives:
  - id: a
    title: a
    points: 1
    dependsOn: [b]
    checks: [{cel: "object.x == 1"}]
  - id: b
    title: b
    points: 1
    dependsOn: [a]
    checks: [{cel: "object.y == 1"}]
`,
			errHas: "cycle",
		},
		{
			name: "bad CEL does not compile",
			yaml: header + `objectives:
  - id: a
    title: a
    points: 1
    checks: [{cel: "object.x =="}]
`,
			errHas: "cel does not compile",
		},
		{
			name: "check with two branches",
			yaml: header + `objectives:
  - id: a
    title: a
    points: 1
    checks:
      - cel: "object.x == 1"
        match:
          target: { kind: Pod }
          object: { x: 1 }
`,
			errHas: "exactly one of match|cel|probe|anyOf",
		},
		{
			name: "enforce on require rule",
			yaml: header + `objectives:
  - id: a
    title: a
    points: 1
    checks: [{cel: "object.x == 1"}]
rules:
  - id: must
    require: { operations: [PATCH], match: { kind: Node } }
    penalty: { points: 5 }
    enforce: true
`,
			errHas: "enforce:true is invalid on a require rule",
		},
		{
			name: "missing referenced manifest",
			yaml: `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata: { name: t, version: "1.0.0", title: t, difficulty: medium }
environment:
  cluster: {}
  setup:
    manifests:
      - path: setup/missing.yaml
objectives:
  - id: a
    title: a
    points: 1
    checks: [{cel: "object.x == 1"}]
solution:
  script: solution/solve.sh
`,
			errHas: "not found",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeChallenge(t, tc.yaml)
			_, err := LoadDir(dir)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errHas)
			}
			if !strings.Contains(err.Error(), tc.errHas) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errHas)
			}
		})
	}
}
