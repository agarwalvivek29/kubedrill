// Package challenge loads and validates challenge directories.
package challenge

import (
	"encoding/json"
	"fmt"
)

// Match reports whether the live object satisfies the match tree, per the
// FROZEN v1alpha1 subset semantics (AD-6, LLD §3). This is the check-language
// contract; its behavior is pinned by match_test.go and must not drift.
//
// Rules:
//  1. Maps: every key in `tree` must exist in `obj` and match recursively;
//     extra keys in `obj` are ignored (subset).
//  2. Scalars: equal after JSON numeric normalization (all numbers compared as
//     float64, the encoding/json default); strings never coerce to numbers.
//  3. Arrays: unordered subset — every element of `tree` must match at least
//     one distinct element of `obj` (greedy with backtracking).
//  4. null in `tree` asserts the field is absent or null in `obj`.
//
// `tree` and `obj` are decoded JSON values (map[string]any, []any, scalars).
func Match(tree, obj any) bool {
	// null in the tree asserts absent/null in the object.
	if tree == nil {
		return obj == nil
	}
	switch t := tree.(type) {
	case map[string]any:
		o, ok := obj.(map[string]any)
		if !ok {
			return false
		}
		for k, tv := range t {
			ov, present := o[k]
			if tv == nil {
				// assert absent-or-null
				if present && ov != nil {
					return false
				}
				continue
			}
			if !present {
				return false
			}
			if !Match(tv, ov) {
				return false
			}
		}
		return true
	case []any:
		o, ok := obj.([]any)
		if !ok {
			return false
		}
		return arraySubset(t, o)
	default:
		return scalarEqual(tree, obj)
	}
}

// arraySubset matches every tree element to a distinct object element.
// Order-independent; backtracks so a greedy early match can't block a later
// element from finding its partner.
func arraySubset(tree, obj []any) bool {
	used := make([]bool, len(obj))
	var assign func(i int) bool
	assign = func(i int) bool {
		if i == len(tree) {
			return true
		}
		for j := range obj {
			if used[j] || !Match(tree[i], obj[j]) {
				continue
			}
			used[j] = true
			if assign(i + 1) {
				return true
			}
			used[j] = false
		}
		return false
	}
	return assign(0)
}

// scalarEqual compares scalars with JSON numeric normalization. Numbers (int,
// int64, float64, json.Number) compare as float64; strings and bools compare
// by identity of kind + value. A string is never equal to a number.
func scalarEqual(a, b any) bool {
	af, aok := asFloat(a)
	bf, bok := asFloat(b)
	if aok || bok {
		return aok && bok && af == bf
	}
	return a == b
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// decodeTree unmarshals a match `object` payload into the generic value shape
// Match expects. Returns a located error on malformed JSON/YAML.
func decodeTree(raw json.RawMessage, where string) (any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s: match.object is empty", where)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s: match.object is not valid: %w", where, err)
	}
	return v, nil
}
