package rules

import "strings"

// Charge is the result of AD-4 default-charge attribution: who an audit event
// is held against for grading.
type Charge int

const (
	// ChargePlayer: the event counts against the player. This is the DEFAULT —
	// the player, any player-created ServiceAccount, and any unknown identity all
	// land here, so no wrapper (a Job's SA) or unfamiliar actor can escape
	// grading.
	ChargePlayer Charge = iota
	// ChargeExempt: the event is the engine's own work or a pinned control-plane
	// component. Grading ignores it, so engine setup/reset and controllers are
	// never blamed on the player.
	ChargeExempt
	// ChargeTamper: the player impersonated the engine identity — an integrity
	// breach that fails the challenge outright (penalty: fail).
	ChargeTamper
)

func (c Charge) String() string {
	switch c {
	case ChargeExempt:
		return "exempt"
	case ChargeTamper:
		return "tamper"
	default:
		return "player"
	}
}

// Attribute applies the AD-4 default-charge model to a single audit event.
//
// The exempt set is deliberately tiny and explicit — the engine identity plus a
// pinned control-plane allowlist — and everything else is charged to the player.
// This is the invariant that closes the ServiceAccount and impersonation
// bypasses: a player who runs a forbidden action through a Job's SA shows up as
// `system:serviceaccount:<their-ns>:<sa>` (never on the kube-system allowlist),
// and a player who impersonates a controller to launder an action is still
// charged, because impersonation is always a player act. Impersonating the
// engine identity is tampering.
//
// Grading and live enforcement (Story 3.4) must exempt EXACTLY this set — never
// `system:*` wholesale — so the two identity models are one definition (AD-5).
func Attribute(e AuditEvent) Charge {
	// Impersonation is always initiated by the player (the engine and controllers
	// act under their own identities, never via impersonate). So an impersonated
	// event is a player action — unless it impersonates the engine, which is
	// tampering. Impersonating a controller does NOT launder the action.
	if e.ImpersonatedUser != nil {
		if e.ImpersonatedUser.Username == EngineIdentity {
			return ChargeTamper
		}
		return ChargePlayer
	}

	if isExempt(e.User.Username) {
		return ChargeExempt
	}
	return ChargePlayer
}

// The AD-4 exempt set — the ONLY identities grading and live enforcement ignore.
// It is deliberately narrow and shared by BOTH paths (grading via isExempt,
// enforcement via the generated VAP matchConditions in enforce.go) so the two
// are one definition (AD-5): it never matches `system:*` wholesale, and in
// particular never a non-kube-system ServiceAccount (so a player-created SA
// stays chargeable and blockable). The exact usernames were confirmed against
// real kind audit output.
func exemptUsernames() []string {
	return []string{
		EngineIdentity,
		"system:apiserver",
		"system:kube-controller-manager",
		"system:kube-scheduler",
		"system:kube-proxy",
	}
}

// exemptPrefixes are username prefixes that are exempt: the kubelet(s)
// (`system:node:<node>`) and the built-in controllers, which run as kube-system
// ServiceAccounts (`system:serviceaccount:kube-system:<controller>`).
func exemptPrefixes() []string {
	return []string{
		"system:node:",
		"system:serviceaccount:kube-system:",
	}
}

// isExempt reports whether a username is in the AD-4 exempt set.
func isExempt(username string) bool {
	for _, u := range exemptUsernames() {
		if username == u {
			return true
		}
	}
	for _, p := range exemptPrefixes() {
		if strings.HasPrefix(username, p) {
			return true
		}
	}
	return false
}
