package api

import "time"

// StateSchemaVersion is the on-disk state.json schema version. Bumping it means
// a migration path must handle older sessions.
const StateSchemaVersion = 1

// Phase is a session's lifecycle state.
type Phase string

const (
	PhaseCreating Phase = "creating"
	PhaseRunning  Phase = "running"
	PhaseVerified Phase = "verified"
	PhaseFailed   Phase = "failed"
	PhaseExpired  Phase = "expired"
	PhaseStopped  Phase = "stopped"
)

// State is the single source of truth for one session, persisted atomically as
// state.json. Only a SessionStore writes it (AD-7).
type State struct {
	SchemaVersion int               `json:"schemaVersion"`
	ID            string            `json:"id"`
	Challenge     ChallengeRef      `json:"challenge"`
	Provider      string            `json:"provider"`
	Cluster       string            `json:"cluster,omitempty"`
	Phase         Phase             `json:"phase"`
	StartedAt     time.Time         `json:"startedAt"`
	Deadline      *time.Time        `json:"deadline,omitempty"`
	HintsUsed     []string          `json:"hintsUsed,omitempty"`
	Attempts      []Attempt         `json:"attempts,omitempty"`
	BestScore     int               `json:"bestScore"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// ChallengeRef identifies the challenge a session is playing.
type ChallengeRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Attempt records one verify run's outcome (appended, never mutated).
type Attempt struct {
	N          int             `json:"n"`
	At         time.Time       `json:"at"`
	Score      int             `json:"score"`
	Late       bool            `json:"late,omitempty"`
	Objectives map[string]bool `json:"objectives,omitempty"`
}

// Event is an append-only audit line in events.log.
type Event struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // started, hint, verify, reset, stopped, ...
	Note string    `json:"note,omitempty"`
}

// SessionStore is the sole writer of session state (AD-7). The v0.1
// implementation is file+flock under ~/.kubedrill/sessions; team mode supplies
// a different implementation of this same port without touching callers.
type SessionStore interface {
	// Create initializes a new session directory and its state.json.
	Create(state State) error
	// Load returns a session's current state.
	Load(id string) (State, error)
	// List returns all known sessions, newest first.
	List() ([]State, error)
	// Update applies mutate under the session's lock and atomically persists
	// the result. mutate must be pure w.r.t. side effects other than editing
	// the passed State.
	Update(id string, mutate func(*State) error) error
	// AppendEvent appends one line to the session's events.log.
	AppendEvent(id string, ev Event) error
	// Remove deletes a session's on-disk state (after its cluster is gone).
	Remove(id string) error
	// Current returns the default ("current") session id, or "" if none.
	Current() (string, error)
	// SetCurrent points the default session at id.
	SetCurrent(id string) error
}
