// Package engine is the hexagonal core (AD-1): it orchestrates the session
// lifecycle over the pkg/api ports and the internal adapters, holding no
// package-global state. The CLI is a thin caller (AD-8).
package engine

import (
	"context"
	"fmt"
	"time"

	"path/filepath"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/challenge"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
	"github.com/agarwalvivek29/kubedrill/internal/provision"
	"github.com/agarwalvivek29/kubedrill/internal/rules"
	"github.com/agarwalvivek29/kubedrill/internal/store"
	"github.com/agarwalvivek29/kubedrill/internal/verify"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// Engine wires the injected ports. No globals (AD-8/NFR-6): constructed by the
// CLI and reusable as a library (the precondition for hosted mode).
type Engine struct {
	Provider api.EnvProvider
	Store    *store.Store
}

// New constructs an Engine.
func New(provider api.EnvProvider, st *store.Store) *Engine {
	return &Engine{Provider: provider, Store: st}
}

// Progressf is an optional progress sink so `start` can show per-phase output.
type Progressf func(format string, args ...any)

// StartResult is what a caller needs after a successful start.
type StartResult struct {
	SessionID      string
	KubeconfigPath string
	Deadline       *time.Time
	Challenge      *v1alpha1.Challenge
}

// Start provisions a cluster for a challenge, applies setup + faults, waits
// for readiness, and records a running session. It is the orchestration behind
// `kubedrill start`.
func (e *Engine) Start(ctx context.Context, dir, sessionID string, prog Progressf) (*StartResult, error) {
	if prog == nil {
		prog = func(string, ...any) {}
	}
	loaded, err := challenge.LoadDir(dir)
	if err != nil {
		return nil, err
	}
	ch := loaded.Challenge

	// Capability gate (AD-5): a challenge with rules needs audit; refuse early
	// rather than run ungraded. (Rules land in Epic 3; harmless now.)
	if len(ch.Rules) > 0 && !e.Provider.Capabilities().AuditLog {
		return nil, &CapabilityError{Need: "auditLog"}
	}

	// Record the session as "creating" BEFORE touching infra: the provider
	// writes the kubeconfig into the session dir, so that dir + state.json
	// must exist first, and a crashed provision then leaves a prunable record.
	deadline := computeDeadline(ch.Metadata.TimeLimit)
	absDir, _ := filepath.Abs(dir)
	st := api.State{
		ID:           sessionID,
		Challenge:    api.ChallengeRef{Name: ch.Metadata.Name, Version: ch.Metadata.Version},
		ChallengeDir: absDir,
		Provider:     e.Provider.Name(),
		Cluster:      "kubedrill-" + sessionID,
		Phase:        api.PhaseCreating,
		Deadline:     deadline,
	}
	if err := e.Store.Create(st); err != nil {
		return nil, err
	}
	_ = e.Store.SetCurrent(sessionID)
	_ = e.Store.AppendEvent(sessionID, api.Event{Kind: "created"})

	prog("creating cluster (this can take 30-60s)...")
	env, err := e.Provider.Provision(ctx, api.EnvRequest{
		SessionID:         sessionID,
		SessionDir:        e.Store.SessionDir(sessionID),
		KubernetesVersion: ch.Environment.Cluster.KubernetesVersion,
		// A ruled challenge gets an audit policy wired into the apiserver so its
		// rules can be graded from what actually happened (AD-5). Unruled
		// challenges pay no audit cost (empty policy → no wiring).
		AuditPolicy: rules.AuditPolicy(ch),
	})
	if err != nil {
		return e.failStart(ctx, sessionID, err)
	}

	// Apply setup + faults + readiness using the ENGINE identity.
	engineKC, err := env.EngineKubeconfig()
	if err != nil {
		return e.failStart(ctx, sessionID, err)
	}
	c, err := kube.FromKubeconfig(engineKC)
	if err != nil {
		return e.failStart(ctx, sessionID, err)
	}
	prog("applying setup and injecting faults...")
	if err := provision.Apply(ctx, c, dir, ch); err != nil {
		return e.failStart(ctx, sessionID, err)
	}

	// Broken as intended: arm the timer.
	if err := e.Store.Update(sessionID, func(s *api.State) error {
		s.Phase = api.PhaseRunning
		if deadline != nil {
			d := time.Now().Add(*durationOrZero(ch.Metadata.TimeLimit))
			s.Deadline = &d
		}
		return nil
	}); err != nil {
		return nil, err
	}
	_ = e.Store.AppendEvent(sessionID, api.Event{Kind: "started"})

	res := &StartResult{
		SessionID:      sessionID,
		KubeconfigPath: e.Store.KubeconfigPath(sessionID),
		Challenge:      ch,
	}
	if st, err := e.Store.Load(sessionID); err == nil {
		res.Deadline = st.Deadline
	}
	return res, nil
}

func (e *Engine) failStart(ctx context.Context, sessionID string, cause error) (*StartResult, error) {
	_ = e.Store.Update(sessionID, func(s *api.State) error { s.Phase = api.PhaseFailed; return nil })
	_ = e.Provider.Destroy(ctx, sessionID)
	_ = e.Store.Remove(sessionID)
	return nil, fmt.Errorf("start failed, cluster torn down: %w", cause)
}

// Verify evaluates the session's challenge against the live cluster, records
// an attempt, and returns the scorecard. Idempotent and side-effect-free on
// the cluster (AD-2/AD-10). It re-reads the frozen challenge copy in the
// session dir so mid-run edits can't change grading.
func (e *Engine) Verify(ctx context.Context, sessionID string) (*verify.Scorecard, bool, error) {
	st, err := e.Store.Load(sessionID)
	if err != nil {
		return nil, false, err
	}
	loaded, err := challenge.LoadDir(st.ChallengeDir)
	if err != nil {
		return nil, false, err
	}
	// Player identity for verify reads.
	kc, err := readFile(e.Store.KubeconfigPath(sessionID))
	if err != nil {
		return nil, false, err
	}
	c, err := kube.FromKubeconfig(kc)
	if err != nil {
		return nil, false, err
	}

	card := verify.Evaluate(ctx, &verify.Evaluator{Client: c, Dir: st.ChallengeDir}, loaded.Challenge)
	// Layer in the session's hint penalties (kept out of the pure evaluator).
	card.HintPenalty = hintPenalty(loaded.Challenge, st.HintsUsed)
	// Layer in rule grading from the audit log (Epic 3): charged violations,
	// their point/fail penalties, and the evidence behind them.
	if len(loaded.Challenge.Rules) > 0 {
		e.gradeRules(ctx, sessionID, e.Store.SessionDir(sessionID), loaded.Challenge, &card)
	}
	net := card.NetScore()

	late := st.Deadline != nil && time.Now().After(*st.Deadline)
	// Persist the attempt; bestScore is monotone.
	if err := e.Store.Update(sessionID, func(s *api.State) error {
		n := len(s.Attempts) + 1
		objs := map[string]bool{}
		for _, o := range card.Objectives {
			objs[o.ID] = o.Passed
		}
		s.Attempts = append(s.Attempts, api.Attempt{N: n, At: time.Now().UTC(), Score: net, Late: late, Objectives: objs})
		if !late && net > s.BestScore {
			s.BestScore = net
		}
		if card.AllPassed {
			s.Phase = api.PhaseVerified
		}
		return nil
	}); err != nil {
		return nil, late, err
	}
	_ = e.Store.AppendEvent(sessionID, api.Event{Kind: "verify", Note: fmt.Sprintf("net=%d/%d penalty=%d late=%v", net, card.MaxScore, card.HintPenalty, late)})
	return &card, late, nil
}

// CapabilityError signals a provider lacks a capability the challenge needs.
type CapabilityError struct{ Need string }

func (e *CapabilityError) Error() string {
	return fmt.Sprintf("provider lacks capability %q required by this challenge; re-run with --rules=advisory to grade from logs only", e.Need)
}

func computeDeadline(timeLimit string) *time.Time {
	d := durationOrZero(timeLimit)
	if d == nil {
		return nil
	}
	t := time.Now().Add(*d)
	return &t
}

func durationOrZero(s string) *time.Duration {
	if s == "" {
		return nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return nil
	}
	return &d
}
