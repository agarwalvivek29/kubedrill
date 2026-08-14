//go:build e2e

// End-to-end rule grading against a real audited kind cluster. Requires Docker.
// Run with: go test -tags e2e -timeout 20m ./internal/engine/...
package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/agarwalvivek29/kubedrill/internal/engine"
	"github.com/agarwalvivek29/kubedrill/internal/providers/kind"
	"github.com/agarwalvivek29/kubedrill/internal/store"
)

const guardChallenge = `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: guard-cm
  version: "1.0.0"
  title: "Guard the ConfigMap"
  difficulty: easy
environment:
  cluster:
    kubernetesVersion: "1.31"
  setup:
    manifests:
      - path: setup/01.yaml
objectives:
  - id: cm-exists
    title: "keep-me still exists"
    points: 100
    checks:
      - match:
          target: { kind: ConfigMap, name: keep-me, namespace: guarded, apiVersion: v1 }
          object:
            metadata:
              name: keep-me
rules:
  - id: no-delete-keepme
    protect:
      match: { kind: ConfigMap, name: keep-me, namespace: guarded }
    penalty: fail
solution:
  script: solution/solve.sh
`

const guardSetup = `apiVersion: v1
kind: Namespace
metadata:
  name: guarded
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: keep-me
  namespace: guarded
data:
  keep: "true"
`

// TestRuleGradingE2E starts a ruled challenge on a real audited cluster, has the
// player delete a protected object, and confirms `verify` charges the protect
// rule with the triggering audit event as evidence and fails the run.
func TestRuleGradingE2E(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "guard-cm")
	writeFile(t, filepath.Join(dir, "challenge.yaml"), guardChallenge)
	writeFile(t, filepath.Join(dir, "setup", "01.yaml"), guardSetup)
	writeFile(t, filepath.Join(dir, "solution", "solve.sh"), "#!/bin/sh\n")

	st := store.New(filepath.Join(home, "sessions"))
	eng := engine.New(kind.New(), st)
	ctx := context.Background()

	res, err := eng.Start(ctx, dir, "gradee2e", func(f string, a ...any) { t.Logf(f, a...) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = kind.New().Destroy(context.Background(), "gradee2e") })

	// Clean verify first: nothing deleted yet → no violation, objective passes.
	card, _, err := eng.Verify(ctx, "gradee2e")
	if err != nil {
		t.Fatalf("verify #1: %v", err)
	}
	if len(card.RuleViolations) != 0 || card.Failed {
		t.Fatalf("clean run must have no violations, got %+v", card.RuleViolations)
	}
	if !card.AllPassed {
		t.Fatalf("objective should pass before deletion, got %+v", card.Objectives)
	}

	// The player deletes the protected ConfigMap.
	cs := clientset(t, res.KubeconfigPath)
	if err := cs.CoreV1().ConfigMaps("guarded").Delete(ctx, "keep-me", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete keep-me: %v", err)
	}

	// Verify again: the protect rule must trip, with evidence, and fail the run.
	card, _, err = eng.Verify(ctx, "gradee2e")
	if err != nil {
		t.Fatalf("verify #2: %v", err)
	}
	if !card.Failed {
		t.Fatalf("a fail-penalty protect violation must fail the run; card=%+v", card)
	}
	found := false
	for _, v := range card.RuleViolations {
		if v.RuleID != "no-delete-keepme" {
			continue
		}
		found = true
		if v.Type != "protect" || !v.Fail {
			t.Fatalf("expected a protect/fail violation, got %+v", v)
		}
		// Evidence must carry the player's delete of keep-me.
		ok := false
		for _, ev := range v.Evidence {
			if ev.Verb == "delete" && ev.Resource == "configmaps" && ev.Name == "keep-me" && ev.User == "kubernetes-admin" {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("violation missing the triggering delete event, evidence=%+v", v.Evidence)
		}
	}
	if !found {
		t.Fatalf("no-delete-keepme violation not found in %+v", card.RuleViolations)
	}
	if card.NetScore() != 0 {
		t.Fatalf("a failed run must score 0, got %d", card.NetScore())
	}
}

func clientset(t *testing.T, kubeconfigPath string) *kubernetes.Clientset {
	t.Helper()
	b, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		t.Fatalf("read kubeconfig: %v", err)
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(b)
	if err != nil {
		t.Fatalf("rest config: %v", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	return cs
}

func writeFile(t *testing.T, path, content string) {
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
