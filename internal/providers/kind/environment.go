package kind

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/agarwalvivek29/kubedrill/internal/rules"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// environment is the kind-backed api.Environment. It is kubeconfig-shaped:
// callers get bytes, never a live client (AD-3).
type environment struct {
	id         string
	playerPath string
	enginePath string
	labels     map[string]string

	// prov and cluster let AuditEvents read the audit log off the control-plane
	// node; audit is true only when an audit policy was wired at provision time.
	prov    *Provider
	cluster string
	audit   bool
}

func (e *environment) ID() string { return e.id }

func (e *environment) Kubeconfig() ([]byte, error) {
	b, err := os.ReadFile(e.playerPath)
	if err != nil {
		return nil, fmt.Errorf("kind: read player kubeconfig: %w", err)
	}
	return b, nil
}

func (e *environment) EngineKubeconfig() ([]byte, error) {
	b, err := os.ReadFile(e.enginePath)
	if err != nil {
		return nil, fmt.Errorf("kind: read engine kubeconfig: %w", err)
	}
	return b, nil
}

func (e *environment) Labels() map[string]string {
	if e.labels == nil {
		return map[string]string{}
	}
	return e.labels
}

// AuditEvents streams new audit-log bytes from the control-plane node starting
// at `from` (a byte offset). It reads only from the cursor to end of file (never
// the whole log on resume, AD-3) and trims to the last complete line so a
// half-written final event is re-read next time rather than split. When no audit
// policy was wired, it is a no-op.
func (e *environment) AuditEvents(ctx context.Context, from api.AuditCursor) ([]byte, api.AuditCursor, error) {
	if !e.audit {
		return nil, from, nil
	}
	cp, err := e.prov.controlPlane(e.cluster)
	if err != nil {
		return nil, from, fmt.Errorf("kind: audit: %w", err)
	}
	// tail -c +N is 1-indexed: +1 is the whole file, +(offset+1) resumes.
	script := fmt.Sprintf("tail -c +%d %s 2>/dev/null || true", int64(from)+1, rules.AuditLogPath)
	var out, errb bytes.Buffer
	cmd := cp.Command("sh", "-c", script)
	cmd.SetStdout(&out)
	cmd.SetStderr(&errb)
	if err := cmd.Run(); err != nil {
		return nil, from, fmt.Errorf("kind: read audit log: %w (%s)", err, errb.String())
	}
	b := out.Bytes()
	// Trim to the last newline so we never hand back a partial final line.
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		b = b[:i+1]
	} else {
		b = nil // no complete line available yet
	}
	return b, from + api.AuditCursor(len(b)), nil
}

// NodeExec runs a command on a named node as root and returns combined output.
func (e *environment) NodeExec(_ context.Context, node string, command []string) ([]byte, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("kind: node exec: empty command")
	}
	n, err := e.prov.findNode(e.cluster, node)
	if err != nil {
		return nil, fmt.Errorf("kind: node exec: %w", err)
	}
	var out bytes.Buffer
	cmd := n.Command(command[0], command[1:]...)
	cmd.SetStdout(&out)
	cmd.SetStderr(&out)
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("kind: node exec %v on %s: %w", command, n.String(), err)
	}
	return out.Bytes(), nil
}

// NodeShellCommand returns the argv for an interactive root shell on a node.
// kind nodes are containers, so this is `docker exec -it <container> bash`.
func (e *environment) NodeShellCommand(node string) ([]string, error) {
	n, err := e.prov.findNode(e.cluster, node)
	if err != nil {
		return nil, fmt.Errorf("kind: node shell: %w", err)
	}
	return []string{"docker", "exec", "-it", n.String(), "bash"}, nil
}
