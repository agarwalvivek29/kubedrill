package engine

import (
	"context"

	"github.com/agarwalvivek29/kubedrill/internal/challenge"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
	"github.com/agarwalvivek29/kubedrill/internal/verify"
)

// EvaluateDirect grades a session's challenge with every objective evaluated
// directly, ignoring dependsOn gating (AD-11). It reads with the PLAYER identity
// (same as Verify) but records nothing — no attempt, no state change. The
// author-test negative phase uses it to prove no objective already passes on a
// freshly-provisioned, unsolved environment.
func (e *Engine) EvaluateDirect(ctx context.Context, sessionID string) (*verify.Scorecard, error) {
	st, err := e.Store.Load(sessionID)
	if err != nil {
		return nil, err
	}
	loaded, err := challenge.LoadDir(st.ChallengeDir)
	if err != nil {
		return nil, err
	}
	kc, err := readFile(e.Store.KubeconfigPath(sessionID))
	if err != nil {
		return nil, err
	}
	c, err := kube.FromKubeconfig(kc)
	if err != nil {
		return nil, err
	}
	card := verify.EvaluateDirect(ctx, &verify.Evaluator{Client: c, Dir: st.ChallengeDir}, loaded.Challenge)
	return &card, nil
}
