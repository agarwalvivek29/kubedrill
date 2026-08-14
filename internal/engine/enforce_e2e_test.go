//go:build e2e

// End-to-end live enforcement (ValidatingAdmissionPolicy) against a real kind
// cluster. Requires Docker. Run: go test -tags e2e -timeout 20m ./internal/engine/...
package engine_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/agarwalvivek29/kubedrill/internal/engine"
	"github.com/agarwalvivek29/kubedrill/internal/providers/kind"
	"github.com/agarwalvivek29/kubedrill/internal/store"
)

const enforceChallenge = `apiVersion: kubedrill.dev/v1alpha1
kind: Challenge
metadata:
  name: locked-cm
  version: "1.0.0"
  title: "Locked ConfigMap"
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
      description: "the keep-me ConfigMap is protected"
    penalty: fail
    enforce: true
solution:
  script: solution/solve.sh
`

// TestLiveEnforcementE2E starts a challenge with an enforced protect rule and
// confirms: (1) the player's forbidden delete is DENIED live with the rule
// message, and (2) the engine's own reset is never blocked by the policy.
func TestLiveEnforcementE2E(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "locked-cm")
	writeFile(t, filepath.Join(dir, "challenge.yaml"), enforceChallenge)
	writeFile(t, filepath.Join(dir, "setup", "01.yaml"), guardSetup) // reused from grade_e2e_test.go
	writeFile(t, filepath.Join(dir, "solution", "solve.sh"), "#!/bin/sh\n")

	st := store.New(filepath.Join(home, "sessions"))
	eng := engine.New(kind.New(), st)
	ctx := context.Background()

	res, err := eng.Start(ctx, dir, "enforcee2e", func(f string, a ...any) { t.Logf(f, a...) })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = kind.New().Destroy(context.Background(), "enforcee2e") })

	cs := clientset(t, res.KubeconfigPath) // from grade_e2e_test.go

	// The player's delete of the protected ConfigMap must be denied. Retry to
	// absorb the brief admission-policy activation delay; a delete that slips
	// through (policy not yet active) is recreated (CREATE is not blocked) so a
	// later attempt can observe the denial.
	var denialErr error
	for i := 0; i < 10 && denialErr == nil; i++ {
		time.Sleep(2 * time.Second)
		err := cs.CoreV1().ConfigMaps("guarded").Delete(ctx, "keep-me", metav1.DeleteOptions{})
		switch {
		case err == nil:
			t.Logf("delete allowed on attempt %d (policy not active yet); recreating", i+1)
			recreateKeepMe(t, ctx, cs)
		case apierrors.IsNotFound(err):
			t.Logf("keep-me missing on attempt %d; recreating", i+1)
			recreateKeepMe(t, ctx, cs)
		default:
			denialErr = err
		}
	}
	if denialErr == nil {
		t.Fatal("player delete of the protected ConfigMap was never denied — enforcement not active")
	}
	if !strings.Contains(denialErr.Error(), "keep-me ConfigMap is protected") {
		t.Fatalf("denial should carry the rule message, got: %v", denialErr)
	}

	// The engine (exempt) must be able to reset — its reset re-applies setup and
	// must not be blocked by the very policy that blocks the player.
	if _, err := eng.Reset(ctx, "enforcee2e", engine.ResetOpts{}, func(string, ...any) {}); err != nil {
		t.Fatalf("engine reset must not be blocked by enforcement, got: %v", err)
	}
}

func recreateKeepMe(t *testing.T, ctx context.Context, cs *kubernetes.Clientset) {
	t.Helper()
	_, err := cs.CoreV1().ConfigMaps("guarded").Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "keep-me", Namespace: "guarded"},
		Data:       map[string]string{"keep": "true"},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("recreate keep-me: %v", err)
	}
}
