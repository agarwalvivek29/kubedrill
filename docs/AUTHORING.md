# Authoring challenges

A kubedrill challenge is a directory of declarative YAML plus a reference
solution. The authoring toolchain takes you from an empty directory to a
challenge that is *proven* correct — one that fails when unsolved, passes when
solved, and does so idempotently — before it ever reaches a player or the
content gate in CI.

## The toolchain

| Command | What it does | Needs Docker? |
|---------|--------------|:---:|
| `kubedrill author schema --print` | Print the authoritative `kubedrill.dev/v1alpha1` JSON Schema — the exact contract. Prompt an LLM with this to generate a parseable challenge. | no |
| `kubedrill author new <name>` | Scaffold a complete, loadable challenge skeleton (`challenge.yaml`, `setup/`, `probes/`, `solution/`). | no |
| `kubedrill author validate <dir>` | Strict decode + semantic validation (unique ids, acyclic `dependsOn`, exactly-one-of unions) + referential checks (referenced files exist) + `match`/`cel` compilation. Fast enough to run on every save. | no |
| `kubedrill author lint <dir>` | Opinionated quality/safety rules: ≥1 hint, no trivially-vacuous checks, scoped `enforce` rules, no field-level `require` on Secrets. | no |
| `kubedrill author test <dir>` | The correctness harness — provisions a throwaway cluster and proves the challenge (see below). | **yes** |

`validate` and `lint` are cluster-free and instant; run them constantly.
`test` is the moat — run it before you open a PR.

A typical loop:

```sh
kubedrill author new fix-dns
# …edit challenge.yaml, setup/, solution/solve.sh…
kubedrill author validate fix-dns   # structure
kubedrill author lint     fix-dns   # quality
kubedrill author test     fix-dns   # correctness (Docker)
```

## `author test`: the correctness harness

On a throwaway kind cluster (always torn down afterward; `--keep` to retain it
for debugging), `author test` runs three phases:

1. **negative** — provisions the fresh, broken environment and evaluates *every*
   objective **directly, ignoring `dependsOn`**. Every objective must **fail**
   (an *errored* check counts as failing) — so a vacuous objective cannot hide
   behind an unmet dependency. An objective declared `expectInitiallyPassing:
   true` must instead **pass**. Any violation is reported as vacuity, naming the
   offending objective.
2. **positive** — runs your reference `solve.sh` (see the contract below), then
   verifies. It must score **100%**. A `solve.sh` that exits non-zero, or an
   *errored* check here, fails the harness.
3. **idempotency** — verifies a second time; it must still pass.

`-o json` emits the full structured report for CI and agents:

```sh
kubedrill author test fix-dns -o json
```

## The `solve.sh` execution contract (FR-16)

The positive phase runs your reference solution exactly as a player running it
by hand would. The contract is fixed:

- **Interpreter:** the host's `bash` (`bash solution/solve.sh`). It runs on the
  machine invoking `author test`, not inside the cluster.
- **Working directory:** the **challenge directory**. Relative paths in the
  script resolve against it — e.g. `kubectl apply -f setup/extra.yaml` or
  `solution/patch.yaml` work as written.
- **`KUBECONFIG`:** points at the **player** kubeconfig for the throwaway
  session (the same least-privileged identity a player gets — never your
  `~/.kube/config`).
- **Network:** allowed. Solutions may pull images or reach the API server.
- **Exit code:** a **non-zero exit fails the harness.** Write `set -euo
  pipefail` and let failures propagate; do not swallow errors.

A good `solve.sh` is deterministic and waits for the state it asserts (e.g.
`kubectl rollout status …`) so the positive and idempotency phases are stable.

## The content gate (CI)

Every pull request that touches `challenges/**` or `packs/**` triggers the
`content-gate` workflow, which runs `author test` against each **changed** or
**added** challenge on a real kind cluster. A challenge that fails **blocks the
merge** — this is what lets maintainers trust community- and AI-authored
content.

The gate is hardened for untrusted contributions (NFR-5): it runs on
`pull_request` (never `pull_request_target`), so a fork's `solve.sh` executes
**with no repository secrets in scope** and a read-only token; action versions
are pinned to commit SHAs. The harness has nothing privileged to exfiltrate.

Run the same check locally before pushing:

```sh
make build
./hack/content-gate.sh "$(git merge-base origin/main HEAD)" ./kubedrill
```
