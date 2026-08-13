package author

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/engine"
	"github.com/agarwalvivek29/kubedrill/internal/store"
	"github.com/agarwalvivek29/kubedrill/internal/verify"
	"github.com/agarwalvivek29/kubedrill/pkg/api"
)

// ProgressFunc is an optional progress sink for the harness.
type ProgressFunc func(format string, args ...any)

// TestOptions tunes a harness run.
type TestOptions struct {
	// KeepEnv leaves the throwaway cluster running after the run (for debugging
	// a failure). Default is to always tear it down.
	KeepEnv bool
	// SessionID overrides the throwaway session/cluster id (tests set this).
	SessionID string
}

// VacuityViolation names an objective that broke the negative-phase contract
// (AD-11): it passed on a fresh environment when it should have failed, or it
// was marked expectInitiallyPassing but did not pass.
type VacuityViolation struct {
	ObjectiveID string `json:"objective"`
	Title       string `json:"title"`
	Reason      string `json:"reason"`
}

// ObjectiveOutcome is a per-objective result surfaced in the positive/
// idempotency phases.
type ObjectiveOutcome struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Points  int    `json:"points"`
	Passed  bool   `json:"passed"`
	Errored bool   `json:"errored,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// PhaseReport is one phase's verdict.
type PhaseReport struct {
	Name       string             `json:"name"`
	Passed     bool               `json:"passed"`
	Skipped    bool               `json:"skipped,omitempty"`
	Detail     string             `json:"detail,omitempty"`
	Violations []VacuityViolation `json:"violations,omitempty"` // negative phase
	Objectives []ObjectiveOutcome `json:"objectives,omitempty"` // positive/idempotency
}

// TestReport is the full author-test verdict, serializable for `-o json`.
type TestReport struct {
	Dir         string      `json:"dir"`
	Challenge   string      `json:"challenge"`
	Passed      bool        `json:"passed"`
	Negative    PhaseReport `json:"negative"`
	Positive    PhaseReport `json:"positive"`
	Idempotency PhaseReport `json:"idempotency"`
}

// Test runs the author-test correctness harness (AD-11, FR-15) on a throwaway
// cluster provisioned by `provider`:
//
//   - negative:    provision the fresh broken env, then evaluate every objective
//     directly (ignoring dependsOn). Every objective must FAIL — an errored
//     check counts as failing — except objectives marked expectInitiallyPassing,
//     which must PASS. Any violation is vacuity: the challenge could be "solved"
//     without doing the work.
//   - positive:    run the reference solve.sh, then verify — must score 100%.
//     A solve.sh error or an errored check here fails the harness.
//   - idempotency: verify again — must still pass.
//
// The run is fully isolated: it uses a private temp store (so it never touches
// the user's sessions or ~/.kube/config) and always tears the cluster down
// unless opts.KeepEnv. It returns a structured report; a non-nil error means the
// harness could not run (e.g. provisioning failed), distinct from a challenge
// that ran and failed (report.Passed == false).
func Test(ctx context.Context, provider api.EnvProvider, dir string, opts TestOptions, prog ProgressFunc) (*TestReport, error) {
	if prog == nil {
		prog = func(string, ...any) {}
	}
	sessionID := opts.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("at%d", os.Getpid())
	}

	tmp, err := os.MkdirTemp("", "kubedrill-authortest-")
	if err != nil {
		return nil, fmt.Errorf("create temp workspace: %w", err)
	}
	st := store.New(filepath.Join(tmp, "sessions"))
	eng := engine.New(provider, st)

	defer func() {
		if opts.KeepEnv {
			prog("keeping throwaway environment %q for debugging (--keep)\n", sessionID)
			return
		}
		// Use a fresh context so a cancelled/expired ctx still tears down.
		_ = provider.Destroy(context.Background(), sessionID)
		_ = os.RemoveAll(tmp)
	}()

	report := &TestReport{Dir: dir}

	prog("provisioning a throwaway cluster (this can take 30-60s)...\n")
	res, err := eng.Start(ctx, dir, sessionID, func(f string, a ...any) { prog("  "+f+"\n", a...) })
	if err != nil {
		return nil, fmt.Errorf("provision fresh environment: %w", err)
	}
	ch := res.Challenge
	report.Challenge = ch.Metadata.Name

	// ---- Negative phase ----
	prog("negative phase: proving every objective fails on the fresh environment...\n")
	neg, err := eng.EvaluateDirect(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("negative phase evaluation: %w", err)
	}
	report.Negative = negativePhase(ch, *neg)
	if !report.Negative.Passed {
		report.Positive = PhaseReport{Name: "positive", Skipped: true, Detail: "skipped: negative phase failed"}
		report.Idempotency = PhaseReport{Name: "idempotency", Skipped: true, Detail: "skipped: negative phase failed"}
		return report, nil
	}

	// ---- Positive phase ----
	prog("positive phase: applying the reference solution...\n")
	if out, serr := runSolution(ctx, dir, ch.Solution.Script, res.KubeconfigPath); serr != nil {
		report.Positive = PhaseReport{
			Name:   "positive",
			Passed: false,
			Detail: fmt.Sprintf("solve.sh failed: %v\n%s", serr, strings.TrimSpace(out)),
		}
		report.Idempotency = PhaseReport{Name: "idempotency", Skipped: true, Detail: "skipped: positive phase failed"}
		return report, nil
	}
	card, _, err := eng.Verify(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("positive phase verify: %w", err)
	}
	report.Positive = phaseFromCard("positive", card)
	if !report.Positive.Passed {
		report.Idempotency = PhaseReport{Name: "idempotency", Skipped: true, Detail: "skipped: positive phase failed"}
		return report, nil
	}

	// ---- Idempotency phase ----
	prog("idempotency phase: re-verifying the solved environment...\n")
	card2, _, err := eng.Verify(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("idempotency phase verify: %w", err)
	}
	report.Idempotency = phaseFromCard("idempotency", card2)

	report.Passed = report.Negative.Passed && report.Positive.Passed && report.Idempotency.Passed
	return report, nil
}

// negativePhase applies the AD-11 rule to per-objective DIRECT outcomes: every
// objective must fail (errored counts as failing) unless it is marked
// expectInitiallyPassing, in which case it must pass. It returns the phase
// verdict with a violation per offending objective. This is pure — unit-tested
// without a cluster.
func negativePhase(ch *v1alpha1.Challenge, card verify.Scorecard) PhaseReport {
	byID := make(map[string]verify.ObjectiveResult, len(card.Objectives))
	for _, o := range card.Objectives {
		byID[o.ID] = o
	}

	var violations []VacuityViolation
	for _, obj := range ch.Objectives {
		r := byID[obj.ID]
		if obj.ExpectInitiallyPassing {
			if !r.Passed {
				reason := "marked expectInitiallyPassing but did not pass on the fresh environment"
				if r.Reason != "" {
					reason += ": " + r.Reason
				}
				violations = append(violations, VacuityViolation{ObjectiveID: obj.ID, Title: obj.Title, Reason: reason})
			}
			continue
		}
		if r.Passed {
			violations = append(violations, VacuityViolation{
				ObjectiveID: obj.ID,
				Title:       obj.Title,
				Reason:      "passed on the fresh, unsolved environment (vacuous or under-constrained check)",
			})
		}
	}

	return PhaseReport{
		Name:       "negative",
		Passed:     len(violations) == 0,
		Violations: violations,
		Detail:     negativeDetail(len(ch.Objectives), violations),
	}
}

func negativeDetail(n int, violations []VacuityViolation) string {
	if len(violations) == 0 {
		return fmt.Sprintf("all %d objective(s) behaved correctly on the fresh environment", n)
	}
	return fmt.Sprintf("%d of %d objective(s) violated the negative-phase contract", len(violations), n)
}

// phaseFromCard turns a verify scorecard into a positive/idempotency phase
// verdict. The phase passes iff every objective passed (AllPassed); an errored
// check therefore fails the phase, and is surfaced distinctly in the detail.
func phaseFromCard(name string, card *verify.Scorecard) PhaseReport {
	p := PhaseReport{Name: name, Passed: card.AllPassed}
	for _, o := range card.Objectives {
		p.Objectives = append(p.Objectives, ObjectiveOutcome{
			ID: o.ID, Title: o.Title, Points: o.Points,
			Passed: o.Passed, Errored: o.Errored, Reason: o.Reason,
		})
	}
	if card.AllPassed {
		p.Detail = fmt.Sprintf("100%% — %d/%d points across %d objective(s)", card.Score, card.MaxScore, len(card.Objectives))
		return p
	}
	var failed, errored []string
	for _, o := range card.Objectives {
		switch {
		case o.Errored:
			errored = append(errored, o.ID)
		case !o.Passed:
			failed = append(failed, o.ID)
		}
	}
	var parts []string
	if len(errored) > 0 {
		parts = append(parts, "errored: "+strings.Join(errored, ", "))
	}
	if len(failed) > 0 {
		parts = append(parts, "failed: "+strings.Join(failed, ", "))
	}
	p.Detail = fmt.Sprintf("%d/%d points; %s", card.Score, card.MaxScore, strings.Join(parts, "; "))
	return p
}

// runSolution executes the reference solve.sh under the FR-16 contract, exactly
// as a player would run it: on the host via bash, with cwd = the challenge
// directory (so the script's relative paths resolve), KUBECONFIG = the player
// kubeconfig, and the network allowed. A non-zero exit fails the harness.
func runSolution(ctx context.Context, dir, script, kubeconfig string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
