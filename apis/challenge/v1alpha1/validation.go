package v1alpha1

import (
	"fmt"
	"strings"
)

// Validate performs structural/semantic validation that JSON Schema cannot
// express: discriminator correctness, unique ids, acyclic dependsOn, and
// exactly-one-of unions. It does NOT touch the filesystem or a cluster — the
// loader layers referential (files-exist) and compile (match/CEL) checks on
// top. Returns a joined, located error or nil.
func (c *Challenge) Validate() error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if c.APIVersion != APIVersion {
		add("apiVersion: got %q, want %q", c.APIVersion, APIVersion)
	}
	if c.Kind != Kind {
		add("kind: got %q, want %q", c.Kind, Kind)
	}
	if c.Metadata.Name == "" {
		add("metadata.name: required")
	}
	if c.Metadata.Version == "" {
		add("metadata.version: required")
	}
	switch c.Metadata.Difficulty {
	case Easy, Medium, Hard, Expert:
	case "":
		add("metadata.difficulty: required")
	default:
		add("metadata.difficulty: %q is not one of easy|medium|hard|expert", c.Metadata.Difficulty)
	}

	// Objectives: at least one; unique ids; acyclic dependsOn; valid checks.
	if len(c.Objectives) == 0 {
		add("objectives: at least one required")
	}
	ids := map[string]bool{}
	for i, o := range c.Objectives {
		if o.ID == "" {
			add("objectives[%d].id: required", i)
			continue
		}
		if ids[o.ID] {
			add("objectives[%d].id: duplicate id %q", i, o.ID)
		}
		ids[o.ID] = true
		if len(o.Checks) == 0 {
			add("objective %q: at least one check required", o.ID)
		}
		for j, ch := range o.Checks {
			if err := ch.validate(); err != nil {
				add("objective %q checks[%d]: %v", o.ID, j, err)
			}
		}
	}
	// dependsOn targets must exist and must not form a cycle.
	for _, o := range c.Objectives {
		for _, dep := range o.DependsOn {
			if !ids[dep] {
				add("objective %q: dependsOn references unknown objective %q", o.ID, dep)
			}
		}
	}
	if cyc := c.dependsOnCycle(); cyc != "" {
		add("objectives: dependsOn cycle detected: %s", cyc)
	}

	// Faults: exactly one of patch|exec|nodeExec.
	for i, f := range c.Environment.Setup.Faults {
		n := b2i(f.Patch != nil) + b2i(f.Exec != nil) + b2i(f.NodeExec != nil)
		if n != 1 {
			add("environment.setup.faults[%d] (%q): exactly one of patch|exec|nodeExec required, got %d", i, f.Name, n)
		}
		if f.NodeExec != nil && !c.Metadata.NodeAccess {
			add("environment.setup.faults[%d] (%q): nodeExec requires metadata.nodeAccess: true", i, f.Name)
		}
	}

	// Rules: exactly one verb; enforce not on require.
	rids := map[string]bool{}
	for i, r := range c.Rules {
		if r.ID == "" {
			add("rules[%d].id: required", i)
		} else if rids[r.ID] {
			add("rules[%d].id: duplicate id %q", i, r.ID)
		}
		rids[r.ID] = true
		n := b2i(r.Deny != nil) + b2i(r.Protect != nil) + b2i(r.Require != nil)
		if n != 1 {
			add("rule %q: exactly one of deny|protect|require required, got %d", r.ID, n)
		}
		if r.Enforce && r.Require != nil {
			add("rule %q: enforce:true is invalid on a require rule (cannot live-deny an omission)", r.ID)
		}
	}

	if c.Solution.Script == "" {
		add("solution.script: required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("challenge validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// validate checks a single Check is a well-formed union: exactly one of
// match|cel|probe|anyOf, and anyOf leaves are themselves valid.
func (ch *Check) validate() error {
	n := b2i(ch.Match != nil) + b2i(ch.CEL != "") + b2i(ch.Probe != nil) + b2i(len(ch.AnyOf) > 0)
	if n != 1 {
		return fmt.Errorf("exactly one of match|cel|probe|anyOf required, got %d", n)
	}
	if len(ch.AnyOf) > 0 {
		for k, leaf := range ch.AnyOf {
			if len(leaf.AnyOf) > 0 {
				return fmt.Errorf("anyOf[%d]: nesting anyOf inside anyOf is not allowed (one level only)", k)
			}
			if err := leaf.validate(); err != nil {
				return fmt.Errorf("anyOf[%d]: %v", k, err)
			}
		}
	}
	return nil
}

// dependsOnCycle returns a human-readable cycle path, or "" if acyclic.
func (c *Challenge) dependsOnCycle() string {
	graph := map[string][]string{}
	for _, o := range c.Objectives {
		graph[o.ID] = o.DependsOn
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var path []string
	var dfs func(string) string
	dfs = func(n string) string {
		color[n] = gray
		path = append(path, n)
		for _, m := range graph[n] {
			if color[m] == gray {
				return strings.Join(append(path, m), " -> ")
			}
			if color[m] == white {
				if c := dfs(m); c != "" {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		color[n] = black
		return ""
	}
	for _, o := range c.Objectives {
		if color[o.ID] == white {
			if c := dfs(o.ID); c != "" {
				return c
			}
		}
	}
	return ""
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
