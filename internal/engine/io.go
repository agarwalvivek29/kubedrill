package engine

import (
	"os"
	"path/filepath"
)

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

// engineKubeconfigPath is where the provider wrote the engine identity for a
// session (never surfaced to player-facing commands).
func (e *Engine) engineKubeconfigPath(sessionID string) string {
	return filepath.Join(e.Store.SessionDir(sessionID), "engine-kubeconfig")
}
