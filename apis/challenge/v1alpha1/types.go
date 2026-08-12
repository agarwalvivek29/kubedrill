package v1alpha1

import "encoding/json"

// APIVersion and Kind are the fixed discriminators the loader dispatches on.
const (
	APIVersion = "kubedrill.dev/v1alpha1"
	Kind       = "Challenge"
)

// Difficulty enumerates the allowed challenge difficulties.
type Difficulty string

const (
	Easy   Difficulty = "easy"
	Medium Difficulty = "medium"
	Hard   Difficulty = "hard"
	Expert Difficulty = "expert"
)

// Challenge is the top-level kubedrill.dev/v1alpha1 Challenge document.
type Challenge struct {
	APIVersion  string      `json:"apiVersion"`
	Kind        string      `json:"kind"`
	Metadata    Metadata    `json:"metadata"`
	Environment Environment `json:"environment"`
	Objectives  []Objective `json:"objectives"`
	Rules       []Rule      `json:"rules,omitempty"`
	Hints       []Hint      `json:"hints,omitempty"`
	Solution    Solution    `json:"solution"`
}

// Metadata carries identity and presentation for a challenge.
type Metadata struct {
	Name             string     `json:"name"`
	Version          string     `json:"version"`
	Title            string     `json:"title"`
	Description      string     `json:"description,omitempty"`
	Difficulty       Difficulty `json:"difficulty"`
	Topics           []string   `json:"topics,omitempty"`
	Exam             []string   `json:"exam,omitempty"`
	TimeLimit        string     `json:"timeLimit,omitempty"` // Go duration; empty = untimed
	NodeAccess       bool       `json:"nodeAccess,omitempty"`
	MinEngineVersion string     `json:"minEngineVersion,omitempty"`
}

// Environment describes the cluster and the broken-as-intended setup.
type Environment struct {
	Cluster Cluster  `json:"cluster"`
	Images  []string `json:"images,omitempty"`
	Setup   Setup    `json:"setup"`
}

// Cluster pins the kind cluster shape.
type Cluster struct {
	Provider          string `json:"provider,omitempty"` // any|kind (default any)
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	Nodes             *Nodes `json:"nodes,omitempty"`
}

// Nodes is the node topology; nil means a single control-plane node.
type Nodes struct {
	ControlPlane int `json:"controlPlane"`
	Workers      int `json:"workers"`
}

// Setup applies manifests, injects faults, and gates on readiness.
type Setup struct {
	Manifests []ManifestRef `json:"manifests,omitempty"`
	Faults    []Fault       `json:"faults,omitempty"`
	Readiness []Check       `json:"readiness,omitempty"`
}

// ManifestRef points at a manifest file relative to the challenge dir.
type ManifestRef struct {
	Path string `json:"path"`
}

// Fault mutates the environment into its broken state. Exactly one of Patch,
// Exec, or NodeExec is set (validated in validation.go).
type Fault struct {
	Name     string    `json:"name"`
	Patch    *Patch    `json:"patch,omitempty"`
	Exec     *Exec     `json:"exec,omitempty"`
	NodeExec *NodeExec `json:"nodeExec,omitempty"`
}

// Patch is a strategic-merge or JSON patch against a live object.
type Patch struct {
	Target Target          `json:"target"`
	Type   string          `json:"type"` // merge|json
	Data   json.RawMessage `json:"data"`
}

// Exec runs a host command (escape hatch for setup noise).
type Exec struct {
	Command []string `json:"command"`
}

// NodeExec runs a command on a cluster node (nodeAccess challenges only).
type NodeExec struct {
	Node    string   `json:"node"`
	Command []string `json:"command"`
}

// Target selects a single object. LabelSelector belongs on Targets, not here.
type Target struct {
	Kind       string `json:"kind"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// Targets selects a set of objects via a label selector.
type Targets struct {
	Kind          string `json:"kind"`
	Namespace     string `json:"namespace,omitempty"`
	LabelSelector string `json:"labelSelector,omitempty"`
	APIVersion    string `json:"apiVersion,omitempty"`
}

// Objective is a scored, stably-identified sub-goal.
type Objective struct {
	ID                     string   `json:"id"`
	Title                  string   `json:"title"`
	Points                 int      `json:"points"`
	DependsOn              []string `json:"dependsOn,omitempty"`
	ExpectInitiallyPassing bool     `json:"expectInitiallyPassing,omitempty"`
	Checks                 []Check  `json:"checks"`
}

// Check is one verification assertion. Exactly one of Match, CEL, Probe, or
// AnyOf is set (validated). AnyOf provides the single level of OR nesting.
type Check struct {
	Match *MatchCheck     `json:"match,omitempty"`
	CEL   string          `json:"cel,omitempty"`
	Probe *Probe          `json:"probe,omitempty"`
	AnyOf []Check         `json:"anyOf,omitempty"`
	Poll  *Poll           `json:"poll,omitempty"`
	// Target/Targets apply to cel checks (match carries its own target).
	Target  *Target  `json:"target,omitempty"`
	Targets *Targets `json:"targets,omitempty"`
}

// MatchCheck asserts a partial-object subset tree against a target object.
type MatchCheck struct {
	Target Target          `json:"target"`
	Object json.RawMessage `json:"object"`
}

// Probe runs a script as an in-cluster Job at an author-chosen namespace.
type Probe struct {
	Image     string `json:"image"`
	Script    string `json:"script"`
	Namespace string `json:"namespace,omitempty"` // default kubedrill-system
	Timeout   string `json:"timeout,omitempty"`
}

// Poll controls polling: Timeout bounds retries; Window (if set) requires the
// check to hold continuously for the duration.
type Poll struct {
	Timeout string `json:"timeout,omitempty"`
	Window  string `json:"window,omitempty"`
}

// RuleVerb is the frozen rule-verb set (AD-5/AD-6).
type RuleVerb string

const (
	Deny    RuleVerb = "deny"
	Protect RuleVerb = "protect"
	Require RuleVerb = "require"
)

// Rule is an audit-graded constraint. Exactly one of Deny, Protect, Require
// is set (validated).
type Rule struct {
	ID      string     `json:"id"`
	Deny    *RuleSpec  `json:"deny,omitempty"`
	Protect *RuleSpec  `json:"protect,omitempty"`
	Require *RuleSpec  `json:"require,omitempty"`
	Penalty Penalty    `json:"penalty"`
	Enforce bool       `json:"enforce,omitempty"` // deny/protect only; lint-error on require
}

// RuleSpec describes the actions a rule matches.
type RuleSpec struct {
	Operations  []string        `json:"operations,omitempty"`
	Match       RuleMatch       `json:"match"`
	Fields      json.RawMessage `json:"fields,omitempty"` // field-level require (Request-tier)
	Description string          `json:"description,omitempty"`
}

// RuleMatch selects the objects/kinds a rule applies to.
type RuleMatch struct {
	Kind      string `json:"kind"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// Penalty is either the sentinel "fail" or a points deduction. Exactly one is
// meaningful; Fail takes precedence when true.
type Penalty struct {
	Fail   bool
	Points int
}

// UnmarshalJSON accepts either `penalty: fail` or `penalty: {points: N}`.
func (p *Penalty) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		p.Fail = s == "fail"
		return nil
	}
	var obj struct {
		Points int `json:"points"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	p.Points = obj.Points
	return nil
}

// MarshalJSON emits the same two shapes.
func (p Penalty) MarshalJSON() ([]byte, error) {
	if p.Fail {
		return json.Marshal("fail")
	}
	return json.Marshal(struct {
		Points int `json:"points"`
	}{Points: p.Points})
}

// Hint is a progressive, penalized disclosure.
type Hint struct {
	ID      string `json:"id"`
	Penalty int    `json:"penalty"`
	Text    string `json:"text"`
}

// Solution is the reference explanation + executable solve script.
type Solution struct {
	Explanation string `json:"explanation,omitempty"`
	Script      string `json:"script"`
}
