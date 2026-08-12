package verify

import (
	"context"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
)

// ObjectiveResult is one objective's evaluated state.
type ObjectiveResult struct {
	ID       string
	Title    string
	Points   int
	Passed   bool
	Errored  bool
	Reason   string
}

// Scorecard is the outcome of verifying a challenge.
type Scorecard struct {
	Objectives []ObjectiveResult
	Score      int
	MaxScore   int
	AllPassed  bool
}

// Evaluate runs every objective's checks against the cluster and computes the
// score. An objective passes only if all its checks Pass; a check that Errors
// marks the objective errored (distinct from failed) and does not count as
// passed. dependsOn gating is applied for PLAYER scoring: an objective whose
// dependency has not passed is left unevaluated (not-yet-reachable).
//
// Note: hint/rule penalties are layered by the engine; this computes the
// positive objective score and per-objective outcomes.
func Evaluate(ctx context.Context, c *kube.Client, ch *v1alpha1.Challenge) Scorecard {
	passed := map[string]bool{}
	var card Scorecard

	for _, obj := range ch.Objectives {
		card.MaxScore += obj.Points
		res := ObjectiveResult{ID: obj.ID, Title: obj.Title, Points: obj.Points}

		if !dependenciesMet(obj, passed) {
			res.Reason = "blocked: a prerequisite objective is not yet passed"
			card.Objectives = append(card.Objectives, res)
			continue
		}

		ok, errored, reason := evalObjective(ctx, c, obj)
		res.Passed = ok
		res.Errored = errored
		res.Reason = reason
		if ok {
			passed[obj.ID] = true
			card.Score += obj.Points
		}
		card.Objectives = append(card.Objectives, res)
	}
	card.AllPassed = card.Score == card.MaxScore && card.MaxScore > 0
	return card
}

func dependenciesMet(obj v1alpha1.Objective, passed map[string]bool) bool {
	for _, dep := range obj.DependsOn {
		if !passed[dep] {
			return false
		}
	}
	return true
}

// evalObjective evaluates all checks (AND). Returns passed, errored, reason.
func evalObjective(ctx context.Context, c *kube.Client, obj v1alpha1.Objective) (bool, bool, string) {
	for _, ch := range obj.Checks {
		r := EvalCheck(ctx, c, ch)
		switch r.Outcome {
		case Errored:
			return false, true, r.Reason
		case Fail:
			return false, false, r.Reason
		}
	}
	return true, false, ""
}
