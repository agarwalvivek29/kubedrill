package verify

import (
	"context"
	"time"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/rules"
)

// defaultPollWindow is how long a failing object check (match/cel) is retried
// before its failure is accepted, absorbing eventual consistency (FR-9). Probe
// checks are not retried here — they re-run a Job, and self-retry in-script.
const defaultPollWindow = 15 * time.Second

// ObjectiveResult is one objective's evaluated state.
type ObjectiveResult struct {
	ID       string
	Title    string
	Points   int
	Passed   bool
	Errored  bool
	Reason   string
}

// Scorecard is the outcome of verifying a challenge. Score is the raw sum of
// passed-objective points (computed here, cluster-only); HintPenalty, the rule
// grading, and Failed are layered by the engine from session state and the audit
// log. NetScore is what the player earns.
type Scorecard struct {
	Objectives  []ObjectiveResult
	Score       int // raw objective points earned
	MaxScore    int
	HintPenalty int // sum of revealed-hint penalties (set by the engine)
	AllPassed   bool

	// Rule grading (Epic 3), layered by the engine from the audit log.
	RuleViolations []rules.Violation // charged rule breaches, with evidence
	RulePenalty    int               // sum of points deducted by violated rules
	Failed         bool              // a `penalty: fail` rule (or tampering) tripped

	// Advisory is set for nodeAccess challenges: node/root access defeats audit
	// tamper-evidence, so rule violations are reported for information but do NOT
	// affect the score or fail the run (AD-5, FR-18).
	Advisory bool
}

// NetScore is the awarded score: objective points minus hint and rule penalties,
// floored at 0. A Failed challenge scores 0 outright — unless grading is
// Advisory (node access), where rule outcomes are informational and never touch
// the score. AllPassed remains about objectives.
func (c Scorecard) NetScore() int {
	if c.Failed && !c.Advisory {
		return 0
	}
	n := c.Score - c.HintPenalty
	if !c.Advisory {
		n -= c.RulePenalty
	}
	if n < 0 {
		return 0
	}
	return n
}

// Evaluate runs every objective's checks against the cluster and computes the
// score. An objective passes only if all its checks Pass; a check that Errors
// marks the objective errored (distinct from failed) and does not count as
// passed. dependsOn gating is applied for PLAYER scoring: an objective whose
// dependency has not passed is left unevaluated (not-yet-reachable).
//
// Note: hint/rule penalties are layered by the engine; this computes the
// positive objective score and per-objective outcomes.
func Evaluate(ctx context.Context, e *Evaluator, ch *v1alpha1.Challenge) Scorecard {
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

		ok, errored, reason := evalObjective(ctx, e, obj)
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

// EvaluateDirect grades every objective's checks directly, ignoring dependsOn
// gating (AD-11): it is the negative phase of `author test`. A vacuous objective
// must not be able to hide behind an unmet dependency, so each objective is
// evaluated on its own — an objective "passes" here iff all its own checks pass.
// Checks are evaluated ONCE (no poll-retry): the negative phase is a snapshot of
// the freshly-provisioned broken environment, and retrying an expected failure
// would only add latency without changing the verdict.
func EvaluateDirect(ctx context.Context, e *Evaluator, ch *v1alpha1.Challenge) Scorecard {
	var card Scorecard
	for _, obj := range ch.Objectives {
		card.MaxScore += obj.Points
		ok, errored, reason := evalObjectiveDirect(ctx, e, obj)
		if ok {
			card.Score += obj.Points
		}
		card.Objectives = append(card.Objectives, ObjectiveResult{
			ID: obj.ID, Title: obj.Title, Points: obj.Points,
			Passed: ok, Errored: errored, Reason: reason,
		})
	}
	card.AllPassed = card.Score == card.MaxScore && card.MaxScore > 0
	return card
}

// evalObjectiveDirect evaluates all checks (AND) exactly once, no poll-retry.
func evalObjectiveDirect(ctx context.Context, e *Evaluator, obj v1alpha1.Objective) (bool, bool, string) {
	for _, ch := range obj.Checks {
		r := e.Check(ctx, ch)
		switch r.Outcome {
		case Errored:
			return false, true, r.Reason
		case Fail:
			return false, false, r.Reason
		}
	}
	return true, false, ""
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
func evalObjective(ctx context.Context, e *Evaluator, obj v1alpha1.Objective) (bool, bool, string) {
	for _, ch := range obj.Checks {
		r := pollCheck(ctx, e, ch)
		switch r.Outcome {
		case Errored:
			return false, true, r.Reason
		case Fail:
			return false, false, r.Reason
		}
	}
	return true, false, ""
}

// pollCheck evaluates a check, retrying a first-pass FAIL over a bounded window
// so eventually-consistent object state isn't graded as failure (FR-9). Pass
// and Errored return immediately. Probe checks are evaluated once (they re-run
// a Job and self-retry in-script), so they are not looped here.
func pollCheck(ctx context.Context, e *Evaluator, ch v1alpha1.Check) CheckResult {
	r := e.Check(ctx, ch)
	if r.Outcome != Fail || ch.Probe != nil {
		return r
	}
	window := defaultPollWindow
	if ch.Poll != nil && ch.Poll.Timeout != "" {
		if d, err := time.ParseDuration(ch.Poll.Timeout); err == nil && d > 0 {
			window = d
		}
	}
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return r
		case <-time.After(2 * time.Second):
		}
		r = e.Check(ctx, ch)
		if r.Outcome != Fail {
			return r
		}
	}
	return r
}
