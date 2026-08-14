#!/usr/bin/env bash
# The volume referenced a non-existent PVC (ledger-old); the real claim is ledger.
set -euo pipefail
kubectl -n records patch deployment archiver --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/volumes/0/persistentVolumeClaim/claimName","value":"ledger"}]'
kubectl -n records rollout status deployment/archiver --timeout=120s
