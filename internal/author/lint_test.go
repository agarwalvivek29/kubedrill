package author_test

import (
	"encoding/json"
	"testing"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/author"
)

// hasRule reports whether findings include one with the given rule id.
func hasRule(findings []author.Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

// goodChallenge is a well-formed challenge that must lint clean.
func goodChallenge() *v1alpha1.Challenge {
	return &v1alpha1.Challenge{
		APIVersion: v1alpha1.APIVersion,
		Kind:       v1alpha1.Kind,
		Metadata:   v1alpha1.Metadata{Name: "good", Version: "1.0.0", Difficulty: v1alpha1.Easy},
		Objectives: []v1alpha1.Objective{{
			ID: "o1", Title: "o1", Points: 100,
			Checks: []v1alpha1.Check{{CEL: "object.status.availableReplicas >= 1"}},
		}},
		Hints:    []v1alpha1.Hint{{ID: "h1", Penalty: 10, Text: "look here"}},
		Solution: v1alpha1.Solution{Script: "solve.sh"},
	}
}

func TestLintCleanChallenge(t *testing.T) {
	if f := author.Lint(goodChallenge()); len(f) != 0 {
		t.Fatalf("expected no findings, got %+v", f)
	}
}

func TestLintMinHints(t *testing.T) {
	c := goodChallenge()
	c.Hints = nil
	if f := author.Lint(c); !hasRule(f, "min-hints") {
		t.Fatalf("expected min-hints finding, got %+v", f)
	}
}

func TestLintVacuousCEL(t *testing.T) {
	c := goodChallenge()
	c.Objectives[0].Checks = []v1alpha1.Check{{CEL: "  true "}}
	if f := author.Lint(c); !hasRule(f, "vacuous-check") {
		t.Fatalf("expected vacuous-check for constant-true CEL, got %+v", f)
	}
}

func TestLintVacuousEmptyMatch(t *testing.T) {
	for _, obj := range []string{"{}", "null", ""} {
		c := goodChallenge()
		c.Objectives[0].Checks = []v1alpha1.Check{{
			Match: &v1alpha1.MatchCheck{
				Target: v1alpha1.Target{Kind: "Deployment", Name: "x", Namespace: "y"},
				Object: json.RawMessage(obj),
			},
		}}
		if f := author.Lint(c); !hasRule(f, "vacuous-check") {
			t.Fatalf("expected vacuous-check for empty match object %q, got %+v", obj, f)
		}
	}
}

func TestLintVacuousAnyOfBranch(t *testing.T) {
	c := goodChallenge()
	c.Objectives[0].Checks = []v1alpha1.Check{{
		AnyOf: []v1alpha1.Check{
			{CEL: "object.spec.replicas == 3"},
			{CEL: "true"}, // vacuous branch makes the whole anyOf vacuous
		},
	}}
	if f := author.Lint(c); !hasRule(f, "vacuous-check") {
		t.Fatalf("expected vacuous-check for vacuous anyOf branch, got %+v", f)
	}
}

func TestLintUnscopedEnforce(t *testing.T) {
	c := goodChallenge()
	c.Rules = []v1alpha1.Rule{{
		ID:      "r1",
		Enforce: true,
		Deny:    &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "Pod"}}, // no name/namespace
		Penalty: v1alpha1.Penalty{Fail: true},
	}}
	if f := author.Lint(c); !hasRule(f, "unscoped-enforce") {
		t.Fatalf("expected unscoped-enforce, got %+v", f)
	}

	// Scoped by namespace → clean.
	c.Rules[0].Deny.Match.Namespace = "retail"
	if f := author.Lint(c); hasRule(f, "unscoped-enforce") {
		t.Fatalf("scoped enforce should not fire unscoped-enforce, got %+v", f)
	}
}

func TestLintFieldRequireOnSecret(t *testing.T) {
	c := goodChallenge()
	c.Rules = []v1alpha1.Rule{{
		ID: "r1",
		Require: &v1alpha1.RuleSpec{
			Match:  v1alpha1.RuleMatch{Kind: "secret", Name: "db"}, // case-insensitive
			Fields: json.RawMessage(`{"data":{"password":"x"}}`),
		},
		Penalty: v1alpha1.Penalty{Points: 10},
	}}
	if f := author.Lint(c); !hasRule(f, "field-require-on-secret") {
		t.Fatalf("expected field-require-on-secret, got %+v", f)
	}

	// Object-level require on a Secret (no fields) is allowed.
	c.Rules[0].Require.Fields = nil
	if f := author.Lint(c); hasRule(f, "field-require-on-secret") {
		t.Fatalf("object-level require on Secret should be allowed, got %+v", f)
	}
}
