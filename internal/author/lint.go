package author

import (
	"encoding/json"
	"fmt"
	"strings"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
)

// MinHints is the minimum number of progressive hints a challenge must ship.
// A challenge with no hints strands a stuck player (AD-11 quality bar).
const MinHints = 1

// Finding is one lint result: a stable rule id plus a human message. An empty
// slice from Lint means the challenge is clean.
type Finding struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Lint applies opinionated quality/safety rules to a challenge and returns
// findings (empty = clean). It assumes the challenge is already structurally
// valid — callers run challenge.LoadDir first — so two contract violations the
// loader already rejects are intentionally NOT re-checked here: a missing
// difficulty (metadata.difficulty: required) and enforce:true on a require rule
// (both fail LoadDir). Lint covers the quality rules the loader does not:
// too-few hints, trivially-vacuous checks, unscoped enforce rules, and
// field-level require on Secrets (AD-5/AD-11, FR-14).
func Lint(c *v1alpha1.Challenge) []Finding {
	var findings []Finding
	add := func(rule, format string, args ...any) {
		findings = append(findings, Finding{Rule: rule, Message: fmt.Sprintf(format, args...)})
	}

	if len(c.Hints) < MinHints {
		add("min-hints", "challenge has %d hint(s); at least %d progressive hint is required", len(c.Hints), MinHints)
	}

	for _, o := range c.Objectives {
		for j, ch := range o.Checks {
			if reason := vacuousCheck(ch); reason != "" {
				add("vacuous-check", "objective %q checks[%d]: %s", o.ID, j, reason)
			}
		}
	}

	for _, r := range c.Rules {
		// An enforce rule (deny/protect) that names neither a target object nor a
		// namespace matches every object of its kind cluster-wide — it can block
		// unrelated system objects. Enforce must be scoped (AD-5).
		if r.Enforce {
			if spec := enforceSpec(r); spec != nil && spec.Match.Name == "" && spec.Match.Namespace == "" {
				add("unscoped-enforce", "rule %q: enforce:true but unscoped (match has no name or namespace); scope it to the challenge's objects", r.ID)
			}
		}
		// Field-level require on Secrets would grade the contents of a Secret,
		// leaking secret data into the scorecard — never allowed (AD-5).
		if r.Require != nil && len(r.Require.Fields) > 0 && strings.EqualFold(r.Require.Match.Kind, "Secret") {
			add("field-require-on-secret", "rule %q: field-level require on a Secret is not allowed (it would grade secret contents)", r.ID)
		}
	}

	return findings
}

// enforceSpec returns the deny/protect spec an enforce rule carries, or nil.
// enforce is only meaningful on deny/protect (the loader rejects it on require).
func enforceSpec(r v1alpha1.Rule) *v1alpha1.RuleSpec {
	switch {
	case r.Deny != nil:
		return r.Deny
	case r.Protect != nil:
		return r.Protect
	default:
		return nil
	}
}

// vacuousCheck reports why a check is trivially satisfied regardless of cluster
// state, or "" if it is meaningful. A vacuous check makes an objective pass on a
// fresh, unfixed environment — exactly what the negative phase of `author test`
// exists to catch, but cheaper to flag here.
func vacuousCheck(ch v1alpha1.Check) string {
	switch {
	case ch.CEL != "":
		if isConstTrue(ch.CEL) {
			return "cel expression is constant true (always passes)"
		}
	case ch.Match != nil:
		if isEmptyObject(ch.Match.Object) {
			return "match object is empty (asserts nothing about the target)"
		}
	case len(ch.AnyOf) > 0:
		// An anyOf passes if ANY branch passes, so one vacuous branch makes the
		// whole check vacuous.
		for _, leaf := range ch.AnyOf {
			if vacuousCheck(leaf) != "" {
				return "anyOf has a vacuous branch (always passes)"
			}
		}
	}
	return ""
}

// isConstTrue reports whether a CEL expression is the boolean literal true.
func isConstTrue(expr string) bool {
	return strings.TrimSpace(expr) == "true"
}

// isEmptyObject reports whether a match object tree asserts nothing: absent,
// null, or an empty object all match any target state.
func isEmptyObject(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null" || s == "{}"
}
