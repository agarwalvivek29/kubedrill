package api

import "context"

// AuditCursor is an opaque resume position in an environment's audit stream.
// Callers persist it between reads and pass it back to AuditEvents to get only
// what is new. It is a byte offset into the control-plane audit log today, but
// callers must treat it as opaque.
type AuditCursor int64

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

	// AuditEvents returns audit-log bytes (newline-delimited audit.k8s.io/v1
	// Event JSON) recorded since `from`, plus a cursor to resume from. It streams
	// incrementally from the control-plane node and never returns the whole log
	// when resumed (AD-3). Returned bytes are always whole lines. When the
	// environment has no audit policy wired (a challenge without rules), it
	// returns no bytes and the same cursor.
	AuditEvents(ctx context.Context, from AuditCursor) (events []byte, next AuditCursor, err error)
}
