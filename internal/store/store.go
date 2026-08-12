// Package store is the file+flock SessionStore — the sole writer of
// ~/.kubedrill/sessions/<id> (AD-7). Writes are atomic (tmp+rename); each
// session is guarded by both an in-process mutex (serializes goroutines) and a
// file lock (serializes separate kubedrill processes).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// ErrNotFound is returned when a session id does not exist.
var ErrNotFound = errors.New("session not found")

// Store is a filesystem-backed api.SessionStore.
type Store struct {
	root string

	mu    sync.Mutex             // guards locks map
	locks map[string]*sync.Mutex // per-session in-process locks
}

var _ api.SessionStore = (*Store)(nil)

// DefaultDir returns ~/.kubedrill/sessions.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".kubedrill", "sessions"), nil
}

// New builds a store rooted at dir.
func New(dir string) *Store {
	return &Store{root: dir, locks: map[string]*sync.Mutex{}}
}

func (s *Store) sessionDir(id string) string { return filepath.Join(s.root, id) }
func (s *Store) statePath(id string) string  { return filepath.Join(s.sessionDir(id), "state.json") }

// SessionDir returns the on-disk directory for a session.
func (s *Store) SessionDir(id string) string { return s.sessionDir(id) }

// KubeconfigPath returns the player kubeconfig path for a session.
func (s *Store) KubeconfigPath(id string) string {
	return filepath.Join(s.sessionDir(id), "kubeconfig")
}

// idLock returns the in-process mutex for a session id.
func (s *Store) idLock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.locks[id]
	if !ok {
		m = &sync.Mutex{}
		s.locks[id] = m
	}
	return m
}

// Create initializes a session dir and writes its initial state.
func (s *Store) Create(state api.State) error {
	if state.ID == "" {
		return fmt.Errorf("store: session id required")
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = api.StateSchemaVersion
	}
	if state.StartedAt.IsZero() {
		state.StartedAt = time.Now().UTC()
	}
	dir := s.sessionDir(state.ID)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("store: session %q already exists", state.ID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create session dir: %w", err)
	}
	if err := s.writeState(state); err != nil {
		return err
	}
	return nil
}

// Load reads a session's state.
func (s *Store) Load(id string) (api.State, error) {
	b, err := os.ReadFile(s.statePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return api.State{}, fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		return api.State{}, fmt.Errorf("store: read state: %w", err)
	}
	var st api.State
	if err := json.Unmarshal(b, &st); err != nil {
		return api.State{}, fmt.Errorf("store: parse state for %q: %w", id, err)
	}
	return st, nil
}

// List returns all sessions, newest StartedAt first.
func (s *Store) List() ([]api.State, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	var out []api.State
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		st, err := s.Load(e.Name())
		if err != nil {
			continue // skip unreadable/partial dirs
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

// Update mutates a session's state under both locks and persists atomically.
func (s *Store) Update(id string, mutate func(*api.State) error) error {
	m := s.idLock(id)
	m.Lock()
	defer m.Unlock()

	fl := flock.New(filepath.Join(s.sessionDir(id), ".lock"))
	if err := fl.Lock(); err != nil {
		return fmt.Errorf("store: flock %q: %w", id, err)
	}
	defer func() { _ = fl.Unlock() }()

	st, err := s.Load(id)
	if err != nil {
		return err
	}
	if err := mutate(&st); err != nil {
		return err
	}
	return s.writeState(st)
}

// writeState persists state atomically (tmp in the same dir + rename).
func (s *Store) writeState(st api.State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("store: marshal state: %w", err)
	}
	dir := s.sessionDir(st.ID)
	tmp, err := os.CreateTemp(dir, ".state-*.json")
	if err != nil {
		return fmt.Errorf("store: temp state: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("store: write state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("store: close state: %w", err)
	}
	if err := os.Rename(tmpPath, s.statePath(st.ID)); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("store: commit state: %w", err)
	}
	return nil
}

// AppendEvent appends one JSON line to the session's events.log.
func (s *Store) AppendEvent(id string, ev api.Event) error {
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	m := s.idLock(id)
	m.Lock()
	defer m.Unlock()

	f, err := os.OpenFile(filepath.Join(s.sessionDir(id), "events.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("store: open events.log: %w", err)
	}
	defer f.Close()
	line, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("store: marshal event: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("store: append event: %w", err)
	}
	return nil
}

// Remove deletes a session directory.
func (s *Store) Remove(id string) error {
	if err := os.RemoveAll(s.sessionDir(id)); err != nil {
		return fmt.Errorf("store: remove session %q: %w", id, err)
	}
	// If it was current, clear the pointer.
	if cur, _ := s.Current(); cur == id {
		_ = os.Remove(filepath.Join(s.root, "current"))
	}
	return nil
}

// Current returns the current session id, or "" if unset.
func (s *Store) Current() (string, error) {
	b, err := os.ReadFile(filepath.Join(s.root, "current"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("store: read current: %w", err)
	}
	return string(b), nil
}

// SetCurrent points the default session at id (atomic rename).
func (s *Store) SetCurrent(id string) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("store: root dir: %w", err)
	}
	tmp := filepath.Join(s.root, ".current.tmp")
	if err := os.WriteFile(tmp, []byte(id), 0o644); err != nil {
		return fmt.Errorf("store: write current: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(s.root, "current")); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("store: commit current: %w", err)
	}
	return nil
}

// LiveCount returns how many sessions still own a running cluster — every
// phase except "stopped". This is the resource-pressure signal (NFR-4): a
// *verified* or *failed* session's kind cluster keeps running (and consuming
// ~2 GB) until `stop` tears it down, so it must count toward the warning just
// as much as a running one. Counting only creating/running would undercount
// live clusters and let a user pile them up until the Docker VM OOMs.
func (s *Store) LiveCount() int {
	states, err := s.List()
	if err != nil {
		return 0
	}
	n := 0
	for _, st := range states {
		if st.Phase != api.PhaseStopped {
			n++
		}
	}
	return n
}
