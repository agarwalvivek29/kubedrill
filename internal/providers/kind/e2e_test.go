//go:build e2e

// Package kind e2e tests provision a real kind cluster and require Docker.
// Run with: go test -tags e2e -timeout 15m ./internal/providers/kind/...
// They are excluded from the default (unit) build so normal CI stays fast.
package kind

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

func TestProvisionDestroyE2E(t *testing.T) {
	// Capture the user's ~/.kube/config so we can prove we never touch it.
	home, _ := os.UserHomeDir()
	userKubeconfig := filepath.Join(home, ".kube", "config")
	before, _ := os.ReadFile(userKubeconfig) // may be empty; that's fine

	p := New()
	ctx := context.Background()
	dir := t.TempDir()
	req := api.EnvRequest{SessionID: "e2etest", SessionDir: dir}

	env, err := p.Provision(ctx, req)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	t.Cleanup(func() { _ = p.Destroy(ctx, req.SessionID) })

	// NFR-2: ~/.kube/config is byte-for-byte unchanged.
	after, _ := os.ReadFile(userKubeconfig)
	if string(before) != string(after) {
		t.Fatalf("~/.kube/config was modified during provision — NFR-2 violation")
	}

	// AD-9: player kubeconfig lives under the session dir.
	if _, err := os.Stat(filepath.Join(dir, "kubeconfig")); err != nil {
		t.Fatalf("player kubeconfig not written to session dir: %v", err)
	}

	// AD-4: player and engine kubeconfigs carry DISTINCT identities.
	playerBytes, err := env.Kubeconfig()
	if err != nil {
		t.Fatalf("player kubeconfig: %v", err)
	}
	engineBytes, err := env.EngineKubeconfig()
	if err != nil {
		t.Fatalf("engine kubeconfig: %v", err)
	}
	if identityCert(t, playerBytes) == identityCert(t, engineBytes) {
		t.Fatal("player and engine kubeconfigs share a credential — AD-4 requires distinct identities")
	}

	// List discovers the cluster by name prefix.
	infos, err := p.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, i := range infos {
		if i.ID == req.SessionID {
			found = true
		}
	}
	if !found {
		t.Fatal("provisioned cluster not discovered by List")
	}

	// Destroy removes it.
	if err := p.Destroy(ctx, req.SessionID); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	infos, _ = p.List(ctx)
	for _, i := range infos {
		if i.ID == req.SessionID {
			t.Fatal("cluster still present after destroy")
		}
	}
}

func identityCert(t *testing.T, kubeconfig []byte) string {
	t.Helper()
	cfg, err := clientcmd.Load(kubeconfig)
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	auth := singleAuthInfo(cfg)
	if auth == nil {
		t.Fatal("kubeconfig has no credential")
	}
	return strings.TrimSpace(string(auth.ClientCertificateData))
}
