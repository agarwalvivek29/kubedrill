//go:build e2e

// End-to-end author-test harness against real kind clusters. Requires Docker.
// Run with: go test -tags e2e -timeout 20m ./internal/author/...
package author_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agarwalvivek29/kubedrill/challenges"
	"github.com/agarwalvivek29/kubedrill/internal/author"
	"github.com/agarwalvivek29/kubedrill/internal/providers/kind"
)

// TestHarnessPassesOnRealChallengeE2E runs the full harness on the reference
// fix-crashloop challenge: negative (all objectives fail on the fresh env),
// positive (solve.sh → 100%), idempotency (verify again passes).
func TestHarnessPassesOnRealChallengeE2E(t *testing.T) {
	dir, err := challenges.Materialize("fix-crashloop", filepath.Join(t.TempDir(), "challenges"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	report, err := author.Test(context.Background(), kind.New(), dir,
		author.TestOptions{SessionID: "attest-good"}, func(f string, a ...any) { t.Logf(f, a...) })
	if err != nil {
		t.Fatalf("harness error: %v", err)
	}
	if !report.Passed {
		t.Fatalf("expected fix-crashloop to pass author test, got:\n"+
			"  negative:    passed=%v %s\n  positive:    passed=%v %s\n  idempotency: passed=%v %s",
			report.Negative.Passed, report.Negative.Detail,
			report.Positive.Passed, report.Positive.Detail,
			report.Idempotency.Passed, report.Idempotency.Detail)
	}
}

// TestHarnessCatchesVacuousChallengeE2E proves the negative phase catches a
// vacuous objective: fix-crashloop with an always-true CEL objective added. The
// reference solution still works, but the harness must reject the challenge
// because that objective passes on the fresh, unsolved environment — and the
// positive/idempotency phases must then be skipped.
func TestHarnessCatchesVacuousChallengeE2E(t *testing.T) {
	dir, err := challenges.Materialize("fix-crashloop", filepath.Join(t.TempDir(), "challenges"))
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	insertVacuousObjective(t, filepath.Join(dir, "challenge.yaml"))

	report, err := author.Test(context.Background(), kind.New(), dir,
		author.TestOptions{SessionID: "attest-vacuous"}, func(f string, a ...any) { t.Logf(f, a...) })
	if err != nil {
		t.Fatalf("harness error: %v", err)
	}
	if report.Passed {
		t.Fatal("harness must reject a challenge with a vacuous objective")
	}
	if report.Negative.Passed {
		t.Fatal("negative phase should have failed")
	}
	found := false
	for _, v := range report.Negative.Violations {
		if v.ObjectiveID == "always-true" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a vacuity violation naming 'always-true', got %+v", report.Negative.Violations)
	}
	if !report.Positive.Skipped {
		t.Fatalf("positive phase should be skipped after a negative failure, got %+v", report.Positive)
	}
}

// insertVacuousObjective adds a constant-true CEL objective (pointed at the
// existing webshop Deployment so it resolves and passes) into the objectives
// list, just before the hints: block.
func insertVacuousObjective(t *testing.T, challengeYAML string) {
	t.Helper()
	raw, err := os.ReadFile(challengeYAML)
	if err != nil {
		t.Fatalf("read challenge.yaml: %v", err)
	}
	const vacuous = `  - id: always-true
    title: "always true (vacuous)"
    points: 10
    checks:
      - cel: "true"
        target: { kind: Deployment, name: webshop, namespace: retail, apiVersion: apps/v1 }
`
	doc := string(raw)
	if !strings.Contains(doc, "\nhints:") {
		t.Fatalf("challenge.yaml layout changed; no hints: block to anchor on")
	}
	doc = strings.Replace(doc, "\nhints:", "\n"+vacuous+"hints:", 1)
	if err := os.WriteFile(challengeYAML, []byte(doc), 0o644); err != nil {
		t.Fatalf("write challenge.yaml: %v", err)
	}
}
