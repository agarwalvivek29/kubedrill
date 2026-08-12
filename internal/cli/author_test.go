package cli

import (
	"bytes"
	"encoding/json"
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
