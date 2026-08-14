package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agarwalvivek29/kubedrill/internal/store"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

func TestSessionExportJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Seed a finished session in the real (temp-HOME) store.
	s := store.New(filepath.Join(home, ".kubedrill", "sessions"))
	st := api.State{
		ID:        "sess1",
		Challenge: api.ChallengeRef{Name: "fix-crashloop", Version: "1.1.0"},
		Phase:     api.PhaseVerified,
		BestScore: 50,
	}
	if err := s.Create(st); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.Update("sess1", func(st *api.State) error {
		st.Attempts = append(st.Attempts, api.Attempt{
			N: 1, At: time.Now().UTC(), Score: 50,
			Objectives: map[string]bool{"deployment-available": true, "service-serves": false},
		})
		st.BestScore = 50
		return nil
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	_ = s.SetCurrent("sess1")

	out, err := runCLI(t, "session", "export", "sess1")
	if err != nil {
		t.Fatalf("session export: %v\n%s", err, out)
	}
	var res sessionResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("session export json: %v\n%s", err, out)
	}
	if res.Challenge != "fix-crashloop" || res.BestScore != 50 || res.AttemptCount != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.FinalObjectives["deployment-available"] != true || res.FinalObjectives["service-serves"] != false {
		t.Fatalf("final objectives wrong: %+v", res.FinalObjectives)
	}

	// -o writes a file.
	f := filepath.Join(t.TempDir(), "result.json")
	if _, err := runCLI(t, "session", "export", "sess1", "-o", f); err != nil {
		t.Fatalf("session export -o: %v", err)
	}
	if b, err := os.ReadFile(f); err != nil || len(b) == 0 {
		t.Fatalf("result file not written: %v", err)
	}
}
