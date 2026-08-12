package author_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/agarwalvivek29/kubedrill/internal/author"
	"github.com/agarwalvivek29/kubedrill/internal/challenge"
)

// TestScaffoldLoads is the load-bearing test: a freshly scaffolded challenge
// must pass the full loader (strict decode + semantic + referential + compile),
// so authors start from a known-good baseline and Story 2.2's `author validate`
// is green on an unedited scaffold.
func TestScaffoldLoads(t *testing.T) {
	parent := t.TempDir()
	dir, err := author.Scaffold(parent, "my-drill")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if want := filepath.Join(parent, "my-drill"); dir != want {
		t.Fatalf("returned dir = %q, want %q", dir, want)
	}

	loaded, err := challenge.LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir on scaffold: %v", err)
	}
	if got := loaded.Challenge.Metadata.Name; got != "my-drill" {
		t.Fatalf("scaffolded metadata.name = %q, want %q", got, "my-drill")
	}
}

// TestScaffoldFilesAndModes verifies the advertised layout exists and solve.sh
// is executable (the harness runs it directly).
func TestScaffoldFilesAndModes(t *testing.T) {
	dir, err := author.Scaffold(t.TempDir(), "demo")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	for _, rel := range []string{
		"challenge.yaml",
		"setup/01-app.yaml",
		"solution/SOLUTION.md",
		"solution/solve.sh",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected file %s: %v", rel, err)
		}
	}

	// probes/ is advertised even though the template ships no probe check.
	if fi, err := os.Stat(filepath.Join(dir, "probes")); err != nil || !fi.IsDir() {
		t.Errorf("expected probes/ directory: err=%v", err)
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, "solution", "solve.sh"))
		if err != nil {
			t.Fatalf("stat solve.sh: %v", err)
		}
		if fi.Mode().Perm()&0o111 == 0 {
			t.Errorf("solve.sh mode = %v, want executable", fi.Mode().Perm())
		}
	}
}

func TestScaffoldRefusesExisting(t *testing.T) {
	parent := t.TempDir()
	if _, err := author.Scaffold(parent, "dup"); err != nil {
		t.Fatalf("first Scaffold: %v", err)
	}
	if _, err := author.Scaffold(parent, "dup"); err == nil {
		t.Fatal("second Scaffold into existing dir: got nil error, want refusal")
	}
}

func TestScaffoldRejectsInvalidName(t *testing.T) {
	parent := t.TempDir()
	tooLong := ""
	for i := 0; i < 64; i++ {
		tooLong += "a"
	}
	for _, name := range []string{"", "Bad_Name", "UPPER", "-leading", "trailing-", "has space", tooLong} {
		if _, err := author.Scaffold(parent, name); err == nil {
			t.Errorf("Scaffold(%q): got nil error, want rejection", name)
		}
	}
}
