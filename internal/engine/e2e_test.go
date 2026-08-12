//go:build e2e

// End-to-end play loop against a real kind cluster. Requires Docker.
// Run with: go test -tags e2e -timeout 20m ./internal/engine/...
package engine_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agarwalvivek29/kubedrill/challenges"
	"github.com/agarwalvivek29/kubedrill/internal/engine"
	"github.com/agarwalvivek29/kubedrill/internal/providers/kind"
	"github.com/agarwalvivek29/kubedrill/internal/store"
)

// TestPlayLoopE2E reproduces the manual drill: start fix-crashloop, verify
// fails on the broken cluster, apply the reference solution, verify passes.
func TestPlayLoopE2E(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	st := store.New(sessionsDir)
	eng := engine.New(kind.New(), st)
	ctx := context.Background()

	dir, err := challenges.Materialize("fix-crashloop", filepath.Join(home, "challenges"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	res, err := eng.Start(ctx, dir, "e2e", nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = kind.New().Destroy(ctx, "e2e") })

	// Verify #1: broken → not all passed.
	card, _, err := eng.Verify(ctx, "e2e")
	if err != nil {
		t.Fatalf("verify #1: %v", err)
	}
	if card.AllPassed {
		t.Fatal("verify #1 should fail on the broken cluster")
	}

	// Apply the reference solution using the player kubeconfig.
	solve := filepath.Join(dir, "solution", "solve.sh")
	cmd := exec.CommandContext(ctx, "bash", solve)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+res.KubeconfigPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("solve.sh: %v\n%s", err, out)
	}

	// Verify #2: fixed → all passed, full score.
	card, _, err = eng.Verify(ctx, "e2e")
	if err != nil {
		t.Fatalf("verify #2: %v", err)
	}
	if !card.AllPassed || card.Score != card.MaxScore {
		t.Fatalf("verify #2 should pass fully, got score %d/%d", card.Score, card.MaxScore)
	}
}
