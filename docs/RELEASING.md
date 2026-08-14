# Releasing kubedrill

Releases are cut by pushing a semver tag. GitHub Actions runs
[goreleaser](https://goreleaser.com) to build, package, sign, and publish
everything (Story 4.3, FR-19).

## Cut a release

```sh
git tag v0.1.0
git push origin v0.1.0
```

The `release` workflow (`.github/workflows/release.yaml`) then produces, for the
tag:

- **Binaries** — `darwin`/`linux` × `arm64`/`amd64` (no native Windows; WSL2
  covers Windows), version-stamped via `-ldflags` into `internal/buildinfo`.
- **Packages** — `deb` and `rpm`.
- **Homebrew formula** — pushed to the tap (see setup below).
- **SBOM** — one per archive (syft).
- **checksums.txt** — plus a **keyless cosign signature** (`checksums.txt.sig` +
  `checksums.txt.pem`) made with GitHub OIDC — no signing key to store.

## Verify a downloaded release

```sh
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github.com/agarwalvivek29/kubedrill' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# then check your download against it
sha256sum -c checksums.txt --ignore-missing
```

## One-time setup (maintainer / human-only)

These can't be done from code — do them before the first *public* release. Until
they exist, the release still succeeds: the Homebrew push self-skips when the tap
token is absent (`brews.skip_upload` in `.goreleaser.yaml`), and cosign signing
needs nothing but the workflow's `id-token: write` (already set).

1. **Register `kubedrill.dev`.** It is the permanent challenge `apiVersion`
   (`kubedrill.dev/v1alpha1`). Do this before challenges circulate widely.
2. **Homebrew tap.** Create the repo `agarwalvivek29/homebrew-tap`, then add a
   repo secret **`HOMEBREW_TAP_GITHUB_TOKEN`** (a PAT with `contents:write` on
   the tap) to the `kubedrill` repo. goreleaser then publishes the formula so
   `brew install agarwalvivek29/tap/kubedrill` works.
3. **cosign** — nothing to create. Signing is **keyless** (Fulcio/Rekor via the
   workflow's OIDC token). If you prefer a fixed key later, generate one with
   `cosign generate-key-pair`, store `COSIGN_PRIVATE_KEY` + `COSIGN_PASSWORD` as
   secrets, and switch the `signs` block to `--key`.

## Local dry run (no publish)

```sh
goreleaser check                     # validate the config
goreleaser build --snapshot --clean  # build all binaries locally, no release
goreleaser release --snapshot --clean --skip=publish,sign  # full artifacts, no upload
```
