// Command gen-schema reflects the v1alpha1 Challenge type into a JSON Schema
// and writes it to internal/schema/challenge-v1alpha1.json.
//
// Run via `go generate ./...` (see internal/schema/schema.go) or directly:
//
//	go run ./hack/gen-schema
//
// The generated file is committed and embedded (AD-6: the schema is generated
// from the Go types and is the single published source of truth).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"

	v1alpha1 "github.com/agarwalvivek29/kubedrill/apis/challenge/v1alpha1"
)

func main() {
	r := &jsonschema.Reflector{
		ExpandedStruct: true,
		DoNotReference: false,
	}
	s := r.Reflect(&v1alpha1.Challenge{})
	s.ID = "https://kubedrill.dev/schema/v1alpha1/challenge.schema.json"
	s.Title = "kubedrill Challenge (v1alpha1)"

	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}
	out = append(out, '\n')

	dest := filepath.Join("internal", "schema", "challenge-v1alpha1.json")
	if err := os.WriteFile(dest, out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", dest, len(out), "bytes")
}
