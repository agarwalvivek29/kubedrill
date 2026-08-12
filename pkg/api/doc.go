// Package api defines kubedrill's public ports — the interfaces third parties
// implement to extend the platform: EnvProvider, Environment, Verifier,
// PackSource, SessionStore, and ChallengeType.
//
// This package and apis/challenge/... are the ONLY packages importable by
// third parties (AD-12). Everything under internal/ is private. The interfaces
// here receive a semver commitment at v1.0; until then they may change with
// conversion-on-load.
//
// The concrete port definitions land in Epic 1 (EnvProvider, Environment,
// Verifier, SessionStore) and later epics (PackSource, ChallengeType). This
// file establishes the package as the public boundary.
package api
