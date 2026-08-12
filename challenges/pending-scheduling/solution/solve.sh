#!/usr/bin/env bash
set -euo pipefail
kubectl -n batch patch deployment worker --type=json \
  -p '[{"op":"remove","path":"/spec/template/spec/nodeSelector"}]'
kubectl -n batch rollout status deployment/worker --timeout=120s
