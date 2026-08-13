package author

import (
	"strings"
	"testing"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/verify"
)

// obj is a tiny objective builder for the negative-phase tests.
func obj(id string, expectPassing bool) v1alpha1.Objective {
	return v1alpha1.Objective{ID: id, Title: id, Points: 50, ExpectInitiallyPassing: expectPassing}
}

// res is a per-objective direct outcome (as EvaluateDirect would produce).
func res(id string, passed, errored bool) verify.ObjectiveResult {
	return verify.ObjectiveResult{ID: id, Title: id, Points: 50, Passed: passed, Errored: errored}
}

func card(rs ...verify.ObjectiveResult) verify.Scorecard {
	var c verify.Scorecard
	for _, r := range rs {
		c.Objectives = append(c.Objectives, r)
		c.MaxScore += r.Points
		if r.Passed {
			c.Score += r.Points
		}
	}
	c.AllPassed = c.MaxScore > 0 && c.Score == c.MaxScore
	return c
}

func TestNegativePhaseAllFailIsClean(t *testing.T) {
	ch := &v1alpha1.Challenge{Objectives: []v1alpha1.Objective{obj("a", false), obj("b", false)}}
	p := negativePhase(ch, card(res("a", false, false), res("b", false, false)))
	if !p.Passed || len(p.Violations) != 0 {
		t.Fatalf("expected clean negative phase, got %+v", p)
	}
}

func TestNegativePhaseErroredCountsAsFail(t *testing.T) {
	// An errored check in the negative phase satisfies "objective fails" (AC).
	ch := &v1alpha1.Challenge{Objectives: []v1alpha1.Objective{obj("a", false)}}
	p := negativePhase(ch, card(res("a", false, true)))
	if !p.Passed {
		t.Fatalf("errored objective should count as failing (clean), got %+v", p)
	}
}

func TestNegativePhaseVacuousObjectiveIsViolation(t *testing.T) {
	// A normal objective that PASSES on the fresh env is vacuous.
	ch := &v1alpha1.Challenge{Objectives: []v1alpha1.Objective{obj("a", false), obj("b", false)}}
	p := negativePhase(ch, card(res("a", false, false), res("b", true, false)))
	if p.Passed {
		t.Fatal("a passing objective on the fresh env must be a violation")
	}
	if len(p.Violations) != 1 || p.Violations[0].ObjectiveID != "b" {
		t.Fatalf("expected exactly one violation naming b, got %+v", p.Violations)
	}
}

func TestNegativePhaseExpectInitiallyPassing(t *testing.T) {
	ch := &v1alpha1.Challenge{Objectives: []v1alpha1.Objective{obj("a", false), obj("keep", true)}}

	// expectInitiallyPassing objective that PASSES → clean.
	clean := negativePhase(ch, card(res("a", false, false), res("keep", true, false)))
	if !clean.Passed {
		t.Fatalf("expectInitiallyPassing objective that passes should be clean, got %+v", clean)
	}

	// expectInitiallyPassing objective that FAILS → violation (it must pass).
	bad := negativePhase(ch, card(res("a", false, false), res("keep", false, false)))
	if bad.Passed || len(bad.Violations) != 1 || bad.Violations[0].ObjectiveID != "keep" {
		t.Fatalf("expected a violation naming keep, got %+v", bad.Violations)
	}
}

func TestPhaseFromCardPassAndFail(t *testing.T) {
	pass := phaseFromCard("positive", &verify.Scorecard{
		Objectives: []verify.ObjectiveResult{res("a", true, false)},
		Score:      50, MaxScore: 50, AllPassed: true,
	})
	if !pass.Passed {
		t.Fatalf("full-score card should pass, got %+v", pass)
	}

	// An errored objective fails the positive phase and is surfaced distinctly.
	fail := phaseFromCard("positive", &verify.Scorecard{
		Objectives: []verify.ObjectiveResult{res("a", true, false), res("b", false, true)},
		Score:      50, MaxScore: 100, AllPassed: false,
	})
	if fail.Passed {
		t.Fatal("errored objective must fail the positive phase")
	}
	if want := "errored: b"; !strings.Contains(fail.Detail, want) {
		t.Fatalf("detail should mention %q, got %q", want, fail.Detail)
	}
}
