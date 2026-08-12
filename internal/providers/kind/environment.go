package kind

import (
	"fmt"
	"os"
)

// environment is the kind-backed api.Environment. It is kubeconfig-shaped:
// callers get bytes, never a live client (AD-3).
type environment struct {
	id         string
	playerPath string
	enginePath string
	labels     map[string]string
}

func (e *environment) ID() string { return e.id }

func (e *environment) Kubeconfig() ([]byte, error) {
	b, err := os.ReadFile(e.playerPath)
	if err != nil {
		return nil, fmt.Errorf("kind: read player kubeconfig: %w", err)
	}
	return b, nil
}

func (e *environment) EngineKubeconfig() ([]byte, error) {
	b, err := os.ReadFile(e.enginePath)
	if err != nil {
		return nil, fmt.Errorf("kind: read engine kubeconfig: %w", err)
	}
	return b, nil
}

func (e *environment) Labels() map[string]string {
	if e.labels == nil {
		return map[string]string{}
	}
	return e.labels
}
