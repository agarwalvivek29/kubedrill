package kind

import (
	"strings"
	"testing"

	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

func TestClusterConfigDefaultIsEmpty(t *testing.T) {
	if got := clusterConfig(api.EnvRequest{}); got != "" {
		t.Fatalf("default topology should yield empty config, got %q", got)
	}
	if got := clusterConfig(api.EnvRequest{ControlPlane: 1}); got != "" {
		t.Fatalf("single control-plane should yield empty config, got %q", got)
	}
}

func TestClusterConfigWithWorkers(t *testing.T) {
	got := clusterConfig(api.EnvRequest{ControlPlane: 1, Workers: 2})
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
