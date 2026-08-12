// Package schema holds the embedded, generated JSON Schema for the v1alpha1
// Challenge document — the single published source of truth (AD-6).
//
//go:generate go run ../../hack/gen-schema
package schema

import _ "embed"

//go:embed challenge-v1alpha1.json
var challengeV1Alpha1 []byte

// ChallengeV1Alpha1 returns the JSON Schema bytes for kubedrill.dev/v1alpha1.
func ChallengeV1Alpha1() []byte { return challengeV1Alpha1 }
