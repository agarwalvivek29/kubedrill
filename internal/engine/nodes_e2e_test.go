//go:build e2e

// End-to-end node-level challenge against a real kind cluster. Requires Docker.
// Run: go test -tags e2e -timeout 20m ./internal/engine/...
package engine_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agarwalvivek29/kubedrill/internal/engine"
	"github.com/agarwalvivek29/kubedrill/internal/providers/kind"
	"github.com/agarwalvivek29/kubedrill/internal/store"
)

const nodeChallenge = `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: node-drill
  version: "1.0.0"
  title: "Bring the kubelet back"
  difficulty: hard
  nodeAccess: true
environment:
  setup:
    manifests:
      - path: setup/01.yaml
    faults:
      - name: stop-kubelet
        nodeExec:
          node: control-plane
          command: ["systemctl", "stop", "kubelet"]
objectives:
  - id: ns-exists
    title: "the workspace namespace exists"
    points: 100
    checks:
      - match:
          target: { kind: Namespace, name: workspace, apiVersion: v1 }
          object:
            metadata:
              name: workspace
solution:
  script: solution/solve.sh
`

const nodeSetup = `apiVersion: v1
kind: Namespace
metadata:
  name: workspace
`

// TestNodeLevelE2E starts a nodeAccess challenge whose nodeExec fault stops the
// kubelet, and confirms: the fault took effect (observed via Environment.NodeExec),
// and NodeShellCommand resolves to a runnable argv for the node.
func TestNodeLevelE2E(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "node-drill")
	writeFile(t, filepath.Join(dir, "challenge.yaml"), nodeChallenge)
	writeFile(t, filepath.Join(dir, "setup", "01.yaml"), nodeSetup)
	writeFile(t, filepath.Join(dir, "solution", "solve.sh"), "#!/bin/sh\n")

	st := store.New(filepath.Join(home, "sessions"))
	prov := kind.New()
	eng := engine.New(prov, st)
	ctx := context.Background()

	if _, err := eng.Start(ctx, dir, "nodee2e", func(f string, a ...any) { t.Logf(f, a...) }); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = kind.New().Destroy(context.Background(), "nodee2e") })

	env, err := prov.Environment(ctx, "nodee2e", st.SessionDir("nodee2e"))
	if err != nil {
		t.Fatalf("reconstruct env: %v", err)
	}

	// The nodeExec fault stopped the kubelet — confirm via node access (docker
	// exec, which works even with the kubelet down).
	out, _ := env.NodeExec(ctx, "control-plane", []string{"systemctl", "is-active", "kubelet"})
	if active := strings.TrimSpace(string(out)); active == "active" {
		t.Fatalf("kubelet should be stopped by the nodeExec fault, is-active=%q", active)
	} else {
		t.Logf("kubelet is-active=%q (fault took effect)", active)
	}

	// And a general node command works (proves NodeExec is usable for authoring).
	if out, err := env.NodeExec(ctx, "control-plane", []string{"sh", "-c", "echo node-ok"}); err != nil || !strings.Contains(string(out), "node-ok") {
		t.Fatalf("node exec echo failed: out=%q err=%v", out, err)
	}

	// NodeShellCommand resolves to a runnable argv naming the node container.
	argv, err := env.NodeShellCommand("control-plane")
	if err != nil {
		t.Fatalf("node shell command: %v", err)
	}
	if len(argv) < 4 || argv[0] != "docker" || !strings.Contains(strings.Join(argv, " "), "nodee2e-control-plane") {
		t.Fatalf("unexpected node-shell argv: %v", argv)
	}
}
