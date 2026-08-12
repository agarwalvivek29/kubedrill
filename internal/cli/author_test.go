package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthorSchemaPrintsValidJSON(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"author", "schema", "--print"})

	if err := root.Execute(); err != nil {
		t.Fatalf("author schema errored: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("schema output is not valid JSON: %v", err)
	}
	if _, ok := doc["properties"]; !ok {
		t.Fatalf("schema missing top-level properties")
	}
}

func TestAuthorNewScaffolds(t *testing.T) {
	parent := t.TempDir()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"author", "new", "scaffolded", "--dir", parent})

	if err := root.Execute(); err != nil {
		t.Fatalf("author new errored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "scaffolded", "challenge.yaml")); err != nil {
		t.Fatalf("expected scaffolded challenge.yaml: %v", err)
	}

	// A second run into the same directory must fail rather than clobber.
	root = newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"author", "new", "scaffolded", "--dir", parent})
	if err := root.Execute(); err == nil {
		t.Fatal("author new into existing dir: got nil error, want refusal")
	}
}
