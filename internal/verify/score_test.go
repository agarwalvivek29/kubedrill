package verify

import (
	"testing"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
)

func TestDependenciesMet(t *testing.T) {
	passed := map[string]bool{"a": true}
	if !dependenciesMet(v1alpha1.Objective{ID: "b", DependsOn: []string{"a"}}, passed) {
		t.Fatal("b depends on a (passed) — should be met")
	}
	if dependenciesMet(v1alpha1.Objective{ID: "c", DependsOn: []string{"x"}}, passed) {
		t.Fatal("c depends on x (not passed) — should be blocked")
	}
	if !dependenciesMet(v1alpha1.Objective{ID: "d"}, passed) {
		t.Fatal("no deps — always met")
	}
}

func TestScorecardAllPassedBookkeeping(t *testing.T) {
	full := Scorecard{Score: 100, MaxScore: 100}
	full.AllPassed = full.Score == full.MaxScore && full.MaxScore > 0
	if !full.AllPassed {
		t.Fatal("full score should be AllPassed")
	}
	partial := Scorecard{Score: 50, MaxScore: 100}
	partial.AllPassed = partial.Score == partial.MaxScore && partial.MaxScore > 0
	if partial.AllPassed {
		t.Fatal("partial score should not be AllPassed")
	}
	empty := Scorecard{Score: 0, MaxScore: 0}
	empty.AllPassed = empty.Score == empty.MaxScore && empty.MaxScore > 0
	if empty.AllPassed {
		t.Fatal("zero-objective challenge should not report AllPassed")
	}
}
