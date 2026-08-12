package verify

import "testing"

func TestInterpretExit(t *testing.T) {
	cases := []struct {
		code   int32
		reason string
		want   Outcome
	}{
		{0, "", Pass},
		{0, "ignored", Pass},
		{1, "service returned 503", Fail},
		{1, "", Fail},
		{2, "boom", Errored},
		{137, "OOMKilled", Errored},
	}
	for _, c := range cases {
		got := interpretExit(c.code, c.reason)
		if got.Outcome != c.want {
			t.Fatalf("exit %d -> %v, want %v", c.code, got.Outcome, c.want)
		}
		if c.code == 1 && c.reason != "" && got.Reason != c.reason {
			t.Fatalf("exit 1 reason = %q, want %q", got.Reason, c.reason)
		}
	}
}

func TestLastLine(t *testing.T) {
	cases := map[string]string{
		"only line":                      "only line",
		"first\nsecond\nthird":           "third",
		"trailing blank\n\n":             "trailing blank",
		"  spaced  \n  final line  \n":   "final line",
		"":                               "",
	}
	for in, want := range cases {
		if got := lastLine(in); got != want {
			t.Fatalf("lastLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsProbeObjectFilter(t *testing.T) {
	probe := map[string]any{"metadata": map[string]any{"labels": map[string]any{probeLabel: "true"}}}
	normal := map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "api"}}}
	noLabels := map[string]any{"metadata": map[string]any{}}
	if !isProbeObject(probe) {
		t.Fatal("probe-labeled object should be detected")
	}
	if isProbeObject(normal) {
		t.Fatal("app object should not be treated as a probe")
	}
	if isProbeObject(noLabels) {
		t.Fatal("label-less object should not be a probe")
	}
}
