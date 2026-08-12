package api

// Environment is a provisioned cluster a session runs in. It is deliberately
// kubeconfig-shaped, not client-go-shaped (AD-3): all cluster access goes
// through these methods, so the same interface serves a local kind cluster
// today and a scoped, remotely-hosted cluster in team mode later.
//
// The v0.1/Epic-1 surface is below. AuditEvents (audit-log streaming) and
// NodeExec (node-level commands) are added to this interface in Epic 3, where
// they are first used; adding them pre-v1.0 is allowed (no semver commitment
// until v1.0, AD-12).
type Environment interface {
	// ID is the session/environment id.
	ID() string

	// Kubeconfig returns the PLAYER kubeconfig bytes: cluster-admin, for exam
	// realism. This is what `start` prints and `shell`/`env` expose.
	Kubeconfig() ([]byte, error)

	// EngineKubeconfig returns the ENGINE kubeconfig bytes: a distinct identity
	// (group kubedrill:engine) used for setup/faults/reset/probes. It is never
	// surfaced to player-facing commands, so grading can attribute actions
	// correctly (AD-4).
	EngineKubeconfig() ([]byte, error)

	// Labels are the provider-native labels attached at provision time.
	Labels() map[string]string
}
