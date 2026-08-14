#!/usr/bin/env bash
# Fix web in place — remove the bad command override so nginx's entrypoint runs.
set -euo pipefail
kubectl -n edge patch deployment web --type=json \
  -p '[{"op":"remove","path":"/spec/template/spec/containers/0/command"}]'
kubectl -n edge rollout status deployment/web --timeout=120s
