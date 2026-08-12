package challenges

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFixCrashloopIsEmbedded(t *testing.T) {
	if !Has("fix-crashloop") {
		t.Fatal("fix-crashloop should be a built-in challenge")
	}
	names := Names()
	if len(names) == 0 {
		t.Fatal("expected at least one built-in challenge")
	}
}

func TestMaterialize(t *testing.T) {
	dest := t.TempDir()
	dir, err := Materialize("fix-crashloop", dest)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	for _, rel := range []string{"challenge.yaml", "setup/01-app.yaml", "solution/solve.sh"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("expected %s materialized: %v", rel, err)
		}
	}
	// solve.sh must be executable.
	info, _ := os.Stat(filepath.Join(dir, "solution", "solve.sh"))
	if info.Mode()&0o100 == 0 {
		t.Fatal("solve.sh should be executable")
	}
	// Idempotent: second call returns same dir, no error.
	if _, err := Materialize("fix-crashloop", dest); err != nil {
		t.Fatalf("second materialize: %v", err)
	}
}

func TestMaterializeUnknown(t *testing.T) {
	if _, err := Materialize("no-such-challenge", t.TempDir()); err == nil {
		t.Fatal("expected error for unknown challenge")
	}
}
