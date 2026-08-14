// Package challenges embeds the built-in reference challenges so kubedrill
// ships playable content in the binary (go:embed). A named challenge is
// materialized to a real directory on demand, because challenge setup
// manifests and probe scripts are referenced by relative path and must exist
// on disk when a session runs.
package challenges

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:fix-crashloop all:fix-rbac all:pending-scheduling
//go:embed all:fix-readiness-probe all:fix-configmap-key all:fix-service-selector
//go:embed all:payments-hotfix all:guarded-config all:node-recon
//go:embed all:fix-volume-claim all:fix-storageclass
var content embed.FS

// Has reports whether name is a built-in challenge.
func Has(name string) bool {
	info, err := fs.Stat(content, name)
	return err == nil && info.IsDir()
}

// Names lists the built-in challenge names.
func Names() []string {
	entries, err := content.ReadDir(".")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// Materialize copies the embedded challenge `name` into destDir/name and
// returns the resulting directory. It is idempotent: an existing, non-empty
// destination is reused.
func Materialize(name, destDir string) (string, error) {
	if !Has(name) {
		return "", fmt.Errorf("no built-in challenge %q (have: %s)", name, strings.Join(Names(), ", "))
	}
	target := filepath.Join(destDir, name)
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
		return target, nil
	}
	err := fs.WalkDir(content, name, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out := filepath.Join(destDir, path)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := content.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		return os.WriteFile(out, b, mode)
	})
	if err != nil {
		return "", fmt.Errorf("materialize %q: %w", name, err)
	}
	return target, nil
}
