package verify

import (
	"context"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
)

// evalCEL evaluates a CEL check against the live cluster. It binds `object`
// (single target) or `objects` (targets list). The v1alpha1 helpers snapshot()
// and restarts0() are declared for compile-compatibility; delta assertions
// that call them are supported once baselines are wired (Story 1.11/verify
// hardening) — for now a program that invokes them errors rather than lying.
func evalCEL(ctx context.Context, c *kube.Client, ch v1alpha1.Check) CheckResult {
	env, err := celProgramEnv()
	if err != nil {
		return CheckResult{Errored, err.Error()}
	}
	prg, iss := env.Compile(ch.CEL)
	if iss != nil && iss.Err() != nil {
		return CheckResult{Errored, fmt.Sprintf("cel compile: %v", iss.Err())}
	}
	program, err := env.Program(prg)
	if err != nil {
		return CheckResult{Errored, fmt.Sprintf("cel program: %v", err)}
	}

	vars := map[string]any{}
	switch {
	case ch.Target != nil:
		obj, res := getTarget(ctx, c, *ch.Target)
		if res != nil {
			return *res
		}
		vars["object"] = obj.Object
		vars["objects"] = []any{}
	case ch.Targets != nil:
		list, res := listTargets(ctx, c, ch.Targets)
		if res != nil {
			return *res
		}
		vars["objects"] = list
		if len(list) > 0 {
			vars["object"] = list[0]
		} else {
			vars["object"] = map[string]any{}
		}
	default:
		return CheckResult{Errored, "cel check needs a target or targets"}
	}

	out, _, err := program.Eval(vars)
	if err != nil {
		return CheckResult{Errored, fmt.Sprintf("cel eval: %v", err)}
	}
	b, ok := out.Value().(bool)
	if !ok {
		return CheckResult{Errored, "cel expression did not evaluate to a bool"}
	}
	if b {
		return CheckResult{Pass, ""}
	}
	return CheckResult{Fail, "cel expression was false"}
}

func celProgramEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("object", cel.DynType),
		cel.Variable("objects", cel.ListType(cel.DynType)),
		cel.Function("snapshot",
			cel.Overload("snapshot_sss", []*cel.Type{cel.StringType, cel.StringType, cel.StringType}, cel.DynType,
				cel.FunctionBinding(func(...ref.Val) ref.Val { return types.NewErr("snapshot() needs a session baseline (not yet wired)") }))),
		cel.Function("restarts0",
			cel.Overload("restarts0_ds", []*cel.Type{cel.DynType, cel.StringType}, cel.IntType,
				cel.FunctionBinding(func(...ref.Val) ref.Val { return types.NewErr("restarts0() needs a session baseline (not yet wired)") }))),
	)
}

func listTargets(ctx context.Context, c *kube.Client, t *v1alpha1.Targets) ([]any, *CheckResult) {
	apiVersion := t.APIVersion
	if apiVersion == "" {
		apiVersion = kube.DefaultAPIVersion(t.Kind)
	}
	if apiVersion == "" {
		return nil, &CheckResult{Errored, fmt.Sprintf("cannot resolve apiVersion for kind %q", t.Kind)}
	}
	ri, err := c.ResourceFor(apiVersion, t.Kind, t.Namespace)
	if err != nil {
		return nil, &CheckResult{Errored, err.Error()}
	}
	ul, err := ri.List(ctx, metav1.ListOptions{LabelSelector: t.LabelSelector})
	if err != nil {
		return nil, &CheckResult{Errored, fmt.Sprintf("list %s: %v", t.Kind, err)}
	}
	out := make([]any, 0, len(ul.Items))
	for i := range ul.Items {
		out = append(out, ul.Items[i].Object)
	}
	return out, nil
}
