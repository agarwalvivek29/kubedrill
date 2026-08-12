package cli

import (
	"context"
	"sort"
	"testing"

	"github.com/agarwalvivek29/kubedrill/internal/store"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// fakeProvider records Destroy calls and reports a fixed cluster list.
type fakeProvider struct {
	clusters  []string
	destroyed []string
}

func (f *fakeProvider) Name() string               { return "fake" }
func (f *fakeProvider) Capabilities() api.Capabilities { return api.Capabilities{} }
func (f *fakeProvider) Provision(context.Context, api.EnvRequest) (api.Environment, error) {
	return nil, nil
}
func (f *fakeProvider) Destroy(_ context.Context, id string) error {
	f.destroyed = append(f.destroyed, id)
	return nil
}
func (f *fakeProvider) List(context.Context) ([]api.EnvInfo, error) {
	var out []api.EnvInfo
	for _, c := range f.clusters {
		out = append(out, api.EnvInfo{ID: c, Name: "kubedrill-" + c})
	}
	return out, nil
}
func (f *fakeProvider) LoadImages(context.Context, string, []string) error { return nil }

func TestPruneReconciliation(t *testing.T) {
	s := store.New(t.TempDir())
	// Session "live" has a cluster; session "stale" does not.
	if err := s.Create(api.State{ID: "live", Phase: api.PhaseRunning}); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(api.State{ID: "stale", Phase: api.PhaseRunning}); err != nil {
		t.Fatal(err)
	}
	// Provider reports "live" (matches a session) and "orphan" (no session).
	prov := &fakeProvider{clusters: []string{"live", "orphan"}}

	lines := pruneOrphans(context.Background(), prov, s)

	// Orphan cluster destroyed.
	if len(prov.destroyed) != 1 || prov.destroyed[0] != "orphan" {
		t.Fatalf("expected orphan cluster destroyed, got %v", prov.destroyed)
	}
	// Stale session (no cluster) removed; live session kept.
	if _, err := s.Load("stale"); err == nil {
		t.Fatal("stale session should have been removed")
	}
	if _, err := s.Load("live"); err != nil {
		t.Fatal("live session should be kept")
	}
	// Two actions reported.
	if len(lines) != 2 {
		sort.Strings(lines)
		t.Fatalf("expected 2 prune actions, got %v", lines)
	}
}
