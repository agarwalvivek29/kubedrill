package challenge

import (
	"encoding/json"
	"testing"
)

// This table IS the frozen match: contract (AD-6). Changing an expectation
// here is a schema-semantics change, not a test fix.
func TestMatchSubsetSemantics(t *testing.T) {
	j := func(s string) any {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Fatalf("bad test json %q: %v", s, err)
		}
		return v
	}
	cases := []struct {
		name string
		tree string
		obj  string
		want bool
	}{
		{"map subset ignores extra keys", `{"status":{"replicas":3}}`, `{"status":{"replicas":3,"ready":3},"spec":{}}`, true},
		{"map missing key fails", `{"status":{"replicas":3}}`, `{"status":{"ready":3}}`, false},
		{"map value mismatch fails", `{"status":{"replicas":3}}`, `{"status":{"replicas":2}}`, false},
		{"nested map recursion", `{"a":{"b":{"c":1}}}`, `{"a":{"b":{"c":1,"d":2}}}`, true},
		{"scalar int normalized to float", `{"n":3}`, `{"n":3.0}`, true},
		{"string not equal to number", `{"n":"3"}`, `{"n":3}`, false},
		{"bool match", `{"ok":true}`, `{"ok":true}`, true},
		{"bool mismatch", `{"ok":true}`, `{"ok":false}`, false},
		{"null asserts absent", `{"x":null}`, `{"y":1}`, true},
		{"null asserts null present", `{"x":null}`, `{"x":null}`, true},
		{"null fails when key present non-null", `{"x":null}`, `{"x":1}`, false},
		{"array unordered subset present", `{"items":[{"name":"b"}]}`, `{"items":[{"name":"a"},{"name":"b"}]}`, true},
		{"array element absent fails", `{"items":[{"name":"z"}]}`, `{"items":[{"name":"a"},{"name":"b"}]}`, false},
		{"array subset needs distinct partners", `{"items":[{"k":1},{"k":1}]}`, `{"items":[{"k":1}]}`, false},
		{"array subset backtracking", `{"items":[{"k":1},{"k":2}]}`, `{"items":[{"k":2},{"k":1}]}`, true},
		{"array order irrelevant", `{"t":[3,1,2]}`, `{"t":[1,2,3,4]}`, true},
		{"type mismatch map vs scalar", `{"a":{"b":1}}`, `{"a":5}`, false},
		{"empty tree matches anything mappish", `{}`, `{"a":1}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Match(j(tc.tree), j(tc.obj))
			if got != tc.want {
				t.Fatalf("Match(%s, %s) = %v, want %v", tc.tree, tc.obj, got, tc.want)
			}
		})
	}
}
