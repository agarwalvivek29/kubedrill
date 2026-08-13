# Epic 2 — the authoring moat, proven

**TL;DR:** kubedrill can now scaffold, validate, lint, and — crucially — *prove*
a Kubernetes challenge is real: that it genuinely fails until you do the work and
passes once you do, idempotently, on a throwaway cluster. Every content PR is
gated by that same proof in CI. To show it isn't just theory, we authored three
brand-new CKAD-style challenges end-to-end through the toolchain and ran them
through the harness on real kind clusters. They pass. This is the trust anchor
that makes human- **and** AI-authored content safe to ship.

## What Epic 2 delivers

Five subcommands under `kubedrill author`:

| Command | Purpose | Cluster? |
|---------|---------|:---:|
| `schema --print` | Emit the authoritative `kubedrill.dev/v1alpha1` JSON Schema — the exact contract to hand a human or an LLM. | no |
| `new <name>` | Scaffold a complete, loadable challenge skeleton. | no |
| `validate <dir>` | Strict decode + semantic checks (unique ids, acyclic `dependsOn`, exactly-one-of unions) + referential checks + `match`/`cel` compilation. | no |
| `lint <dir>` | Quality/safety rules: ≥1 hint, no vacuous checks, scoped `enforce`, no field-`require` on Secrets. | no |
| `test <dir>` | **The moat** — provision a throwaway cluster and prove the challenge (negative / positive / idempotency). | **yes** |

And the CI content gate (`.github/workflows/content-gate.yaml` +
`hack/content-gate.sh`): on every PR touching `challenges/**` or `packs/**`, it
runs `author test` against each changed challenge under fork-PR hardening
(`pull_request` not `pull_request_target`, no secrets in scope, pinned action
SHAs). A challenge that fails **blocks the merge**.

Full authoring guide: [`AUTHORING.md`](AUTHORING.md).

## Why `author test` is the moat

Anyone — a contributor, an LLM in a loop — can write plausible-looking challenge
YAML. Plausible YAML is worthless if the "broken" cluster isn't actually broken,
or if the objective passes without doing anything, or if the reference solution
doesn't work. `author test` makes that impossible to fake:

- **Negative phase** — on the fresh, unsolved cluster, *every* objective is
  evaluated **directly, ignoring `dependsOn`**, and must **fail** (an errored
  check counts as failing). This is what catches a vacuous objective — one that
  would pass without the player doing the work — because it can't hide behind an
  unmet dependency. (An objective may opt in to `expectInitiallyPassing: true`,
  which must instead pass.)
- **Positive phase** — the reference `solve.sh` runs, then verification must
  score **100%**.
- **Idempotency phase** — a second verification must still pass.

## The proof: three new CKAD challenges, authored through the toolchain

We picked three distinct CKAD domains and authored each from an empty directory
using only the toolchain — `author new`, then edit, then `validate` → `lint` →
`test`.

| Challenge | CKAD domain | The fault | The fix | Verified by |
|-----------|-------------|-----------|---------|-------------|
| `fix-readiness-probe` | Observability | readiness probe aimed at port 8080; nginx serves on 80 → 0/3 Ready | repoint the probe at 80 | `match` on `status.readyReplicas == 3` |
| `fix-configmap-key` | Configuration | env var references ConfigMap key `DB_URL`; the key is `DATABASE_URL` → `CreateContainerConfigError` | fix the `configMapKeyRef` key | `match` on `status.availableReplicas == 2` |
| `fix-service-selector` | Services & networking | Service selects `app=frontend-v2`; pods are `app=frontend` → no endpoints | repoint the Service selector | `cel` on the `Endpoints` object having ready addresses |

Together they exercise the harness across **two check kinds** (`match` and `cel`)
and **multiple resource kinds** (Deployment status, and the `Endpoints` object) —
not a single copy-pasted template.

### Cluster-free checks (instant, run on every save)

```
$ kubedrill author validate challenges/fix-readiness-probe
✓ fix-readiness-probe is valid (kubedrill.dev/v1alpha1)
  1 objective · 1 check · 1 fault · 2 hints · no cluster needed

$ kubedrill author lint challenges/fix-configmap-key
✓ fix-configmap-key: no lint findings
```

### The harness on real kind

All three, run on real kind clusters, passed every phase:

```
$ kubedrill author test challenges/fix-readiness-probe
  ✓ negative     all 1 objective(s) behaved correctly on the fresh environment
  ✓ positive     100% — 100/100 points across 1 objective(s)
  ✓ idempotency  100% — 100/100 points across 1 objective(s)
✓ fix-readiness-probe is solvable and non-vacuous

$ kubedrill author test challenges/fix-configmap-key
  ✓ negative     all 1 objective(s) behaved correctly on the fresh environment
  ✓ positive     100% — 100/100 points across 1 objective(s)
  ✓ idempotency  100% — 100/100 points across 1 objective(s)
✓ fix-configmap-key is solvable and non-vacuous

$ kubedrill author test challenges/fix-service-selector
  ✓ negative     all 1 objective(s) behaved correctly on the fresh environment
  ✓ positive     100% — 100/100 points across 1 objective(s)
  ✓ idempotency  100% — 100/100 points across 1 objective(s)
✓ fix-service-selector is solvable and non-vacuous
```

Every challenge: **negative** proves each objective fails on the fresh cluster,
**positive** drives the reference solution to 100%, **idempotency** re-verifies.
No leftover clusters or temp directories — each run provisions and tears down its
own throwaway environment.

### The same proof, in CI

Because these three challenges are new content, the PR that adds them triggers the
`content-gate` workflow, which runs exactly this `author test` on each of them on
a GitHub-hosted kind cluster — the ultimate dogfood. A green check on that job is
CI vouching that the challenges are solvable and non-vacuous, with no privileged
credentials ever in reach of the challenge's `solve.sh`.

## Status

**Epic 2 is complete (5/5).** The authoring toolchain and its CI gate are shipped;
built-in content grew from 3 to 6 challenges, each proven by the harness. Next up
is **Epic 3 — enforce rules and grade integrity** (deny/protect/require from the
API-server audit log, actor attribution, live enforcement, node-level
challenges).
