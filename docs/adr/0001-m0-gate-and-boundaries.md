# ADR 0001 — M0 gate: name, frozen semantics, and package boundaries

- **Status:** Accepted
- **Date:** 2026-08-12
- **Context:** Story 1.1 (bootstrap). Recorded before any schema types are written.

## Decision

### Identity (permanent)
- Product name: **kubedrill** (chosen over the working name "kodelocal" to avoid confusion with KodeKloud).
- Go module: `github.com/agarwalvivek29/kubedrill`.
- Challenge apiVersion: `kubedrill.dev/v1alpha1`.
- The GitHub repository is being renamed to `kubedrill` to match.

These are baked into every authored challenge and cannot change without breaking the content contract.

### Frozen at M0 (AD-6) — before `apis/challenge/v1alpha1/types.go` is written
The following schema semantics are frozen because they live in every authored challenge:
- `match:` — partial-object subset matching (maps recursive; arrays unordered-subset; `null` asserts absent/null; scalars numeric-normalized).
- CEL helper surface — exactly two functions: `snapshot()` and `restarts0()`.
- Rule-verb set — `{deny, protect, require}`.
- Check combinator — `checks:` is AND, `anyOf:` is OR, one level of nesting.

### Package boundaries (AD-12, AD-1)
- Only `pkg/api` (ports) and `apis/challenge/...` (schema types) are importable by third parties. Everything under `internal/` is private.
- The engine core (`internal/engine`) imports only `pkg/api` and `apis/` — never a concrete adapter. Adapters depend inward and never on each other; wiring happens once in `cmd/kubedrill`.
- `pkg/api` receives a semver commitment at v1.0; until then the schema evolves via conversion-on-load.

## Consequences
- Contributors have a stable contract to author challenges and plugins against from day one.
- A future rename or semantics change is a deliberate, versioned migration — not an incidental edit.

## References
- Architecture Spine AD-1, AD-6, AD-12; HLD §11 (decision log); PRD §8 (name resolution).
