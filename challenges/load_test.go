package challenges_test

import (
	"path/filepath"
	"testing"

	"github.com/agarwalvivek29/kubedrill/challenges"
	"github.com/agarwalvivek29/kubedrill/internal/challenge"
)

// TestAllBuiltinsLoad materializes every built-in challenge and runs it through
// the strict loader (schema + referential + compile checks). This is a
// lightweight content gate until the full author-test harness lands (Epic 2):
// a shipped challenge must at least be valid and loadable.
func TestAllBuiltinsLoad(t *testing.T) {
	names := challenges.Names()
	if len(names) < 3 {
		t.Fatalf("expected >=3 built-in challenges, got %v", names)
	}
	for _, n := range names {
		t.Run(n, func(t *testing.T) {
			dir, err := challenges.Materialize(n, filepath.Join(t.TempDir(), "c"))
			if err != nil {
				t.Fatalf("materialize %s: %v", n, err)
			}
			if _, err := challenge.LoadDir(dir); err != nil {
				t.Fatalf("built-in challenge %s does not load: %v", n, err)
			}
		})
	}
}
