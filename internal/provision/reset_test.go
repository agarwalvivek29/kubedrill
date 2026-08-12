package provision

import (
	"os"
	"path/filepath"
	"testing"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
)

func TestSetupObjectsParse(t *testing.T) {
	dir := t.TempDir()
	manifest := `apiVersion: v1
kind: Namespace
metadata: { name: retail }
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: webshop, namespace: retail }
---
apiVersion: v1
kind: Service
metadata: { name: webshop, namespace: retail }
`
	if err := os.WriteFile(filepath.Join(dir, "01.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	ch := &v1alpha1.Challenge{}
	ch.Environment.Setup.Manifests = []v1alpha1.ManifestRef{{Path: "01.yaml"}}

	objs, err := setupObjects(dir, ch)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(objs) != 3 {
		t.Fatalf("want 3 objects, got %d: %+v", len(objs), objs)
	}
	// Reset deletes in reverse, skipping Namespaces — assert exactly which
	// objects would be deleted.
	var deleted []string
	for i := len(objs) - 1; i >= 0; i-- {
		if objs[i].Kind == "Namespace" {
			continue
		}
		deleted = append(deleted, objs[i].Kind+"/"+objs[i].Name)
	}
	want := []string{"Service/webshop", "Deployment/webshop"}
	if len(deleted) != len(want) {
		t.Fatalf("delete set = %v, want %v", deleted, want)
	}
	for i := range want {
		if deleted[i] != want[i] {
			t.Fatalf("delete[%d] = %q, want %q (reverse order, ns skipped)", i, deleted[i], want[i])
		}
	}
}
