#!/usr/bin/env bash
# content-gate.sh — run `kubedrill author test` on every challenge changed in a
# PR (Story 2.5, FR-15/FR-21). Used by .github/workflows/content-gate.yaml.
#
# It is fork-PR safe by construction: it reads only the checked-out working tree
# and the git diff, shells out to the built kubedrill binary, and needs no
# secrets. The workflow that calls it uses `pull_request` (never
# pull_request_target) with a read-only token, so an untrusted solve.sh runs
# with no privileged credentials in scope.
#
# Usage: content-gate.sh <base-sha> [kubedrill-binary]
#   base-sha         the PR base commit to diff against (git three-dot merge-base)
#   kubedrill-binary path to the built binary (default: ./kubedrill)
#
# Written for portability (no bash 4+ mapfile/associative arrays) so it runs the
# same on a maintainer's machine and on ubuntu CI. Challenge paths are DNS-1123
# directory names, never containing spaces, so word-splitting the diff is safe.
set -euo pipefail

BASE="${1:?usage: content-gate.sh <base-sha> [kubedrill-binary]}"
BIN="${2:-./kubedrill}"

# Roots we treat as challenge collections. packs/ may not exist yet — a pathspec
# that matches nothing is not an error for git diff.
ROOTS="challenges packs"

# Files changed by the PR under those roots (three-dot: changes introduced since
# the merge-base with BASE).
changed=$(git diff --name-only "${BASE}...HEAD" -- $ROOTS)

# Reduce changed files to the set of challenge directories (those that still
# contain a challenge.yaml in the working tree — deletions are skipped).
seen=" "
dirs=""
for f in $changed; do
  d="$f"
  while [ -n "$d" ] && [ "$d" != "." ]; do
    if [ -f "$d/challenge.yaml" ]; then
      case "$seen" in
        *" $d "*) : ;; # already collected
        *) seen="$seen$d "; dirs="$dirs $d" ;;
      esac
      break
    fi
    d=$(dirname "$d")
  done
done

# shellcheck disable=SC2086 # intentional word-splitting of space-free paths
set -- $dirs
if [ "$#" -eq 0 ]; then
  echo "content-gate: no changed challenges to test."
  exit 0
fi

echo "content-gate: testing $# changed challenge(s):$dirs"
failed=""
nfail=0
for d in "$@"; do
  echo "::group::author test ${d}"
  if "$BIN" author test "$d"; then
    echo "PASS ${d}"
  else
    echo "::error title=author test failed::${d} did not pass author test"
    failed="$failed $d"
    nfail=$((nfail + 1))
  fi
  echo "::endgroup::"
done

if [ "$nfail" -gt 0 ]; then
  echo "content-gate: FAILED (${nfail}/$#):$failed"
  exit 1
fi
echo "content-gate: all $# challenge(s) passed."
