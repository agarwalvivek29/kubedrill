package kind

import (
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

const playerKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: kind
  cluster:
    server: https://127.0.0.1:54321
    certificate-authority-data: UExBWUVSQ0E=
contexts:
- name: kind
  context:
    cluster: kind
    user: kubernetes-admin
current-context: kind
users:
- name: kubernetes-admin
  user:
    client-certificate-data: UExBWUVSQ0VSVA==
    client-key-data: UExBWUVSS0VZ
`

// minted mimics `kubeadm kubeconfig user` output: correct credential, but an
// in-cluster server address unreachable from the host.
const mintedKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: kubernetes
  cluster:
    server: https://kubedrill-x-control-plane:6443
    certificate-authority-data: SU5URVJOQUxDQQ==
contexts:
- name: kubedrill:engine@kubernetes
  context:
    cluster: kubernetes
    user: kubedrill:engine
current-context: kubedrill:engine@kubernetes
users:
- name: kubedrill:engine
  user:
    client-certificate-data: RU5HSU5FQ0VSVA==
    client-key-data: RU5HSU5FS0VZ
`

func TestSplicedEngineKubeconfig(t *testing.T) {
	out, err := splicedEngineKubeconfig([]byte(playerKubeconfig), []byte(mintedKubeconfig))
	if err != nil {
		t.Fatalf("splice failed: %v", err)
	}
	cfg, err := clientcmd.Load(out)
	if err != nil {
		t.Fatalf("output not a valid kubeconfig: %v", err)
	}

	// Server + CA must come from the PLAYER (host-reachable).
	var server string
	for _, c := range cfg.Clusters {
		server = c.Server
	}
	if server != "https://127.0.0.1:54321" {
		t.Fatalf("engine kubeconfig server = %q, want the player's host-reachable address", server)
	}

	// Credential must be the ENGINE identity, not the player's.
	auth := singleAuthInfo(cfg)
	if auth == nil {
		t.Fatal("no auth info in spliced kubeconfig")
	}
	if got := string(auth.ClientCertificateData); got != "ENGINECERT" {
		t.Fatalf("client cert = %q, want the engine cert (ENGINECERT)", got)
	}

	// The active context must point at the engine credential.
	kctx := cfg.Contexts[cfg.CurrentContext]
	if kctx == nil {
		kctx = firstContext(cfg)
	}
	if kctx == nil || kctx.AuthInfo != "kubedrill-engine" {
		t.Fatalf("context does not use the engine credential: %+v", kctx)
	}
}

func TestSplicedEngineKubeconfigRejectsBadInput(t *testing.T) {
	if _, err := splicedEngineKubeconfig([]byte("not yaml: ["), []byte(mintedKubeconfig)); err == nil {
		t.Fatal("expected error on malformed player kubeconfig")
	}
}
