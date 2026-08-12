package engine

import (
	"fmt"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/challenge"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// NextHint returns the next not-yet-revealed hint for a session, and how many
// hints remain (including this one). ok is false when there are no more hints.
func (e *Engine) NextHint(sessionID string) (hint v1alpha1.Hint, remaining int, ok bool, err error) {
	st, err := e.Store.Load(sessionID)
	if err != nil {
		return v1alpha1.Hint{}, 0, false, err
	}
	loaded, err := challenge.LoadDir(st.ChallengeDir)
	if err != nil {
		return v1alpha1.Hint{}, 0, false, err
	}
	used := map[string]bool{}
	for _, id := range st.HintsUsed {
		used[id] = true
	}
	var unused []v1alpha1.Hint
	for _, h := range loaded.Challenge.Hints {
		if !used[h.ID] {
			unused = append(unused, h)
		}
	}
	if len(unused) == 0 {
		return v1alpha1.Hint{}, 0, false, nil
	}
	return unused[0], len(unused), true, nil
}

// RevealHint reveals the next hint and records it as used (applying its penalty
// at the next verify). Revealing is idempotent per hint id: a hint's penalty is
// recorded exactly once.
func (e *Engine) RevealHint(sessionID string) (v1alpha1.Hint, error) {
	h, _, ok, err := e.NextHint(sessionID)
	if err != nil {
		return v1alpha1.Hint{}, err
	}
	if !ok {
		return v1alpha1.Hint{}, fmt.Errorf("no more hints for this challenge")
	}
	if err := e.Store.Update(sessionID, func(s *api.State) error {
		for _, id := range s.HintsUsed {
			if id == h.ID {
				return nil // already recorded; don't double-count
			}
		}
		s.HintsUsed = append(s.HintsUsed, h.ID)
		return nil
	}); err != nil {
		return v1alpha1.Hint{}, err
	}
	_ = e.Store.AppendEvent(sessionID, api.Event{Kind: "hint", Note: fmt.Sprintf("%s (-%d)", h.ID, h.Penalty)})
	return h, nil
}

// hintPenalty sums the penalties of the hints a session has revealed.
func hintPenalty(ch *v1alpha1.Challenge, used []string) int {
	byID := map[string]int{}
	for _, h := range ch.Hints {
		byID[h.ID] = h.Penalty
	}
	total := 0
	for _, id := range used {
		total += byID[id]
	}
	return total
}
