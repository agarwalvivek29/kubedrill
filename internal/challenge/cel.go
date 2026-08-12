package challenge

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// celEnv is the FROZEN CEL environment for v1alpha1 (AD-6). Variables:
//   object  — the single target object (dyn map)
//   objects — a list of target objects (dyn list)
//   now     — evaluation timestamp
// Helpers — exactly two, and no more in v1alpha1:
//   snapshot(kind, namespace, name) — object as captured at session start
//   restarts0(pod, container)       — baseline restartCount from the snapshot
//
// This function builds a compile-only env: it declares the helpers so programs
// referencing them type-check at load. Evaluation bindings are supplied by the
// verify engine (Story 1.8); here we only need Compile to catch syntax/type
// errors before anyone runs the challenge.
func celEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("object", cel.DynType),
		cel.Variable("objects", cel.ListType(cel.DynType)),
		cel.Variable("now", cel.TimestampType),
		cel.Function("snapshot",
			cel.Overload("snapshot_string_string_string",
				[]*cel.Type{cel.StringType, cel.StringType, cel.StringType},
				cel.DynType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					// Compile-time declaration; real binding is set at eval.
					return types.NewErr("snapshot() is only bound during verify")
				}),
			),
		),
		cel.Function("restarts0",
			cel.Overload("restarts0_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType},
				cel.IntType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					return types.NewErr("restarts0() is only bound during verify")
				}),
			),
		),
	)
}

// CompileCEL type-checks a CEL expression against the frozen env. It returns a
// located error if the expression fails to parse or type-check, so lint and
// the loader catch bad CEL before the challenge ever runs.
func CompileCEL(expr, where string) error {
	env, err := celEnv()
	if err != nil {
		return fmt.Errorf("internal: cel env: %w", err)
	}
	_, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return fmt.Errorf("%s: cel does not compile: %w", where, iss.Err())
	}
	return nil
}
