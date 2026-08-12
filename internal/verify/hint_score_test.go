package verify

import "testing"

func TestNetScoreFloorsAtZero(t *testing.T) {
	cases := []struct {
		score, penalty, want int
	}{
		{100, 0, 100},
		{100, 30, 70},
		{50, 60, 0},   // penalty exceeds score → floored, not negative
		{0, 10, 0},
	}
	for _, c := range cases {
		card := Scorecard{Score: c.score, HintPenalty: c.penalty}
		if got := card.NetScore(); got != c.want {
			t.Fatalf("NetScore(score=%d,penalty=%d) = %d, want %d", c.score, c.penalty, got, c.want)
		}
	}
}
