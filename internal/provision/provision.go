// Package provision applies a challenge's setup manifests, injects its faults,
// and gates on readiness so the timer only starts once the environment is
// broken as intended.
package provision

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
	"github.com/agarwalvivek29/kubedrill/internal/verify"
)

// fieldManager identifies kubedrill's server-side-apply writes.
const fieldManager = "kubedrill"

// challengeLabel marks every object kubedrill applies, so reset can select and
// delete exactly what setup created (AD-9 / Story 1.11).
const challengeLabel = "kubedrill.dev/challenge"

// Apply runs the full setup: manifests (SSA), then faults, then waits for the
// readiness gates. All work uses the engine client (c) — attributed to the
// engine identity, not the player (AD-4).
func Apply(ctx context.Context, c *kube.Client, dir string, ch *v1alpha1.Challenge) error {
	for _, m := range ch.Environment.Setup.Manifests {
		if err := applyManifest(ctx, c, filepath.Join(dir, m.Path), ch.Metadata.Name); err != nil {
			return fmt.Errorf("apply %s: %w", m.Path, err)
		}
	}
	for _, f := range ch.Environment.Setup.Faults {
		if err := injectFault(ctx, c, f); err != nil {
			return fmt.Errorf("fault %q: %w", f.Name, err)
		}
	}
	if err := waitReadiness(ctx, c, dir, ch); err != nil {
		return err
	}
	return nil
}

// applyManifest server-side-applies every YAML doc in a file, stamping the
// challenge label so teardown can find it later.
func applyManifest(ctx context.Context, c *kube.Client, path, challengeName string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, doc := range splitYAML(raw) {
		if len(doc) == 0 {
			continue
		}
		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(doc, &obj.Object); err != nil {
			return fmt.Errorf("parse doc: %w", err)
		}
		if len(obj.Object) == 0 {
			continue
		}
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[challengeLabel] = challengeName
		obj.SetLabels(labels)

		ri, err := c.ResourceFor(obj.GetAPIVersion(), obj.GetKind(), obj.GetNamespace())
		if err != nil {
			return err
		}
		data, err := obj.MarshalJSON()
		if err != nil {
			return err
		}
		if _, err := ri.Patch(ctx, obj.GetName(), types.ApplyPatchType, data,
			metav1.PatchOptions{FieldManager: fieldManager, Force: ptrBool(true)}); err != nil {
			return fmt.Errorf("server-side apply %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

// injectFault mutates a live object into its broken state. Only patch faults
// are handled here; exec/nodeExec faults are wired in later stories.
func injectFault(ctx context.Context, c *kube.Client, f v1alpha1.Fault) error {
	if f.Patch == nil {
		return fmt.Errorf("only patch faults are supported in this build (fault %q)", f.Name)
	}
	t := f.Patch.Target
	apiVersion := t.APIVersion
	if apiVersion == "" {
		apiVersion = kube.DefaultAPIVersion(t.Kind)
	}
	ri, err := c.ResourceFor(apiVersion, t.Kind, t.Namespace)
	if err != nil {
		return err
	}
	var pt types.PatchType
	switch f.Patch.Type {
	case "merge", "":
		pt = types.StrategicMergePatchType
	case "json":
		pt = types.JSONPatchType
	default:
		return fmt.Errorf("unknown patch type %q", f.Patch.Type)
	}
	if _, err := ri.Patch(ctx, t.Name, pt, f.Patch.Data, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch %s/%s: %w", t.Kind, t.Name, err)
	}
	return nil
}

// waitReadiness polls the readiness gates until all pass or the deadline hits,
// so the timer never starts on a not-yet-broken environment.
func waitReadiness(ctx context.Context, c *kube.Client, dir string, ch *v1alpha1.Challenge) error {
	gates := ch.Environment.Setup.Readiness
	if len(gates) == 0 {
		return nil
	}
	ev := &verify.Evaluator{Client: c, Dir: dir}
	deadline := time.Now().Add(90 * time.Second)
	for _, g := range gates {
		to := 60 * time.Second
		if g.Poll != nil && g.Poll.Timeout != "" {
			if d, err := time.ParseDuration(g.Poll.Timeout); err == nil {
				to = d
			}
		}
		gateDeadline := time.Now().Add(to)
		if gateDeadline.After(deadline) {
			gateDeadline = deadline
		}
		for {
			r := ev.Check(ctx, g)
			if r.Outcome == verify.Pass {
				break
			}
			if time.Now().After(gateDeadline) {
				return fmt.Errorf("readiness gate did not become true in time (%s): %s",
					to, r.Reason)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
	}
	return nil
}

func ptrBool(b bool) *bool { return &b }
