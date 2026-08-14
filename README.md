# kubedrill

**Local-first Kubernetes practice labs.** A single Go binary that turns any machine with Docker into a Kubernetes practice lab: it spins up a throwaway [kind](https://kind.sigs.k8s.io/) cluster, injects a fault, hands you a real `kubectl` prompt, and grades your solution — with scoring, progressive hints, time limits, and rule grading from the API server's audit log. Everything runs locally and offline; your `~/.kube/config` is never touched.

> **Status: v0.1 in development.** This is the project bootstrap. The play loop, authoring toolchain, and rules engine land across the milestones below.

## Why

Hands-on Kubernetes practice — the kind that prepares you for the CKA/CKAD/CKS exams or for debugging a real cluster at 3am — is locked behind paid remote platforms or shallow browser scenarios. You can't run those offline, see how verification works, or author your own challenges. kubedrill makes challenges **declarative YAML** that anyone (human or LLM) can author, and proves every challenge is solvable and non-vacuous with an `author test` harness — the trust anchor that makes AI-authored content safe.

## Requirements

- Docker
- (that's it — the binary bundles everything else)

## Quick start

```sh
# once released (Homebrew, or download a signed binary / deb / rpm from Releases):
brew install agarwalvivek29/tap/kubedrill
kubedrill start fix-crashloop     # provisions a broken cluster + prints objectives
# ...fix it with your own kubectl...
kubedrill verify                  # scorecard
```

Every release ships `darwin`/`linux` × `arm64`/`amd64` binaries, `deb`/`rpm`
packages, an SBOM, and a keyless **cosign** signature over the checksums — see
[`docs/RELEASING.md`](docs/RELEASING.md) to verify a download.

Building from source:

```sh
make build
./kubedrill catalog               # available challenges (built-ins + installed packs)
./kubedrill version
```

## Design

- **Architecture:** hexagonal (ports-and-adapters) — a pure engine core behind the ports in `pkg/api`; kind, verifiers, rules, sources, and the store are inward-depending adapters.
- **The contract:** challenges are `kubedrill.dev/v1alpha1` manifests validated against a published JSON Schema.
- Architecture decisions are recorded in [`docs/adr/`](docs/adr/).

## Authoring challenges

Challenges are declarative and provably correct. The `kubedrill author` toolchain scaffolds, validates, lints, and — with `author test` — proves a challenge fails when unsolved and passes when solved, on a throwaway cluster. Every content PR is gated by the same harness in CI. See [`docs/AUTHORING.md`](docs/AUTHORING.md).

## License

[Apache-2.0](LICENSE).
