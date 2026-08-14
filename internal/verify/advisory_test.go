package verify

import "testing"

// TestNetScoreAdvisory: in advisory mode (nodeAccess), rule penalties and a
// fail are informational — they never touch the score or zero the run.
func TestNetScoreAdvisory(t *testing.T) {
	base := Scorecard{Score: 100, MaxScore: 100, RulePenalty: 40, Failed: true}

	// Non-advisory: a fail zeroes the score.
	if got := base.NetScore(); got != 0 {
		t.Fatalf("non-advisory failed run should score 0, got %d", got)
	}

	// Advisory: the fail and rule penalty are ignored for scoring.
	adv := base
	adv.Advisory = true
	if got := adv.NetScore(); got != 100 {
		t.Fatalf("advisory run should ignore rule penalty/fail, got %d", got)
	}

	// Advisory still subtracts hint penalties (those aren't integrity-based).
	adv.HintPenalty = 15
	if got := adv.NetScore(); got != 85 {
		t.Fatalf("advisory should still apply hint penalty, got %d", got)
	}
}
