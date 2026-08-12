package author

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/template"
)

// dns1123Label matches a Kubernetes DNS-1123 label. The scaffold uses the
// challenge name verbatim as both the directory name and the namespace its
// setup manifests create, so it must be a legal namespace name.
var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// scaffoldFile is one file the scaffold writes, relative to the challenge dir.
type scaffoldFile struct {
	rel  string      // path relative to the challenge directory
	mode os.FileMode // 0644 for data, 0755 for scripts
	tmpl string      // text/template body, rendered with scaffoldData
}

// scaffoldData is the substitution context for the templates.
type scaffoldData struct {
	Name      string // challenge (and directory) name
	Namespace string // namespace the setup manifests create; equals Name
}

// Scaffold writes a new challenge skeleton named `name` into parentDir/name and
// returns the created directory. The skeleton is a complete, loadable challenge
// (challenge.yaml + setup/ + probes/ + solution/ with SOLUTION.md and a solve.sh
// stub) so that `kubedrill author validate`/`test` pass on it unedited and an
// author edits from a known-good baseline (FR-13, Story 2.1).
//
// It refuses to overwrite: if parentDir/name already exists, it returns an
// error. `name` must be a DNS-1123 label because it doubles as the namespace
// the scaffold's manifests create.
func Scaffold(parentDir, name string) (string, error) {
	if !dns1123Label.MatchString(name) || len(name) > 63 {
		return "", fmt.Errorf("invalid challenge name %q: must be a DNS-1123 label (lowercase alphanumerics and '-', starting and ending alphanumeric, ≤ 63 chars)", name)
	}

	dir := filepath.Join(parentDir, name)
	if _, err := os.Stat(dir); err == nil {
		return "", fmt.Errorf("%s already exists; refusing to overwrite", dir)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", dir, err)
	}

	data := scaffoldData{Name: name, Namespace: name}
	for _, f := range scaffoldFiles {
		body, err := render(f.rel, f.tmpl, data)
		if err != nil {
			return "", err
		}
		abs := filepath.Join(dir, f.rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return "", fmt.Errorf("create %s: %w", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, body, f.mode); err != nil {
			return "", fmt.Errorf("write %s: %w", abs, err)
		}
	}
	// Ensure the (initially empty) probes/ directory exists — the template ships
	// no probe check, but the scaffold advertises the directory so authors have
	// an obvious home for in-cluster probe scripts.
	if err := os.MkdirAll(filepath.Join(dir, "probes"), 0o755); err != nil {
		return "", fmt.Errorf("create probes/: %w", err)
	}
	return dir, nil
}

func render(name, tmpl string, data scaffoldData) ([]byte, error) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// scaffoldFiles is the template the scaffold materializes. The challenge.yaml
// describes a coherent, solvable drill: the setup creates a namespace and a
// 1-replica Deployment, a fault scales it to 0 (so the fresh env fails the
// objective and satisfies readiness), and solve.sh scales it back.
var scaffoldFiles = []scaffoldFile{
	{
		rel:  "challenge.yaml",
		mode: 0o644,
		tmpl: challengeYAMLTmpl,
	},
	{
		rel:  "setup/01-app.yaml",
		mode: 0o644,
		tmpl: setupManifestTmpl,
	},
	{
		rel:  "solution/SOLUTION.md",
		mode: 0o644,
		tmpl: solutionMDTmpl,
	},
	{
		rel:  "solution/solve.sh",
		mode: 0o755,
		tmpl: solveScriptTmpl,
	},
}

const challengeYAMLTmpl = `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: {{ .Name }}
  version: "0.1.0"
  title: "TODO: a one-line, player-facing title"
  description: |
    TODO: describe the broken state the player finds and what "fixed" means.
    The scaffold ships a working example: the {{ .Namespace }}/app Deployment is
    scaled to 0; the player must restore it to 1 available replica.
  difficulty: easy
  topics: [workloads]
  timeLimit: 15m
environment:
  cluster:
    kubernetesVersion: "1.31"
  images: [nginx:1.27-alpine]
  setup:
    manifests:
      - path: setup/01-app.yaml
    faults:
      - name: scale-to-zero
        patch:
          target: { kind: Deployment, name: app, namespace: {{ .Namespace }}, apiVersion: apps/v1 }
          type: merge
          data:
            spec:
              replicas: 0
    readiness:
      # The environment is "ready to play" once the fault has taken hold: the
      # Deployment is below its target, so the objective genuinely fails.
      - cel: "!has(object.status.availableReplicas) || object.status.availableReplicas < 1"
        target: { kind: Deployment, name: app, namespace: {{ .Namespace }}, apiVersion: apps/v1 }
        poll: { timeout: 60s }
objectives:
  - id: app-available
    title: "the app Deployment has 1 available replica"
    points: 100
    checks:
      - match:
          target: { kind: Deployment, name: app, namespace: {{ .Namespace }}, apiVersion: apps/v1 }
          object:
            status:
              availableReplicas: 1
hints:
  - { id: h1, penalty: 10, text: "TODO: kubectl -n {{ .Namespace }} get deploy — what is the replica count?" }
solution:
  explanation: solution/SOLUTION.md
  script: solution/solve.sh
`

const setupManifestTmpl = `apiVersion: v1
kind: Namespace
metadata:
  name: {{ .Namespace }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
  namespace: {{ .Namespace }}
  labels: { app: app }
spec:
  replicas: 1
  selector:
    matchLabels: { app: app }
  template:
    metadata:
      labels: { app: app }
    spec:
      containers:
        - name: app
          image: nginx:1.27-alpine
          ports:
            - containerPort: 80
`

const solutionMDTmpl = `# Solution

TODO: explain the root cause and the fix in the author's own words.

The scaffold's example fault scales the ` + "`app`" + ` Deployment in namespace
` + "`{{ .Namespace }}`" + ` to zero replicas. Scaling it back restores service:

    kubectl -n {{ .Namespace }} scale deployment/app --replicas=1

The Deployment rolls out and reaches 1 available replica, satisfying the
` + "`app-available`" + ` objective.
`

const solveScriptTmpl = `#!/usr/bin/env bash
# Reference solution. ` + "`kubedrill author test`" + ` runs this against a fresh
# environment and asserts it drives every objective to a full pass.
# TODO: replace with the real fix for your challenge.
set -euo pipefail
kubectl -n {{ .Namespace }} scale deployment/app --replicas=1
kubectl -n {{ .Namespace }} rollout status deployment/app --timeout=120s
`
