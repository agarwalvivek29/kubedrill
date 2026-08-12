package provision

import "testing"

func TestSplitYAML(t *testing.T) {
	in := []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: a\n---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: b\n")
	docs := splitYAML(in)
	if len(docs) != 2 {
		t.Fatalf("want 2 docs, got %d", len(docs))
	}
	if string(docs[0]) == "" || string(docs[1]) == "" {
		t.Fatalf("docs should be non-empty: %q", docs)
	}
}

func TestSplitYAMLSingleDoc(t *testing.T) {
	docs := splitYAML([]byte("kind: Pod\n"))
	if len(docs) != 1 {
		t.Fatalf("want 1 doc, got %d", len(docs))
	}
}
