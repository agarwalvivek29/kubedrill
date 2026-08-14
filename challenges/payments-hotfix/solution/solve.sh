#!/usr/bin/env bash
# Fix in place: remove the bad command override so nginx's entrypoint runs.
# No delete/recreate (respects the protect rule); nothing touched in kube-system
# (respects the deny rule).
set -euo pipefail
kubectl -n prod patch deployment payments --type=json \
  -p '[{"op":"remove","path":"/spec/template/spec/containers/0/command"}]'
kubectl -n prod rollout status deployment/payments --timeout=120s
