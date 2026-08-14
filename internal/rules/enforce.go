package rules

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
)

// enforcePrefix names the generated objects so they are recognizable and can be
// snapshotted for tamper detection.
const enforcePrefix = "kubedrill-enforce-"

// EnforcementPolicies generates the ValidatingAdmissionPolicy + Binding objects
// for a challenge's `enforce: true` rules (FR-11, AD-5). For each enforced
// deny/protect rule it emits a policy that DENIES the forbidden action at
// admission for every actor EXCEPT the AD-4 exempt set — so the engine's own
// setup/reset is never blocked, while a player (or a player-created
// ServiceAccount) is stopped live. `require` rules are never enforced (a lint
// error, Story 2.3) and are skipped here defensively.
//
// The exempt matchConditions are derived from the same exemptUsernames/
// exemptPrefixes that grading uses, so enforcement and grading are one
// definition (never `system:*` wholesale).
func EnforcementPolicies(ch *v1alpha1.Challenge) []unstructured.Unstructured {
	var out []unstructured.Unstructured
	for _, r := range ch.Rules {
		if !r.Enforce {
			continue
		}
		var spec *v1alpha1.RuleSpec
		var ops []string
		switch {
		case r.Deny != nil:
			spec, ops = r.Deny, admissionOps(r.Deny.Operations)
		case r.Protect != nil:
			spec, ops = r.Protect, []string{"DELETE"}
		default:
			continue // require or malformed — not enforceable
		}
		name := enforcePrefix + sanitize(r.ID)
		out = append(out,
			validatingAdmissionPolicy(name, spec, ops, enforceMessage(r)),
			validatingAdmissionPolicyBinding(name),
		)
	}
	return out
}

// EnforcedPolicyName returns the VAP name generated for a rule id (so callers can
// snapshot and later check it still exists).
func EnforcedPolicyName(ruleID string) string { return enforcePrefix + sanitize(ruleID) }

func validatingAdmissionPolicy(name string, spec *v1alpha1.RuleSpec, ops []string, message string) unstructured.Unstructured {
	gr := kindToResource(spec.Match.Kind)
	group := gr.group // "" = core

	conditions := []any{
		map[string]any{"name": "not-exempt", "expression": exemptCEL()},
	}
	// Scope to the rule's target object where specified. request.name /
	// request.namespace are populated on delete/update (the primary enforce
	// cases) and on named creates.
	if spec.Match.Namespace != "" {
		conditions = append(conditions, map[string]any{
			"name": "target-namespace", "expression": fmt.Sprintf("request.namespace == %q", spec.Match.Namespace),
		})
	}
	if spec.Match.Name != "" {
		conditions = append(conditions, map[string]any{
			"name": "target-name", "expression": fmt.Sprintf("request.name == %q", spec.Match.Name),
		})
	}

	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       "ValidatingAdmissionPolicy",
		"metadata":   map[string]any{"name": name, "labels": map[string]any{"app.kubernetes.io/managed-by": "kubedrill"}},
		"spec": map[string]any{
			"failurePolicy": "Fail",
			"matchConstraints": map[string]any{
				"resourceRules": []any{map[string]any{
					"apiGroups":   []any{group},
					"apiVersions": []any{"*"},
					"operations":  toAnySlice(ops),
					"resources":   []any{gr.resource},
				}},
			},
			"matchConditions": conditions,
			// A request that reaches validation is a non-exempt actor performing
			// the forbidden action on the target — always deny.
			"validations": []any{map[string]any{
				"expression": "false",
				"message":    message,
				"reason":     "Forbidden",
			}},
		},
	}}
}

func validatingAdmissionPolicyBinding(policyName string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       "ValidatingAdmissionPolicyBinding",
		"metadata":   map[string]any{"name": policyName + "-binding", "labels": map[string]any{"app.kubernetes.io/managed-by": "kubedrill"}},
		"spec": map[string]any{
			"policyName":        policyName,
			"validationActions": []any{"Deny"},
		},
	}}
}

// exemptCEL is the AD-4 exempt set as a CEL predicate that is TRUE when the
// requesting user is NOT exempt (so the policy only fires for chargeable actors).
// Built from the same source as grading's isExempt.
func exemptCEL() string {
	quoted := make([]string, 0, len(exemptUsernames()))
	for _, u := range exemptUsernames() {
		quoted = append(quoted, fmt.Sprintf("%q", u))
	}
	parts := []string{fmt.Sprintf("!(request.userInfo.username in [%s])", strings.Join(quoted, ", "))}
	for _, p := range exemptPrefixes() {
		parts = append(parts, fmt.Sprintf("!request.userInfo.username.startsWith(%q)", p))
	}
	return strings.Join(parts, " && ")
}

// admissionOps maps rule (audit) verbs to admission operations. An empty rule
// operation set (a blanket deny) blocks all mutating operations.
func admissionOps(verbs []string) []string {
	if len(verbs) == 0 {
		return []string{"CREATE", "UPDATE", "DELETE"}
	}
	seen := map[string]bool{}
	var out []string
	add := func(op string) {
		if !seen[op] {
			seen[op] = true
			out = append(out, op)
		}
	}
	for _, v := range verbs {
		switch v {
		case "create":
			add("CREATE")
		case "update", "patch":
			add("UPDATE")
		case "delete", "deletecollection":
			add("DELETE")
		}
	}
	if len(out) == 0 {
		out = []string{"CREATE", "UPDATE", "DELETE"}
	}
	return out
}

func enforceMessage(r v1alpha1.Rule) string {
	var desc string
	switch {
	case r.Deny != nil:
		desc = r.Deny.Description
	case r.Protect != nil:
		desc = r.Protect.Description
	}
	if desc != "" {
		return "kubedrill: " + desc
	}
	return fmt.Sprintf("kubedrill: this action is blocked by rule %q", r.ID)
}

// sanitize makes a rule id safe as a DNS-1123 object name fragment.
func sanitize(id string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(id) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			b.WriteRune(c)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
