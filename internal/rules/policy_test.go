package rules_test

import (
	"encoding/json"
	"testing"

	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/rules"
)

func challengeWithRules(rs ...v1alpha1.Rule) *v1alpha1.Challenge {
	return &v1alpha1.Challenge{
		APIVersion: v1alpha1.APIVersion, Kind: v1alpha1.Kind,
		Metadata: v1alpha1.Metadata{Name: "c", Version: "1", Difficulty: v1alpha1.Easy},
		Rules:    rs,
	}
}

// auditPolicyDoc is enough of the audit Policy schema to assert the generated
// YAML parses and has the right shape.
type auditPolicyDoc struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Rules      []struct {
		Level     string   `json:"level"`
		Verbs     []string `json:"verbs"`
		Resources []struct {
			Group     string   `json:"group"`
			Resources []string `json:"resources"`
		} `json:"resources"`
	} `json:"rules"`
}

func parsePolicy(t *testing.T, s string) auditPolicyDoc {
	t.Helper()
	var doc auditPolicyDoc
	if err := yaml.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatalf("generated audit policy is not valid YAML: %v\n%s", err, s)
	}
	if doc.APIVersion != "audit.k8s.io/v1" || doc.Kind != "Policy" {
		t.Fatalf("wrong discriminators: %+v", doc)
	}
	return doc
}

func TestAuditPolicyEmptyWhenNoRules(t *testing.T) {
	ch := challengeWithRules()
	if got := rules.AuditPolicy(ch); got != "" {
		t.Fatalf("expected empty policy for an unruled challenge, got:\n%s", got)
	}
}

func TestAuditPolicyBaselineForRules(t *testing.T) {
	ch := challengeWithRules(v1alpha1.Rule{
		ID:      "no-delete-ns",
		Protect: &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "Namespace", Name: "retail"}},
		Penalty: v1alpha1.Penalty{Fail: true},
	})
	doc := parsePolicy(t, rules.AuditPolicy(ch))

	// Secrets pinned Metadata must be the FIRST rule (wins over anything later).
	if len(doc.Rules) < 2 {
		t.Fatalf("expected at least secrets + baseline rules, got %d", len(doc.Rules))
	}
	first := doc.Rules[0]
	if first.Level != "Metadata" || len(first.Resources) != 1 || first.Resources[0].Resources[0] != "secrets" {
		t.Fatalf("first rule must pin secrets to Metadata, got %+v", first)
	}
	// A Metadata baseline over mutating verbs must exist.
	if !hasBaseline(doc) {
		t.Fatalf("expected a Metadata baseline over mutating verbs, got %+v", doc.Rules)
	}
	// No field-require here, so there must be NO Request-level rule.
	for _, r := range doc.Rules {
		if r.Level == "Request" {
			t.Fatalf("did not expect a Request-level rule without field-require, got %+v", r)
		}
	}
}

func TestAuditPolicyRequestForFieldRequire(t *testing.T) {
	ch := challengeWithRules(v1alpha1.Rule{
		ID: "must-set-limits",
		Require: &v1alpha1.RuleSpec{
			Match:  v1alpha1.RuleMatch{Kind: "Deployment", Name: "web"},
			Fields: json.RawMessage(`{"spec":{"template":{"spec":{"containers":[{"resources":{}}]}}}}`),
		},
		Penalty: v1alpha1.Penalty{Points: 20},
	})
	doc := parsePolicy(t, rules.AuditPolicy(ch))

	var reqRule *struct {
		Level     string   `json:"level"`
		Verbs     []string `json:"verbs"`
		Resources []struct {
			Group     string   `json:"group"`
			Resources []string `json:"resources"`
		} `json:"resources"`
	}
	for i := range doc.Rules {
		if doc.Rules[i].Level == "Request" {
			reqRule = &doc.Rules[i]
		}
	}
	if reqRule == nil {
		t.Fatalf("expected a Request-level rule for the field-required Deployment, got %+v", doc.Rules)
	}
	if reqRule.Resources[0].Group != "apps" || reqRule.Resources[0].Resources[0] != "deployments" {
		t.Fatalf("Request rule should target apps/deployments, got %+v", reqRule.Resources)
	}
	// Ordering: secrets-Metadata first, Request before the broad Metadata baseline.
	if doc.Rules[0].Resources[0].Resources[0] != "secrets" {
		t.Fatalf("secrets must remain the first rule")
	}
}

// TestAuditPolicyNeverRequestsSecretBodies guards AD-5: even a (lint-rejected but
// defensively handled) field-require on a Secret must never produce a
// Request-level rule for secrets.
func TestAuditPolicyNeverRequestsSecretBodies(t *testing.T) {
	ch := challengeWithRules(v1alpha1.Rule{
		ID: "bad",
		Require: &v1alpha1.RuleSpec{
			Match:  v1alpha1.RuleMatch{Kind: "Secret", Name: "db"},
			Fields: json.RawMessage(`{"data":{"x":"y"}}`),
		},
		Penalty: v1alpha1.Penalty{Points: 10},
	})
	policy := rules.AuditPolicy(ch)
	doc := parsePolicy(t, policy)
	for _, r := range doc.Rules {
		if r.Level == "Request" {
			for _, res := range r.Resources {
				for _, name := range res.Resources {
					if name == "secrets" {
						t.Fatalf("secrets must NEVER be captured at Request level:\n%s", policy)
					}
				}
			}
		}
	}
}

func hasBaseline(doc auditPolicyDoc) bool {
	for _, r := range doc.Rules {
		if r.Level == "Metadata" && contains(r.Verbs, "delete") && contains(r.Verbs, "create") {
			return true
		}
	}
	return false
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
