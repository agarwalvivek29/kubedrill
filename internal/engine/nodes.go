package engine

import (
	"context"
	"fmt"
	"strings"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// challengeUsesNodeExec reports whether any fault needs node-level access.
func challengeUsesNodeExec(ch *v1alpha1.Challenge) bool {
	for _, f := range ch.Environment.Setup.Faults {
		if f.NodeExec != nil {
			return true
		}
	}
	return false
}

// applyNodeFaults runs the challenge's nodeExec faults through the Environment
// (root on the node) — e.g. stopping the kubelet or corrupting a static-pod
// manifest. Applied after setup and enforcement so those are not disrupted.
func (e *Engine) applyNodeFaults(ctx context.Context, env api.Environment, ch *v1alpha1.Challenge, prog Progressf) error {
	for _, f := range ch.Environment.Setup.Faults {
		if f.NodeExec == nil {
			continue
		}
		node := f.NodeExec.Node
		if node == "" {
			node = "control-plane"
		}
		prog("applying node fault %q on %s...", f.Name, node)
		if out, err := env.NodeExec(ctx, f.NodeExec.Node, f.NodeExec.Command); err != nil {
			return fmt.Errorf("node fault %q: %w\n%s", f.Name, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
