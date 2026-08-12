package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/agarwalvivek29/kubedrill/internal/challenge"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
	"github.com/agarwalvivek29/kubedrill/internal/provision"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// ResetOpts controls a reset.
type ResetOpts struct {
	Hard  bool // recreate the whole cluster
	Clock bool // restart the timer from now
	Hints bool // clear revealed hints (practice mode)
}

// Reset restores a session's challenge to its broken state. A fast reset works
// in place; if readiness can't be re-established it auto-falls back to a hard
// reset (recreate the cluster). `didHard` reports which path actually ran.
func (e *Engine) Reset(ctx context.Context, sessionID string, opts ResetOpts, prog Progressf) (didHard bool, err error) {
	if prog == nil {
		prog = func(string, ...any) {}
	}
	st, err := e.Store.Load(sessionID)
	if err != nil {
		return false, err
	}
	loaded, err := challenge.LoadDir(st.ChallengeDir)
	if err != nil {
		return false, err
	}
	ch := loaded.Challenge

	if opts.Hard {
		if err := e.hardReset(ctx, sessionID, st.ChallengeDir, prog); err != nil {
			return true, err
		}
		didHard = true
	} else {
		engineKC, err := readFile(e.engineKubeconfigPath(sessionID))
		if err != nil {
			return false, err
		}
		c, err := kube.FromKubeconfig(engineKC)
		if err != nil {
			return false, err
		}
		prog("resetting challenge state...")
		if rerr := provision.Reset(ctx, c, st.ChallengeDir, ch); rerr != nil {
			prog("fast reset couldn't restore the environment (%v); recreating the cluster...", rerr)
			if herr := e.hardReset(ctx, sessionID, st.ChallengeDir, prog); herr != nil {
				return true, herr
			}
			didHard = true
		}
	}

	// Update session state: back to running; optional clock/hints reset.
	if err := e.Store.Update(sessionID, func(s *api.State) error {
		s.Phase = api.PhaseRunning
		if opts.Clock && ch.Metadata.TimeLimit != "" {
			if d := durationOrZero(ch.Metadata.TimeLimit); d != nil {
				t := time.Now().Add(*d)
				s.Deadline = &t
			}
		}
		if opts.Hints {
			s.HintsUsed = nil
		}
		return nil
	}); err != nil {
		return didHard, err
	}
	_ = e.Store.AppendEvent(sessionID, api.Event{Kind: "reset", Note: fmt.Sprintf("hard=%v clock=%v hints=%v", didHard, opts.Clock, opts.Hints)})
	return didHard, nil
}

// hardReset destroys and recreates the cluster, then re-provisions.
func (e *Engine) hardReset(ctx context.Context, sessionID, dir string, prog Progressf) error {
	if err := e.Provider.Destroy(ctx, sessionID); err != nil {
		return fmt.Errorf("hard reset: destroy: %w", err)
	}
	loaded, err := challenge.LoadDir(dir)
	if err != nil {
		return err
	}
	prog("recreating cluster...")
	env, err := e.Provider.Provision(ctx, api.EnvRequest{
		SessionID:         sessionID,
		SessionDir:        e.Store.SessionDir(sessionID),
		KubernetesVersion: loaded.Challenge.Environment.Cluster.KubernetesVersion,
	})
	if err != nil {
		return fmt.Errorf("hard reset: provision: %w", err)
	}
	engineKC, err := env.EngineKubeconfig()
	if err != nil {
		return err
	}
	c, err := kube.FromKubeconfig(engineKC)
	if err != nil {
		return err
	}
	prog("re-applying setup and faults...")
	return provision.Apply(ctx, c, dir, loaded.Challenge)
}
