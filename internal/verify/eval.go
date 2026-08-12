// Package verify evaluates a challenge's checks against a live cluster and
// scores the result. It is a pure function of (Environment, spec) — no CLI
// state (AD-2) — so player verify and author-test share this path.
package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/challenge"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
)

// Outcome is the result of evaluating one check.
type Outcome int

const (
	// Pass: the assertion held.
	Pass Outcome = iota
	// Fail: the assertion did not hold (a legitimate "you're not done yet").
	Fail
	// Errored: the check could not be evaluated (missing tooling, probe crash).
	// Distinct from Fail — never reported as "you failed" (LLD §10).
	Errored
)

// CheckResult is one check's evaluated outcome with a user-facing reason.
type CheckResult struct {
	Outcome Outcome
	Reason  string
}

// EvalCheck evaluates a single non-probe check (match or cel) against the live
// cluster. anyOf is evaluated as OR of its leaves. Probe checks are handled by
// the probe runner (Story 1.9) and reported Errored here.
func EvalCheck(ctx context.Context, c *kube.Client, ch v1alpha1.Check) CheckResult {
	switch {
	case ch.Match != nil:
		return evalMatch(ctx, c, ch.Match)
	case ch.CEL != "":
		return evalCEL(ctx, c, ch)
	case len(ch.AnyOf) > 0:
		return evalAnyOf(ctx, c, ch.AnyOf)
	case ch.Probe != nil:
		return CheckResult{Errored, "probe checks are evaluated by the probe runner (not yet wired)"}
	default:
		return CheckResult{Errored, "check has no assertion"}
	}
}

func evalAnyOf(ctx context.Context, c *kube.Client, leaves []v1alpha1.Check) CheckResult {
	var lastFail string
	for _, leaf := range leaves {
		r := EvalCheck(ctx, c, leaf)
		if r.Outcome == Pass {
			return CheckResult{Pass, ""}
		}
		if r.Outcome == Fail {
			lastFail = r.Reason
		}
	}
	if lastFail == "" {
		lastFail = "no anyOf branch matched"
	}
	return CheckResult{Fail, "anyOf: " + lastFail}
}

func evalMatch(ctx context.Context, c *kube.Client, m *v1alpha1.MatchCheck) CheckResult {
	obj, res := getTarget(ctx, c, m.Target)
	if res != nil {
		return *res
	}
	tree, err := decodeMatchTree(m.Object)
	if err != nil {
		return CheckResult{Errored, err.Error()}
	}
	if challenge.Match(tree, obj.Object) {
		return CheckResult{Pass, ""}
	}
	return CheckResult{Fail, fmt.Sprintf("%s/%s did not match the expected subset", m.Target.Kind, m.Target.Name)}
}

// getTarget fetches a single target object as unstructured. Returns a non-nil
// *CheckResult when the fetch itself determines the outcome (not-found = Fail,
// resolution error = Errored).
func getTarget(ctx context.Context, c *kube.Client, t v1alpha1.Target) (*unstructuredObj, *CheckResult) {
	apiVersion := t.APIVersion
	if apiVersion == "" {
		apiVersion = kube.DefaultAPIVersion(t.Kind)
	}
	if apiVersion == "" {
		return nil, &CheckResult{Errored, fmt.Sprintf("cannot resolve apiVersion for kind %q; set target.apiVersion", t.Kind)}
	}
	ri, err := c.ResourceFor(apiVersion, t.Kind, t.Namespace)
	if err != nil {
		return nil, &CheckResult{Errored, err.Error()}
	}
	u, err := ri.Get(ctx, t.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, &CheckResult{Fail, fmt.Sprintf("%s %q not found", t.Kind, t.Name)}
		}
		return nil, &CheckResult{Errored, fmt.Sprintf("get %s/%s: %v", t.Kind, t.Name, err)}
	}
	return &unstructuredObj{u.Object}, nil
}

type unstructuredObj struct{ Object map[string]any }

func decodeMatchTree(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, errors.New("match.object is empty")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("match.object invalid: %w", err)
	}
	return v, nil
}
