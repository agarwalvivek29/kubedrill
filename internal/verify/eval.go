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

// probeLabel marks probe-created objects; they are excluded from all check
// target resolution so a probe pod can never satisfy or break a check (AD-10).
const probeLabel = "kubedrill.dev/probe"

// Evaluator holds what check evaluation needs: the cluster client and the
// challenge directory (probe scripts are read from there).
type Evaluator struct {
	Client *kube.Client
	Dir    string
}

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

// Check evaluates a single check (match, cel, probe, or anyOf) against the
// live cluster.
func (e *Evaluator) Check(ctx context.Context, ch v1alpha1.Check) CheckResult {
	switch {
	case ch.Match != nil:
		return e.evalMatch(ctx, ch.Match)
	case ch.CEL != "":
		return e.evalCEL(ctx, ch)
	case ch.Probe != nil:
		return e.evalProbe(ctx, ch.Probe)
	case len(ch.AnyOf) > 0:
		return e.evalAnyOf(ctx, ch.AnyOf)
	default:
		return CheckResult{Errored, "check has no assertion"}
	}
}

func (e *Evaluator) evalAnyOf(ctx context.Context, leaves []v1alpha1.Check) CheckResult {
	var lastFail string
	for _, leaf := range leaves {
		r := e.Check(ctx, leaf)
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

func (e *Evaluator) evalMatch(ctx context.Context, m *v1alpha1.MatchCheck) CheckResult {
	obj, res := e.getTarget(ctx, m.Target)
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
func (e *Evaluator) getTarget(ctx context.Context, t v1alpha1.Target) (*unstructuredObj, *CheckResult) {
	apiVersion := t.APIVersion
	if apiVersion == "" {
		apiVersion = kube.DefaultAPIVersion(t.Kind)
	}
	if apiVersion == "" {
		return nil, &CheckResult{Errored, fmt.Sprintf("cannot resolve apiVersion for kind %q; set target.apiVersion", t.Kind)}
	}
	ri, err := e.Client.ResourceFor(apiVersion, t.Kind, t.Namespace)
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

// isProbeObject reports whether an unstructured object is a probe artifact
// (labeled kubedrill.dev/probe), which must be excluded from target resolution.
func isProbeObject(obj map[string]any) bool {
	md, ok := obj["metadata"].(map[string]any)
	if !ok {
		return false
	}
	labels, ok := md["labels"].(map[string]any)
	if !ok {
		return false
	}
	_, marked := labels[probeLabel]
	return marked
}
