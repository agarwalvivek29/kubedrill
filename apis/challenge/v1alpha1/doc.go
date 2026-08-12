// Package v1alpha1 defines the kubedrill Challenge schema — the public,
// versioned contract every authored challenge is written against.
//
// apiVersion: kubedrill.dev/v1alpha1. This package (with pkg/api) is one of
// the only two importable by third parties (AD-12). The schema evolves via
// conversion-on-load (v1alpha1 -> v1beta1 -> v1), never a silent break.
//
// Frozen at M0 (AD-6), the semantics that live in every authored challenge:
//   - match: partial-object subset matching (maps recursive, arrays unordered
//     subset, null asserts absent/null, scalars numeric-normalized)
//   - the CEL helper surface: exactly snapshot() and restarts0()
//   - the rule-verb set: {deny, protect, require}
//   - the check combinator: checks = AND, anyOf = OR, one level of nesting
//
// Types (Challenge, Objective, Check, Rule, Hint, ...) land in Story 1.2.
package v1alpha1
