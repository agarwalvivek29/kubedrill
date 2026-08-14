package kind

import (
	"strings"
	"testing"

	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

func TestClusterConfigDefaultIsEmpty(t *testing.T) {
	if got := clusterConfig(api.EnvRequest{}, ""); got != "" {
		t.Fatalf("default topology, no audit should yield empty config, got %q", got)
	}
	if got := clusterConfig(api.EnvRequest{ControlPlane: 1}, ""); got != "" {
		t.Fatalf("single control-plane, no audit should yield empty config, got %q", got)
	}
}

func TestClusterConfigWithWorkers(t *testing.T) {
	got := clusterConfig(api.EnvRequest{ControlPlane: 1, Workers: 2}, "")
	if !strings.Contains(got, "kind.x-k8s.io/v1alpha4") {
		t.Fatalf("missing apiVersion in config: %q", got)
	}
	if strings.Count(got, "role: control-plane") != 1 {
		t.Fatalf("want 1 control-plane, got %q", got)
	}
	if strings.Count(got, "role: worker") != 2 {
		t.Fatalf("want 2 workers, got %q", got)
	}
}

func TestClusterConfigWiresAuditWhenPolicyGiven(t *testing.T) {
	// Even a default single-node cluster must emit a config when audit is on.
	got := clusterConfig(api.EnvRequest{}, "/sessions/s1/audit-policy.yaml")
	if !strings.Contains(got, "role: control-plane") {
		t.Fatalf("audit config must define the control-plane node, got %q", got)
	}
	for _, want := range []string{
		"extraMounts:",
		"hostPath: /sessions/s1/audit-policy.yaml",
		"containerPath: " + auditPolicyNodePath,
		"kubeadmConfigPatches:",
		"kind: ClusterConfiguration",
		"name: audit-policy-file", // v1beta4 list-form extraArgs
		"value: " + auditPolicyNodePath,
		"name: audit-log-path",
		"pathType: DirectoryOrCreate",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("audit config missing %q\n---\n%s", want, got)
		}
	}
}

func TestClusterConfigAuditOnEveryControlPlane(t *testing.T) {
	got := clusterConfig(api.EnvRequest{ControlPlane: 3}, "/p.yaml")
	if strings.Count(got, "role: control-plane") != 3 {
		t.Fatalf("want 3 control-plane nodes, got %q", got)
	}
	// Every control-plane apiserver must be audited, not just the first.
	if n := strings.Count(got, "audit-policy-file"); n != 3 {
		t.Fatalf("want audit wiring on all 3 control-plane nodes, got %d", n)
	}
}

func TestNodeImageForFallsBackToDefault(t *testing.T) {
	if img := nodeImageFor("1.31"); img != "" {
		t.Fatalf("unknown/unpinned version should fall back to kind default (empty), got %q", img)
	}
	if img := nodeImageFor(""); img != "" {
		t.Fatalf("empty version should be default, got %q", img)
	}
}

func TestClusterNameRoundTrip(t *testing.T) {
	name := clusterName("abc123")
	if name != "kubedrill-abc123" {
		t.Fatalf("clusterName = %q", name)
	}
	if !strings.HasPrefix(name, clusterPrefix) {
		t.Fatalf("name %q missing ownership prefix", name)
	}
}
