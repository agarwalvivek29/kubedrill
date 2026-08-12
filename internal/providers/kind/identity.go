package kind

import (
	"bytes"
	"fmt"
	"os"

	"sigs.k8s.io/kind/pkg/exec"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// nodeCommander is the slice of sigs.k8s.io/kind's node interface we use:
// running a command on the node. Kept minimal so it is easy to fake in tests.
type nodeCommander interface {
	Command(command string, args ...string) exec.Cmd
	String() string
}

// mintEngineKubeconfig creates the distinct engine identity (AD-4).
//
// kubeadm on the control-plane node signs a fresh client cert for
// CN=kubedrill:engine with org system:masters (so the engine can perform
// setup/reset). kubeadm emits a full kubeconfig, but its server address points
// at the in-cluster advertise address, unreachable from the host. So we keep
// only the engine's client credential and splice it onto the PLAYER
// kubeconfig's cluster (server + CA), producing a host-reachable engine
// kubeconfig with a distinguishable identity.
func (p *Provider) mintEngineKubeconfig(clusterName, playerPath, enginePath string) error {
	cp, err := p.controlPlane(clusterName)
	if err != nil {
		return err
	}

	var stdout, stderr bytes.Buffer
	cmd := cp.Command("kubeadm", "kubeconfig", "user",
		"--client-name", "kubedrill:engine",
		"--org", "system:masters")
	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubeadm kubeconfig user: %w (%s)", err, stderr.String())
	}

	playerBytes, err := os.ReadFile(playerPath)
	if err != nil {
		return fmt.Errorf("read player kubeconfig: %w", err)
	}
	out, err := splicedEngineKubeconfig(playerBytes, stdout.Bytes())
	if err != nil {
		return err
	}
	if err := os.WriteFile(enginePath, out, 0o600); err != nil {
		return fmt.Errorf("write engine kubeconfig: %w", err)
	}
	return nil
}

// splicedEngineKubeconfig builds an engine kubeconfig from raw player + minted
// bytes without touching the filesystem — the pure core of mintEngineKubeconfig,
// unit-tested in identity_test.go.
func splicedEngineKubeconfig(playerBytes, mintedBytes []byte) ([]byte, error) {
	minted, err := clientcmd.Load(mintedBytes)
	if err != nil {
		return nil, fmt.Errorf("parse minted kubeconfig: %w", err)
	}
	auth := singleAuthInfo(minted)
	if auth == nil {
		return nil, fmt.Errorf("minted kubeconfig has no user credential")
	}
	base, err := clientcmd.Load(playerBytes)
	if err != nil {
		return nil, fmt.Errorf("parse player kubeconfig: %w", err)
	}
	const engineUser = "kubedrill-engine"
	base.AuthInfos = map[string]*clientcmdapi.AuthInfo{engineUser: auth}
	for _, kctx := range base.Contexts {
		kctx.AuthInfo = engineUser
	}
	return clientcmd.Write(*base)
}

// singleAuthInfo returns the sole AuthInfo from a kubeconfig, or nil.
func singleAuthInfo(cfg *clientcmdapi.Config) *clientcmdapi.AuthInfo {
	for _, ai := range cfg.AuthInfos {
		return ai
	}
	return nil
}

// firstContext returns any context from a kubeconfig, or nil.
func firstContext(cfg *clientcmdapi.Config) *clientcmdapi.Context {
	for _, c := range cfg.Contexts {
		return c
	}
	return nil
}
