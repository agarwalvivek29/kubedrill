// Package kind implements the kind EnvProvider (AD-9): ephemeral cluster
// lifecycle, two-identity certs, kubeconfig isolation, and name-prefix
// ownership for orphan reconciliation.
package kind

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"

	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// clusterPrefix marks kind clusters this tool owns. kind manages the Docker
// container labels itself, so the cluster NAME prefix is our ownership marker
// (equivalent to a dedicated Docker label for reconciliation — AD-9).
const clusterPrefix = "kubedrill-"

// Provider is the kind-backed api.EnvProvider.
type Provider struct {
	kind *cluster.Provider
}

// New constructs a kind Provider using the default (docker) runtime.
func New() *Provider {
	return &Provider{kind: cluster.NewProvider()}
}

// Name implements api.EnvProvider.
func (*Provider) Name() string { return "kind" }

// Capabilities implements api.EnvProvider. kind can preload images and, in
// Epic 3, surface audit logs and exec on nodes. AuditLog/NodeExec are reported
// true because the kind control plane supports both; the engine wiring for
// them lands in Epic 3.
func (*Provider) Capabilities() api.Capabilities {
	return api.Capabilities{
		AuditLog:     true,
		NodeExec:     true,
		ImagePreload: true,
		MultiNode:    true,
	}
}

// clusterName returns the kind cluster name for a session id.
func clusterName(sessionID string) string { return clusterPrefix + sessionID }

// Provision implements api.EnvProvider.
func (p *Provider) Provision(ctx context.Context, req api.EnvRequest) (api.Environment, error) {
	if req.SessionID == "" || req.SessionDir == "" {
		return nil, fmt.Errorf("kind: SessionID and SessionDir are required")
	}
	if err := os.MkdirAll(req.SessionDir, 0o755); err != nil {
		return nil, fmt.Errorf("kind: session dir: %w", err)
	}
	name := clusterName(req.SessionID)
	playerPath := filepath.Join(req.SessionDir, "kubeconfig")

	// When the challenge has rules, the engine passes an audit policy. Write it to
	// the session dir so kind can mount it into the control-plane node(s).
	auditPolicyHostPath := ""
	if req.AuditPolicy != "" {
		auditPolicyHostPath = filepath.Join(req.SessionDir, "audit-policy.yaml")
		if err := os.WriteFile(auditPolicyHostPath, []byte(req.AuditPolicy), 0o644); err != nil {
			return nil, fmt.Errorf("kind: write audit policy: %w", err)
		}
	}

	opts := []cluster.CreateOption{
		// Write the player kubeconfig ONLY here — never merge into ~/.kube/config.
		cluster.CreateWithKubeconfigPath(playerPath),
		cluster.CreateWithDisplayUsage(false),
		cluster.CreateWithDisplaySalutation(false),
	}
	if img := nodeImageFor(req.KubernetesVersion); img != "" {
		opts = append(opts, cluster.CreateWithNodeImage(img))
	}
	if cfg := clusterConfig(req, auditPolicyHostPath); cfg != "" {
		opts = append(opts, cluster.CreateWithRawConfig([]byte(cfg)))
	}

	if err := p.kind.Create(name, opts...); err != nil {
		return nil, fmt.Errorf("kind: create cluster %q: %w", name, err)
	}

	// Mint the distinct engine identity and write it alongside the player
	// kubeconfig (never surfaced to player-facing commands).
	enginePath := filepath.Join(req.SessionDir, "engine-kubeconfig")
	if err := p.mintEngineKubeconfig(name, playerPath, enginePath); err != nil {
		// Best-effort teardown so a failed provision leaves no cluster.
		_ = p.kind.Delete(name, "")
		return nil, fmt.Errorf("kind: mint engine identity: %w", err)
	}

	return &environment{
		id:         req.SessionID,
		playerPath: playerPath,
		enginePath: enginePath,
		labels:     req.Labels,
		prov:       p,
		cluster:    name,
		audit:      auditPolicyHostPath != "",
	}, nil
}

// Destroy implements api.EnvProvider. Idempotent.
func (p *Provider) Destroy(_ context.Context, envID string) error {
	if err := p.kind.Delete(clusterName(envID), ""); err != nil {
		return fmt.Errorf("kind: delete cluster for %q: %w", envID, err)
	}
	return nil
}

// List implements api.EnvProvider by reconciling on the cluster-name prefix.
func (p *Provider) List(_ context.Context) ([]api.EnvInfo, error) {
	names, err := p.kind.List()
	if err != nil {
		return nil, fmt.Errorf("kind: list clusters: %w", err)
	}
	var out []api.EnvInfo
	for _, n := range names {
		if !strings.HasPrefix(n, clusterPrefix) {
			continue
		}
		id := strings.TrimPrefix(n, clusterPrefix)
		out = append(out, api.EnvInfo{
			ID:     id,
			Name:   n,
			Labels: map[string]string{"dev.kubedrill.session": id},
		})
	}
	return out, nil
}

// LoadImages implements api.EnvProvider, loading image archives into all nodes.
func (p *Provider) LoadImages(_ context.Context, envID string, tarPaths []string) error {
	name := clusterName(envID)
	nodeList, err := p.kind.ListNodes(name)
	if err != nil {
		return fmt.Errorf("kind: list nodes for %q: %w", envID, err)
	}
	for _, tar := range tarPaths {
		f, err := os.Open(tar)
		if err != nil {
			return fmt.Errorf("kind: open image archive %q: %w", tar, err)
		}
		for _, node := range nodeList {
			if _, err := f.Seek(0, 0); err != nil {
				f.Close()
				return fmt.Errorf("kind: rewind %q: %w", tar, err)
			}
			cmd := node.Command("ctr", "--namespace=k8s.io", "images", "import", "-")
			cmd.SetStdin(f)
			if err := cmd.Run(); err != nil {
				f.Close()
				return fmt.Errorf("kind: import %q into %s: %w", tar, node.String(), err)
			}
		}
		f.Close()
	}
	return nil
}

// controlPlane returns the control-plane node for a cluster.
func (p *Provider) controlPlane(name string) (nodeCommander, error) {
	nodeList, err := p.kind.ListNodes(name)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	cps, err := nodeutils.ControlPlaneNodes(nodeList)
	if err != nil {
		return nil, fmt.Errorf("control-plane nodes: %w", err)
	}
	if len(cps) == 0 {
		return nil, fmt.Errorf("no control-plane node found")
	}
	return cps[0], nil
}
