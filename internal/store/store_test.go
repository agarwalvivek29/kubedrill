package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

func newSession(t *testing.T, s *Store, id string) {
	t.Helper()
	if err := s.Create(api.State{ID: id, Phase: api.PhaseRunning, Challenge: api.ChallengeRef{Name: "c", Version: "1.0.0"}}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

func TestCreateLoadRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	newSession(t, s, "abc")
	st, err := s.Load("abc")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if st.SchemaVersion != api.StateSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", st.SchemaVersion, api.StateSchemaVersion)
	}
	if st.StartedAt.IsZero() {
		t.Fatal("StartedAt should be defaulted")
	}
	if st.Phase != api.PhaseRunning {
		t.Fatalf("phase = %q", st.Phase)
	}
}

func TestLoadNotFound(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Load("nope"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestConcurrentUpdateNoLostWrites(t *testing.T) {
	s := New(t.TempDir())
	newSession(t, s, "race")

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Update("race", func(st *api.State) error {
				st.BestScore++ // read-modify-write; must not lose increments
				return nil
			})
			if err != nil {
				t.Errorf("update: %v", err)
			}
		}()
	}
	wg.Wait()

	st, _ := s.Load("race")
	if st.BestScore != n {
		t.Fatalf("BestScore = %d, want %d (lost writes under concurrency)", st.BestScore, n)
	}
	// No leftover temp state files.
	entries, _ := os.ReadDir(s.sessionDir("race"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".state-") {
			t.Fatalf("leftover temp state file: %s", e.Name())
		}
	}
}

func TestStatePersistedIsValidJSON(t *testing.T) {
	s := New(t.TempDir())
	newSession(t, s, "j")
	b, err := os.ReadFile(s.statePath("j"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"schemaVersion"`) {
		t.Fatalf("state.json missing schemaVersion: %s", b)
	}
}

func TestAppendEventAndList(t *testing.T) {
	s := New(t.TempDir())
	newSession(t, s, "e1")
	newSession(t, s, "e2")
	if err := s.AppendEvent("e1", api.Event{Kind: "started"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent("e1", api.Event{Kind: "hint", Note: "h1"}); err != nil {
		t.Fatal(err)
	}
	log, _ := os.ReadFile(filepath.Join(s.sessionDir("e1"), "events.log"))
	if strings.Count(string(log), "\n") != 2 {
		t.Fatalf("expected 2 event lines, got: %s", log)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d sessions, want 2", len(list))
	}
}

func TestCurrentPointer(t *testing.T) {
	s := New(t.TempDir())
	newSession(t, s, "cur")
	if cur, _ := s.Current(); cur != "" {
		t.Fatalf("current should start empty, got %q", cur)
	}
	if err := s.SetCurrent("cur"); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.Current(); cur != "cur" {
		t.Fatalf("current = %q, want cur", cur)
	}
	// Removing the current session clears the pointer.
	if err := s.Remove("cur"); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.Current(); cur != "" {
		t.Fatalf("current should be cleared after remove, got %q", cur)
	}
}

func TestActiveCount(t *testing.T) {
	s := New(t.TempDir())
	newSession(t, s, "a") // running
	newSession(t, s, "b") // running
	_ = s.Update("b", func(st *api.State) error { st.Phase = api.PhaseStopped; return nil })
	if got := s.ActiveCount(); got != 1 {
		t.Fatalf("ActiveCount = %d, want 1 (one running, one stopped)", got)
	}
}
