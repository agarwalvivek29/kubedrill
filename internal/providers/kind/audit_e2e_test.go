//go:build e2e

// End-to-end audit wiring against a real kind cluster. Requires Docker.
// Run with: go test -tags e2e -timeout 20m ./internal/providers/kind/...
package kind

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/rules"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// TestAuditWiringE2E provisions a cluster with a generated audit policy, performs
// a player action, and confirms the action streams back through AuditEvents with
// an advancing cursor — the full Story 3.1 path in production code.
func TestAuditWiringE2E(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// A challenge whose rules exercise BOTH tiers: a deny rule (Metadata baseline)
	// and a field-level require on ConfigMap (so the generated policy captures
	// configmap request bodies at Request level).
	ch := &v1alpha1.Challenge{Rules: []v1alpha1.Rule{
		{
			ID:      "no-delete-configmaps",
			Deny:    &v1alpha1.RuleSpec{Operations: []string{"delete"}, Match: v1alpha1.RuleMatch{Kind: "ConfigMap"}},
			Penalty: v1alpha1.Penalty{Points: 10},
		},
		{
			ID: "configmap-must-have-key",
			Require: &v1alpha1.RuleSpec{
				Match:  v1alpha1.RuleMatch{Kind: "ConfigMap"},
				Fields: json.RawMessage(`{"data":{"k":""}}`),
			},
			Penalty: v1alpha1.Penalty{Points: 10},
		},
	}}
	policy := rules.AuditPolicy(ch)
	if policy == "" {
		t.Fatal("expected a non-empty audit policy for a ruled challenge")
	}

	p := New()
	env, err := p.Provision(ctx, api.EnvRequest{
		SessionID:   "auditwire",
		SessionDir:  dir,
		AuditPolicy: policy,
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	t.Cleanup(func() { _ = p.Destroy(context.Background(), "auditwire") })

	// Player action: create a configmap via the player kubeconfig.
	kc, err := env.Kubeconfig()
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}
	cs := clientsetFrom(t, kc)
	if _, err := cs.CoreV1().ConfigMaps("default").Create(ctx,
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "player-marker"}, Data: map[string]string{"k": "v"}},
		metav1.CreateOptions{}); err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	// Read the whole stream from the start; it must contain our action.
	raw, cursor, err := env.AuditEvents(ctx, 0)
	if err != nil {
		t.Fatalf("AuditEvents: %v", err)
	}
	if cursor == 0 || len(raw) == 0 {
		t.Fatalf("expected audit bytes and a non-zero cursor, got %d bytes cursor=%d", len(raw), cursor)
	}
	events := decodeEvents(t, raw)
	if !containsAction(events, "create", "configmaps", "player-marker") {
		t.Fatalf("player configmap create not found in %d audit events", len(events))
	}
	// The secret rule pins secrets to Metadata; confirm configmaps carry a body
	// (proving the generated two-tier policy took effect in a real apiserver).
	if !anyHasRequestBody(events, "configmaps") {
		t.Fatalf("expected at least one configmap event at Request level with a body")
	}

	// Cursor semantics: resuming returns only new bytes, never the whole log.
	raw2, cursor2, err := env.AuditEvents(ctx, cursor)
	if err != nil {
		t.Fatalf("AuditEvents resume: %v", err)
	}
	if cursor2 < cursor {
		t.Fatalf("cursor went backwards: %d -> %d", cursor, cursor2)
	}
	if len(raw2) > len(raw) {
		t.Fatalf("resume returned more than the full initial read (%d > %d) — not incremental", len(raw2), len(raw))
	}
}

func clientsetFrom(t *testing.T, kubeconfig []byte) *kubernetes.Clientset {
	t.Helper()
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		t.Fatalf("rest config: %v", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	return cs
}

func decodeEvents(t *testing.T, raw []byte) []rules.AuditEvent {
	t.Helper()
	var out []rules.AuditEvent
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var e rules.AuditEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // tolerate non-event lines
		}
		out = append(out, e)
	}
	return out
}

func containsAction(events []rules.AuditEvent, verb, resource, name string) bool {
	for _, e := range events {
		if e.Verb == verb && e.ObjectRef.Resource == resource && e.ObjectRef.Name == name {
			return true
		}
	}
	return false
}

func anyHasRequestBody(events []rules.AuditEvent, resource string) bool {
	for _, e := range events {
		if e.ObjectRef.Resource == resource && len(e.RequestObject) > 0 {
			return true
		}
	}
	return false
}
