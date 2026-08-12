package engine

import (
	"testing"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
)

func TestHintPenaltySumsRevealedOnly(t *testing.T) {
	ch := &v1alpha1.Challenge{Hints: []v1alpha1.Hint{
		{ID: "h1", Penalty: 5},
		{ID: "h2", Penalty: 10},
		{ID: "h3", Penalty: 15},
	}}
	if got := hintPenalty(ch, nil); got != 0 {
		t.Fatalf("no hints used → penalty %d, want 0", got)
	}
	if got := hintPenalty(ch, []string{"h1", "h3"}); got != 20 {
		t.Fatalf("h1+h3 → penalty %d, want 20", got)
	}
	// Unknown ids contribute nothing (defensive).
	if got := hintPenalty(ch, []string{"h1", "ghost"}); got != 5 {
		t.Fatalf("h1+ghost → penalty %d, want 5", got)
	}
}
