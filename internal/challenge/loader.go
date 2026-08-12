package challenge

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
)

// Loaded is a fully validated challenge plus the directory it came from.
type Loaded struct {
	Dir       string
	Challenge *v1alpha1.Challenge
}

// LoadDir reads and fully validates a challenge directory: it strict-decodes
// challenge.yaml (unknown fields error), dispatches on (apiVersion, kind),
// runs semantic validation, then layers referential checks (referenced files
// exist) and compile checks (every match tree decodes, every CEL expression
// type-checks). It never partially loads — any failure returns an error and a
// nil challenge.
func LoadDir(dir string) (*Loaded, error) {
	path := filepath.Join(dir, "challenge.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Strict decode: reject unknown/misspelled fields (AD-6).
	var c v1alpha1.Challenge
	if err := yaml.UnmarshalStrict(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: strict decode failed: %w", path, err)
	}

	// Dispatch discriminators (explicit, so a wrong doc gets a clear message).
	if c.APIVersion != v1alpha1.APIVersion || c.Kind != v1alpha1.Kind {
		return nil, fmt.Errorf("%s: unsupported document (apiVersion=%q kind=%q); want %q/%q",
			path, c.APIVersion, c.Kind, v1alpha1.APIVersion, v1alpha1.Kind)
	}

	// Semantic validation (ids, unions, acyclic dependsOn).
	if err := c.Validate(); err != nil {
		return nil, err
	}

	// Referential checks: files the challenge points at must exist.
	if err := checkRefs(dir, &c); err != nil {
		return nil, err
	}

	// Compile checks: match trees decode, CEL type-checks.
	if err := checkCompiles(&c); err != nil {
		return nil, err
	}

	return &Loaded{Dir: dir, Challenge: &c}, nil
}

func checkRefs(dir string, c *v1alpha1.Challenge) error {
	mustExist := func(rel, where string) error {
		if rel == "" {
			return nil
		}
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			return fmt.Errorf("%s: referenced file %q not found", where, rel)
		}
		return nil
	}
	for i, m := range c.Environment.Setup.Manifests {
		if err := mustExist(m.Path, fmt.Sprintf("environment.setup.manifests[%d]", i)); err != nil {
			return err
		}
	}
	for _, o := range c.Objectives {
		for j, ch := range o.Checks {
			if ch.Probe != nil {
				if err := mustExist(ch.Probe.Script, fmt.Sprintf("objective %q checks[%d].probe.script", o.ID, j)); err != nil {
					return err
				}
			}
		}
	}
	if err := mustExist(c.Solution.Script, "solution.script"); err != nil {
		return err
	}
	return mustExist(c.Solution.Explanation, "solution.explanation")
}

func checkCompiles(c *v1alpha1.Challenge) error {
	var walk func(checks []v1alpha1.Check, where string) error
	walk = func(checks []v1alpha1.Check, where string) error {
		for i, ch := range checks {
			w := fmt.Sprintf("%s[%d]", where, i)
			switch {
			case ch.Match != nil:
				if _, err := decodeTree(ch.Match.Object, w); err != nil {
					return err
				}
			case ch.CEL != "":
				if err := CompileCEL(ch.CEL, w); err != nil {
					return err
				}
			case len(ch.AnyOf) > 0:
				if err := walk(ch.AnyOf, w+".anyOf"); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, o := range c.Objectives {
		if err := walk(o.Checks, fmt.Sprintf("objective %q checks", o.ID)); err != nil {
			return err
		}
	}
	return walk(c.Environment.Setup.Readiness, "environment.setup.readiness")
}
