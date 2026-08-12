package kind

import (
	"strings"

	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// nodeImageFor maps a requested Kubernetes minor to a pinned kind node image.
// Unknown or empty versions return "" so kind uses its bundled default node
// image (always valid for the installed kind version). Precise per-minor
// pinning is refined as the CI matrix settles; returning "" is the safe path.
func nodeImageFor(version string) string {
	switch strings.TrimPrefix(version, "v") {
	// Intentionally empty in v0.1: fall back to kind's default node image.
	// Add explicit "1.31": "kindest/node:v1.31.x@sha256:..." pins here once
	// the CI matrix fixes the exact digests (Story 2.5 / e2e lane).
	default:
		return ""
	}
}

// clusterConfig returns a kind v1alpha4 Config YAML when the request needs a
// non-default topology (worker nodes). Empty string means "use kind defaults"
// (a single control-plane node).
func clusterConfig(req api.EnvRequest) string {
	workers := req.Workers
	cp := req.ControlPlane
	if cp <= 1 && workers <= 0 {
		return ""
	}
	if cp < 1 {
		cp = 1
	}
	var b strings.Builder
	b.WriteString("kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n")
	for i := 0; i < cp; i++ {
		b.WriteString("  - role: control-plane\n")
	}
	for i := 0; i < workers; i++ {
		b.WriteString("  - role: worker\n")
	}
	return b.String()
}
