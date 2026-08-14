package rules

import (
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/challenge"
)

// Violation is a graded rule breach, carrying the evidence that triggered it so
// the scorecard can show the player exactly what they did (FR-10).
type Violation struct {
	RuleID   string     `json:"rule"`
	Type     string     `json:"type"` // deny | protect | require | tamper
	Message  string     `json:"message"`
	Fail     bool       `json:"fail,omitempty"`   // penalty: fail
	Points   int        `json:"points,omitempty"` // penalty: points deducted
	Evidence []Evidence `json:"evidence,omitempty"`
}

// Evidence is a lightweight projection of the audit event(s) behind a violation.
type Evidence struct {
	AuditID   string `json:"auditID,omitempty"`
	Verb      string `json:"verb"`
	User      string `json:"user"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// Grade evaluates a challenge's rules against the audit event stream, using AD-4
// attribution so only the player's own actions count (FR-10, AD-5):
//
//   - deny    violates iff a charged event matches the rule's target/operations.
//   - protect violates iff a charged delete (or deletecollection) touches the
//     target — a player must not remove/recreate a protected object.
//   - require violates iff NO charged event matches (the player was required to
//     do something and there's no evidence they did). A field-level require also
//     checks the captured request body contains the required fields.
//
// Impersonating the engine identity is surfaced as a tamper violation (fail).
// Rules are graded independently; the returned violations preserve rule order,
// with any tamper violation first.
func Grade(rs []v1alpha1.Rule, events []AuditEvent) []Violation {
	var charged []AuditEvent
	var tamper []AuditEvent
	for _, e := range events {
		switch Attribute(e) {
		case ChargePlayer:
			charged = append(charged, e)
		case ChargeTamper:
			tamper = append(tamper, e)
		}
	}

	var out []Violation
	if len(tamper) > 0 {
		out = append(out, Violation{
			RuleID:   "integrity",
			Type:     "tamper",
			Fail:     true,
			Message:  "impersonated the engine identity (" + EngineIdentity + ") — integrity breach",
			Evidence: evidences(tamper),
		})
	}

	for _, r := range rs {
		switch {
		case r.Deny != nil:
			if hits := matchSpec(r.Deny.Match, r.Deny.Operations, charged); len(hits) > 0 {
				out = append(out, ruleViolation(r, "deny",
					fmt.Sprintf("performed a denied action on %s", targetDesc(r.Deny.Match)), hits))
			}
		case r.Protect != nil:
			if hits := matchSpec(r.Protect.Match, []string{"delete", "deletecollection"}, charged); len(hits) > 0 {
				out = append(out, ruleViolation(r, "protect",
					fmt.Sprintf("deleted the protected %s", targetDesc(r.Protect.Match)), hits))
			}
		case r.Require != nil:
			if hits := matchRequire(r.Require, charged); len(hits) == 0 {
				out = append(out, ruleViolation(r, "require",
					fmt.Sprintf("required change to %s was not made", targetDesc(r.Require.Match)), nil))
			}
		}
	}
	return out
}

// matchSpec returns the charged events matching a rule target. If ops is
// non-empty, only those verbs match; otherwise any (mutating) verb matches —
// the audit policy only records mutating verbs, so reads never appear here.
func matchSpec(m v1alpha1.RuleMatch, ops []string, events []AuditEvent) []AuditEvent {
	var hits []AuditEvent
	for _, e := range events {
		if targetMatches(m, e) && (len(ops) == 0 || contains(ops, e.Verb)) {
			hits = append(hits, e)
		}
	}
	return hits
}

// matchRequire returns the charged events that satisfy a require rule: they hit
// the target with a create/update/patch (or the rule's explicit operations) and,
// for a field-level require, carry a request body containing the required fields.
func matchRequire(spec *v1alpha1.RuleSpec, events []AuditEvent) []AuditEvent {
	ops := spec.Operations
	if len(ops) == 0 {
		ops = []string{"create", "update", "patch"}
	}
	var hits []AuditEvent
	for _, e := range events {
		if !targetMatches(spec.Match, e) || !contains(ops, e.Verb) {
			continue
		}
		if len(spec.Fields) > 0 && !fieldsPresent(spec.Fields, e) {
			continue
		}
		hits = append(hits, e)
	}
	return hits
}

// targetMatches reports whether an event acted on the object a rule selects.
func targetMatches(m v1alpha1.RuleMatch, e AuditEvent) bool {
	gr := kindToResource(m.Kind)
	if e.ObjectRef.Resource != gr.resource {
		return false
	}
	if gr.group != "" && e.ObjectRef.APIGroup != "" && e.ObjectRef.APIGroup != gr.group {
		return false
	}
	if m.Name != "" && e.ObjectRef.Name != m.Name {
		return false
	}
	if m.Namespace != "" && e.ObjectRef.Namespace != m.Namespace {
		return false
	}
	return true
}

// fieldsPresent reports whether an event's captured request body contains the
// required fields (subset match). If no body was captured, it cannot be
// confirmed present.
func fieldsPresent(fields json.RawMessage, e AuditEvent) bool {
	if len(e.RequestObject) == 0 {
		return false
	}
	var tree, obj any
	if err := json.Unmarshal(fields, &tree); err != nil {
		return false
	}
	if err := json.Unmarshal(e.RequestObject, &obj); err != nil {
		return false
	}
	return challenge.Match(tree, obj)
}

func ruleViolation(r v1alpha1.Rule, typ, msg string, hits []AuditEvent) Violation {
	return Violation{
		RuleID:   r.ID,
		Type:     typ,
		Message:  msg,
		Fail:     r.Penalty.Fail,
		Points:   r.Penalty.Points,
		Evidence: evidences(hits),
	}
}

func evidences(events []AuditEvent) []Evidence {
	if len(events) == 0 {
		return nil
	}
	out := make([]Evidence, 0, len(events))
	for _, e := range events {
		out = append(out, Evidence{
			AuditID:   e.AuditID,
			Verb:      e.Verb,
			User:      e.User.Username,
			Resource:  e.ObjectRef.Resource,
			Namespace: e.ObjectRef.Namespace,
			Name:      e.ObjectRef.Name,
		})
	}
	return out
}

func targetDesc(m v1alpha1.RuleMatch) string {
	s := m.Kind
	if m.Namespace != "" {
		s += " " + m.Namespace + "/"
	} else if m.Name != "" {
		s += " "
	}
	if m.Name != "" {
		s += m.Name
	}
	return s
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
