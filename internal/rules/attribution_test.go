package rules_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agarwalvivek29/kubedrill/internal/rules"
)

// loadFixtures reads the real captured audit events (Epic 3 recon spike) plus
// the one synthesized genuine-engine event.
func loadFixtures(t *testing.T) []rules.AuditEvent {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "audit-events.jsonl"))
	if err != nil {
		t.Fatalf("open fixtures: %v", err)
	}
	defer f.Close()
	var out []rules.AuditEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var e rules.AuditEvent
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan fixtures: %v", err)
	}
	return out
}

// byObject indexes fixtures by the object name they acted on, so tests can
// assert against a specific captured event.
func byObject(events []rules.AuditEvent) map[string]rules.AuditEvent {
	m := map[string]rules.AuditEvent{}
	for _, e := range events {
		if e.ObjectRef.Name != "" {
			m[e.ObjectRef.Name] = e
		}
	}
	return m
}

// TestAttributionOnRealEvents asserts AD-4 default-charge attribution against the
// real audit events captured from a kind cluster — one per actor class.
func TestAttributionOnRealEvents(t *testing.T) {
	events := loadFixtures(t)
	obj := byObject(events)

	cases := []struct {
		object string
		want   rules.Charge
		why    string
	}{
		{"player-cm", rules.ChargePlayer, "the player (kubernetes-admin) is charged"},
		{"sa-genuine", rules.ChargePlayer, "a player-created SA (not kube-system) is charged — the loophole is closed"},
		{"imp-cm", rules.ChargePlayer, "impersonating another user is still a player action"},
		{"tamper-cm", rules.ChargeTamper, "impersonating the engine identity is tampering"},
		{"topsecret", rules.ChargePlayer, "the player's secret write is charged (and logged Metadata-only)"},
		{"engine-setup-cm", rules.ChargeExempt, "the engine's own action is exempt"},
	}
	for _, c := range cases {
		e, ok := obj[c.object]
		if !ok {
			t.Fatalf("fixture for %q missing", c.object)
		}
		if got := rules.Attribute(e); got != c.want {
			t.Errorf("Attribute(%s) = %s, want %s — %s", c.object, got, c.want, c.why)
		}
	}
}

// TestAttributionExemptsControllers asserts every pinned control-plane identity
// captured from the real cluster is exempt.
func TestAttributionExemptsControllers(t *testing.T) {
	for _, e := range loadFixtures(t) {
		u := e.User.Username
		isController := u == "system:kube-controller-manager" ||
			u == "system:kube-scheduler" ||
			u == "system:apiserver" ||
			u == "system:kube-proxy" ||
			hasPrefix(u, "system:node:") ||
			hasPrefix(u, "system:serviceaccount:kube-system:")
		if !isController {
			continue
		}
		if got := rules.Attribute(e); got != rules.ChargeExempt {
			t.Errorf("controller %q: Attribute = %s, want exempt", u, got)
		}
	}
}

// TestAttributionTable covers the decision logic directly, including cases the
// live spike could not easily produce (impersonating a controller).
func TestAttributionTable(t *testing.T) {
	cases := []struct {
		name string
		ev   rules.AuditEvent
		want rules.Charge
	}{
		{"player admin", ev("kubernetes-admin", nil), rules.ChargePlayer},
		{"unknown identity is charged", ev("some:randomuser", nil), rules.ChargePlayer},
		{"player-created SA is charged", ev("system:serviceaccount:demo:jobsa", nil), rules.ChargePlayer},
		{"engine is exempt", ev(rules.EngineIdentity, nil), rules.ChargeExempt},
		{"kube-controller-manager is exempt", ev("system:kube-controller-manager", nil), rules.ChargeExempt},
		{"node/kubelet is exempt", ev("system:node:worker-1", nil), rules.ChargeExempt},
		{"kube-system SA is exempt", ev("system:serviceaccount:kube-system:job-controller", nil), rules.ChargeExempt},
		{"impersonating a user is charged to player", ev("kubernetes-admin", ptr("alice")), rules.ChargePlayer},
		{"impersonating a controller does NOT launder — charged", ev("kubernetes-admin", ptr("system:kube-controller-manager")), rules.ChargePlayer},
		{"impersonating the engine is tampering", ev("kubernetes-admin", ptr(rules.EngineIdentity)), rules.ChargeTamper},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rules.Attribute(c.ev); got != c.want {
				t.Errorf("Attribute = %s, want %s", got, c.want)
			}
		})
	}
}

func ev(user string, impersonated *string) rules.AuditEvent {
	e := rules.AuditEvent{Verb: "create", User: rules.Actor{Username: user}}
	if impersonated != nil {
		e.ImpersonatedUser = &rules.Actor{Username: *impersonated}
	}
	return e
}

func ptr(s string) *string { return &s }

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }
