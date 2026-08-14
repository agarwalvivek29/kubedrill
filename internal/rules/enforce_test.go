package rules_test

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/rules"
)

func nestedString(t *testing.T, obj map[string]any, path ...string) string {
	t.Helper()
	s, _, err := unstructured.NestedString(obj, path...)
	if err != nil {
		t.Fatalf("nested %v: %v", path, err)
	}
	return s
}

func TestEnforcementNoneWhenNoEnforce(t *testing.T) {
	ch := challengeWithRules(v1alpha1.Rule{
		ID: "graded-only", Deny: &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "ConfigMap"}},
		Penalty: v1alpha1.Penalty{Points: 5}, // Enforce: false
	})
	if got := rules.EnforcementPolicies(ch); len(got) != 0 {
		t.Fatalf("no enforce rules should yield no policies, got %d", len(got))
	}
}

func TestEnforcementGeneratesPolicyAndBinding(t *testing.T) {
	ch := challengeWithRules(v1alpha1.Rule{
		ID:      "no-delete-web",
		Protect: &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "Deployment", Namespace: "shop", Name: "web"}},
		Penalty: v1alpha1.Penalty{Fail: true},
		Enforce: true,
	})
	objs := rules.EnforcementPolicies(ch)
	if len(objs) != 2 {
		t.Fatalf("expected a policy + binding, got %d", len(objs))
	}
	policy, binding := objs[0], objs[1]
	if policy.GetKind() != "ValidatingAdmissionPolicy" || binding.GetKind() != "ValidatingAdmissionPolicyBinding" {
		t.Fatalf("unexpected kinds: %s, %s", policy.GetKind(), binding.GetKind())
	}
	if policy.GetName() != rules.EnforcedPolicyName("no-delete-web") {
		t.Fatalf("policy name = %q", policy.GetName())
	}
	// The binding references the policy and denies.
	if pn := nestedString(t, binding.Object, "spec", "policyName"); pn != policy.GetName() {
		t.Fatalf("binding.policyName = %q, want %q", pn, policy.GetName())
	}

	// Protect enforces DELETE on apps/deployments.
	rr, _, _ := unstructured.NestedSlice(policy.Object, "spec", "matchConstraints", "resourceRules")
	rule0 := rr[0].(map[string]any)
	if ops, _, _ := unstructured.NestedStringSlice(rule0, "operations"); len(ops) != 1 || ops[0] != "DELETE" {
		t.Fatalf("protect should enforce DELETE, got %v", ops)
	}
	if res, _, _ := unstructured.NestedStringSlice(rule0, "resources"); res[0] != "deployments" {
		t.Fatalf("resource = %v", res)
	}
}

// TestEnforcementExemptsExactlyAD4Set is the load-bearing check: the exempt
// matchCondition must name precisely the engine + pinned allowlist and NEVER
// exempt `system:*` wholesale (which would reopen the SA loophole AD-4 closes).
func TestEnforcementExemptsExactlyAD4Set(t *testing.T) {
	ch := challengeWithRules(v1alpha1.Rule{
		ID: "d", Deny: &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "Pod"}},
		Penalty: v1alpha1.Penalty{Fail: true}, Enforce: true,
	})
	policy := rules.EnforcementPolicies(ch)[0]
	conds, _, _ := unstructured.NestedSlice(policy.Object, "spec", "matchConditions")
	var cel string
	for _, c := range conds {
		m := c.(map[string]any)
		if m["name"] == "not-exempt" {
			cel = m["expression"].(string)
		}
	}
	if cel == "" {
		t.Fatal("missing not-exempt matchCondition")
	}
	for _, want := range []string{
		"kubedrill:engine",
		"system:apiserver",
		"system:kube-controller-manager",
		"system:kube-scheduler",
		"system:kube-proxy",
		"system:node:",
		"system:serviceaccount:kube-system:",
	} {
		if !strings.Contains(cel, want) {
			t.Errorf("exempt CEL missing %q\nCEL: %s", want, cel)
		}
	}
	// It must NOT wholesale-exempt system:* or all serviceaccounts.
	for _, forbidden := range []string{"'system:*'", "startsWith(\"system:\")", "startsWith(\"system:serviceaccount:\")"} {
		if strings.Contains(cel, forbidden) {
			t.Errorf("exempt CEL must never wholesale-exempt %q\nCEL: %s", forbidden, cel)
		}
	}
	// A player-created SA must be able to reach validation (i.e. NOT match any
	// exempt clause). Sanity: the CEL scopes the kube-system SA prefix only.
	if strings.Contains(cel, "system:serviceaccount:") && !strings.Contains(cel, "system:serviceaccount:kube-system:") {
		t.Errorf("SA exemption must be scoped to kube-system only\nCEL: %s", cel)
	}
}

func TestEnforcementDenyOperationMapping(t *testing.T) {
	ch := challengeWithRules(v1alpha1.Rule{
		ID: "no-mutate-cm",
		Deny: &v1alpha1.RuleSpec{
			Operations: []string{"patch", "update"}, // both map to UPDATE (deduped)
			Match:      v1alpha1.RuleMatch{Kind: "ConfigMap"},
		},
		Penalty: v1alpha1.Penalty{Points: 10}, Enforce: true,
	})
	policy := rules.EnforcementPolicies(ch)[0]
	rr, _, _ := unstructured.NestedSlice(policy.Object, "spec", "matchConstraints", "resourceRules")
	ops, _, _ := unstructured.NestedStringSlice(rr[0].(map[string]any), "operations")
	if len(ops) != 1 || ops[0] != "UPDATE" {
		t.Fatalf("patch+update should dedupe to a single UPDATE, got %v", ops)
	}
}
