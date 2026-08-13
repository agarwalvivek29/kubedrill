#!/usr/bin/env bash
# The Service selects app=frontend-v2, but the pods are labelled app=frontend.
# Repoint the selector so the endpoints controller finds the pods.
set -euo pipefail
kubectl -n web patch service frontend --type=merge \
  -p '{"spec":{"selector":{"app":"frontend"}}}'
# Give the endpoints controller a moment and confirm the endpoints populate.
for i in $(seq 1 15); do
  if [ -n "$(kubectl -n web get endpoints frontend -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null)" ]; then
    echo "frontend Service now has endpoints"
    exit 0
  fi
  sleep 2
done
echo "endpoints did not populate in time" >&2
exit 1
