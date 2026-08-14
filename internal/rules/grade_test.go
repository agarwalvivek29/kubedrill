package rules_test

import (
	"encoding/json"
	"testing"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/rules"
)

// event builds a charged (player) audit event acting on an object.
func event(verb, resource, ns, name string) rules.AuditEvent {
	return rules.AuditEvent{
		Verb:      verb,
		User:      rules.Actor{Username: "kubernetes-admin"},
		ObjectRef: rules.ObjectRef{Resource: resource, Namespace: ns, Name: name},
	}
}

func findViolation(vs []rules.Violation, ruleID string) *rules.Violation {
	for i := range vs {
		if vs[i].RuleID == ruleID {
			return &vs[i]
		}
	}
	return nil
}

func TestGradeDeny(t *testing.T) {
	rs := []v1alpha1.Rule{{
		ID:      "no-touch-kube-system",
		Deny:    &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "ConfigMap", Namespace: "kube-system"}},
		Penalty: v1alpha1.Penalty{Points: 25},
	}}

	// A charged create in kube-system violates.
	vs := rules.Grade(rs, []rules.AuditEvent{event("create", "configmaps", "kube-system", "x")})
	v := findViolation(vs, "no-touch-kube-system")
	if v == nil || v.Type != "deny" || v.Points != 25 {
		t.Fatalf("expected a deny violation with 25 points, got %+v", vs)
	}
	if len(v.Evidence) != 1 || v.Evidence[0].Verb != "create" {
		t.Fatalf("violation must carry the triggering event, got %+v", v.Evidence)
	}

	// An action in another namespace does not.
	vs = rules.Grade(rs, []rules.AuditEvent{event("create", "configmaps", "default", "x")})
	if findViolation(vs, "no-touch-kube-system") != nil {
		t.Fatalf("action outside the target namespace should not violate: %+v", vs)
	}
}

func TestGradeDenyRespectsOperations(t *testing.T) {
	rs := []v1alpha1.Rule{{
		ID:      "no-delete-svc",
		Deny:    &v1alpha1.RuleSpec{Operations: []string{"delete"}, Match: v1alpha1.RuleMatch{Kind: "Service"}},
		Penalty: v1alpha1.Penalty{Fail: true},
	}}
	if vs := rules.Grade(rs, []rules.AuditEvent{event("create", "services", "default", "s")}); findViolation(vs, "no-delete-svc") != nil {
		t.Fatalf("create should not trip a delete-only deny: %+v", vs)
	}
	vs := rules.Grade(rs, []rules.AuditEvent{event("delete", "services", "default", "s")})
	v := findViolation(vs, "no-delete-svc")
	if v == nil || !v.Fail {
		t.Fatalf("delete should trip the deny with a fail penalty, got %+v", vs)
	}
}

func TestGradeProtect(t *testing.T) {
	rs := []v1alpha1.Rule{{
		ID:      "keep-web",
		Protect: &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "Deployment", Namespace: "shop", Name: "web"}},
		Penalty: v1alpha1.Penalty{Fail: true},
	}}
	// A charged delete of the protected object violates.
	vs := rules.Grade(rs, []rules.AuditEvent{event("delete", "deployments", "shop", "web")})
	if v := findViolation(vs, "keep-web"); v == nil || v.Type != "protect" || !v.Fail {
		t.Fatalf("delete of protected object should violate with fail, got %+v", vs)
	}
	// A mere update does NOT (protect is about removal).
	if vs := rules.Grade(rs, []rules.AuditEvent{event("update", "deployments", "shop", "web")}); findViolation(vs, "keep-web") != nil {
		t.Fatalf("update should not trip protect: %+v", vs)
	}
}

func TestGradeRequireObjectLevel(t *testing.T) {
	rs := []v1alpha1.Rule{{
		ID:      "must-create-quota",
		Require: &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "ResourceQuota", Namespace: "team"}},
		Penalty: v1alpha1.Penalty{Points: 30},
	}}
	// No matching event -> require violates.
	if v := findViolation(rules.Grade(rs, nil), "must-create-quota"); v == nil || v.Type != "require" {
		t.Fatalf("missing required action should violate, got %+v", rules.Grade(rs, nil))
	}
	// A matching create satisfies it (no violation).
	vs := rules.Grade(rs, []rules.AuditEvent{event("create", "resourcequotas", "team", "q")})
	if findViolation(vs, "must-create-quota") != nil {
		t.Fatalf("required action present should not violate: %+v", vs)
	}
}

func TestGradeRequireFieldLevel(t *testing.T) {
	rs := []v1alpha1.Rule{{
		ID: "cm-needs-key",
		Require: &v1alpha1.RuleSpec{
			Match:  v1alpha1.RuleMatch{Kind: "ConfigMap", Namespace: "app", Name: "cfg"},
			Fields: json.RawMessage(`{"data":{"FEATURE":"on"}}`),
		},
		Penalty: v1alpha1.Penalty{Points: 15},
	}}
	withBody := func(body string) rules.AuditEvent {
		e := event("update", "configmaps", "app", "cfg")
		e.RequestObject = json.RawMessage(body)
		return e
	}

	// Body has the required field -> satisfied.
	vs := rules.Grade(rs, []rules.AuditEvent{withBody(`{"data":{"FEATURE":"on","other":"z"}}`)})
	if findViolation(vs, "cm-needs-key") != nil {
		t.Fatalf("body with required field should satisfy require: %+v", vs)
	}
	// Body missing / wrong value -> violates.
	vs = rules.Grade(rs, []rules.AuditEvent{withBody(`{"data":{"FEATURE":"off"}}`)})
	if findViolation(vs, "cm-needs-key") == nil {
		t.Fatalf("body without required field should violate: %+v", vs)
	}
	// A matching event with NO captured body cannot confirm the field -> violates.
	vs = rules.Grade(rs, []rules.AuditEvent{event("update", "configmaps", "app", "cfg")})
	if findViolation(vs, "cm-needs-key") == nil {
		t.Fatalf("no body means the field is unconfirmed -> violate: %+v", vs)
	}
}

// TestGradeExemptAndTamperOnRealEvents grades against the real captured fixtures:
// controller/engine actions must never trip a rule, and the engine-impersonation
// fixture must produce a tamper (fail) violation.
func TestGradeExemptAndTamperOnRealEvents(t *testing.T) {
	events := loadFixtures(t)

	// A broad deny on configmaps in the demo namespace. The controllers and the
	// engine also touch configmaps cluster-wide, but only the PLAYER's charged
	// actions may trip it.
	rs := []v1alpha1.Rule{{
		ID:      "no-configmaps",
		Deny:    &v1alpha1.RuleSpec{Match: v1alpha1.RuleMatch{Kind: "ConfigMap"}},
		Penalty: v1alpha1.Penalty{Points: 5},
	}}
	vs := rules.Grade(rs, events)

	// Tamper fixture (engine impersonation) must fail.
	if tv := findViolation(vs, "integrity"); tv == nil || !tv.Fail {
		t.Fatalf("expected a tamper fail violation from the engine-impersonation fixture, got %+v", vs)
	}
	// The deny should have tripped (the player created configmaps), and every
	// evidence event must be a charged player action — never a controller/engine.
	dv := findViolation(vs, "no-configmaps")
	if dv == nil {
		t.Fatalf("expected the configmap deny to trip on player actions, got %+v", vs)
	}
	for _, ev := range dv.Evidence {
		if ev.User == rules.EngineIdentity ||
			hasPrefix(ev.User, "system:kube-") ||
			hasPrefix(ev.User, "system:node:") ||
			hasPrefix(ev.User, "system:serviceaccount:kube-system:") ||
			ev.User == "system:apiserver" {
			t.Fatalf("exempt actor %q leaked into violation evidence", ev.User)
		}
	}
}
