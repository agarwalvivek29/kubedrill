package kind

import (
	"fmt"
	"strings"

	"github.com/agarwalvivek29/kubedrill/internal/rules"
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

// The in-node paths the audit policy is mounted at and the log is written to.
// auditLogDir is the parent of rules.AuditLogPath and is created on the node.
const (
	auditPolicyNodePath = "/etc/kubernetes/audit/policy.yaml"
	auditLogDir         = "/var/log/kubernetes/audit"
)

// clusterConfig returns a kind v1alpha4 Config YAML, or "" to use kind's default
// (a single control-plane node with no audit). A config is emitted when the
// request needs a non-default topology (worker or extra control-plane nodes) OR
// when audit is wired (auditPolicyHostPath != ""). Every control-plane node gets
// the audit apiserver patches so grading works regardless of which apiserver
// served a request.
func clusterConfig(req api.EnvRequest, auditPolicyHostPath string) string {
	audit := auditPolicyHostPath != ""
	workers := req.Workers
	cp := req.ControlPlane
	if cp < 1 {
		cp = 1
	}
	if !audit && cp <= 1 && workers <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n")
	for i := 0; i < cp; i++ {
		b.WriteString("  - role: control-plane\n")
		if audit {
			writeAuditNode(&b, auditPolicyHostPath)
		}
	}
	for i := 0; i < workers; i++ {
		b.WriteString("  - role: worker\n")
	}
	return b.String()
}

// writeAuditNode appends the per-control-plane-node audit wiring: mount the host
// policy file into the node, and patch the apiserver (kubeadm v1beta4: extraArgs
// is a LIST) to read that policy and write the log to a DirectoryOrCreate volume.
// This exact recipe was validated on a live kind cluster (k8s 1.36) before it
// was productionized.
func writeAuditNode(b *strings.Builder, hostPolicy string) {
	fmt.Fprintf(b, "    extraMounts:\n")
	fmt.Fprintf(b, "      - hostPath: %s\n", hostPolicy)
	fmt.Fprintf(b, "        containerPath: %s\n", auditPolicyNodePath)
	fmt.Fprintf(b, "        readOnly: true\n")
	b.WriteString("    kubeadmConfigPatches:\n")
	b.WriteString("      - |\n")
	b.WriteString("        kind: ClusterConfiguration\n")
	b.WriteString("        apiServer:\n")
	b.WriteString("          extraArgs:\n")
	fmt.Fprintf(b, "          - name: audit-policy-file\n            value: %s\n", auditPolicyNodePath)
	fmt.Fprintf(b, "          - name: audit-log-path\n            value: %s\n", rules.AuditLogPath)
	b.WriteString("          - name: audit-log-maxsize\n            value: \"100\"\n")
	b.WriteString("          - name: audit-log-maxbackup\n            value: \"1\"\n")
	b.WriteString("          extraVolumes:\n")
	fmt.Fprintf(b, "          - name: audit-policy\n            hostPath: %s\n            mountPath: %s\n            readOnly: true\n            pathType: File\n", auditPolicyNodePath, auditPolicyNodePath)
	fmt.Fprintf(b, "          - name: audit-logs\n            hostPath: %s\n            mountPath: %s\n            readOnly: false\n            pathType: DirectoryOrCreate\n", auditLogDir, auditLogDir)
}
