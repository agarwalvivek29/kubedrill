package provision

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
	"github.com/agarwalvivek29/kubedrill/internal/kube"
)

// Reset restores a challenge to its broken-as-intended state in place: it
// deletes the workload objects setup created (everything except Namespaces,
// so we don't wait on namespace finalizers), waits for them to be gone, then
// re-applies setup, re-injects faults, and re-checks readiness. Returns an
// error if readiness can't be re-established — the engine then falls back to a
// hard reset.
func Reset(ctx context.Context, c *kube.Client, dir string, ch *v1alpha1.Challenge) error {
	objs, err := setupObjects(dir, ch)
	if err != nil {
		return err
	}
	// Delete in reverse declaration order; skip Namespaces (kept to avoid
	// termination races — re-apply is idempotent on them).
	for i := len(objs) - 1; i >= 0; i-- {
		o := objs[i]
		if o.Kind == "Namespace" {
			continue
		}
		if err := deleteAndWait(ctx, c, o); err != nil {
			return fmt.Errorf("reset delete %s/%s: %w", o.Kind, o.Name, err)
		}
	}
	// Re-apply setup + faults + readiness (the same path start uses).
	return Apply(ctx, c, dir, ch)
}

type objRef struct {
	APIVersion, Kind, Namespace, Name string
}

// setupObjects parses the challenge's setup manifests into object references.
func setupObjects(dir string, ch *v1alpha1.Challenge) ([]objRef, error) {
	var out []objRef
	for _, m := range ch.Environment.Setup.Manifests {
		raw, err := os.ReadFile(filepath.Join(dir, m.Path))
		if err != nil {
			return nil, err
		}
		for _, doc := range splitYAML(raw) {
			if len(doc) == 0 {
				continue
			}
			obj := map[string]any{}
			if err := yaml.Unmarshal(doc, &obj); err != nil {
				return nil, fmt.Errorf("parse %s: %w", m.Path, err)
			}
			u := unstructured.Unstructured{Object: obj}
			if u.GetKind() == "" {
				continue
			}
			out = append(out, objRef{
				APIVersion: u.GetAPIVersion(),
				Kind:       u.GetKind(),
				Namespace:  u.GetNamespace(),
				Name:       u.GetName(),
			})
		}
	}
	return out, nil
}

// deleteAndWait deletes one object and polls until it is gone (or already was).
func deleteAndWait(ctx context.Context, c *kube.Client, o objRef) error {
	ri, err := c.ResourceFor(o.APIVersion, o.Kind, o.Namespace)
	if err != nil {
		return err
	}
	err = ri.Delete(ctx, o.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := ri.Get(ctx, o.Name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s/%s still present after 30s", o.Kind, o.Name)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
