// Package rules grades deny/protect/require from the API-server audit log with
// default-charge attribution (AD-4, AD-5). The event types and attribution here
// are pure — they operate on decoded audit events, never on a cluster — so they
// are exhaustively unit-testable against real captured audit output.
package rules

import "encoding/json"

// EngineIdentity is the username of the engine's client certificate (minted as
// CN=kubedrill:engine in providers/kind/identity.go). It is the ONLY non-system
// identity the grader exempts, and impersonating it is tampering (AD-4).
const EngineIdentity = "kubedrill:engine"

// AuditEvent is the subset of a Kubernetes audit.k8s.io/v1 Event that grading
// needs. Field shapes are confirmed against real kind audit output (the Epic 3
// recon spike): `user`/`impersonatedUser`, `objectRef`, and — at Request level —
// `requestObject` for field-level `require` checks.
type AuditEvent struct {
	AuditID          string          `json:"auditID"`
	Stage            string          `json:"stage"`
	Level            string          `json:"level"`
	Verb             string          `json:"verb"`
	User             Actor           `json:"user"`
	ImpersonatedUser *Actor          `json:"impersonatedUser,omitempty"`
	ObjectRef        ObjectRef       `json:"objectRef"`
	RequestObject    json.RawMessage `json:"requestObject,omitempty"`
}

// Actor is an audit event's authenticated (or impersonated) identity.
type Actor struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups,omitempty"`
}

// ObjectRef identifies the object an event acted on.
type ObjectRef struct {
	Resource    string `json:"resource"`
	Namespace   string `json:"namespace,omitempty"`
	Name        string `json:"name,omitempty"`
	APIGroup    string `json:"apiGroup,omitempty"`
	APIVersion  string `json:"apiVersion,omitempty"`
	Subresource string `json:"subresource,omitempty"`
}
