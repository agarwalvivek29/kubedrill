#!/usr/bin/env bash
# The kubelet triage log points at the cause: the readiness probe targets :9999,
# but nginx serves on :80. Fix the probe port (in place).
set -euo pipefail
kubectl -n frontline patch deployment web --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/httpGet/port","value":80}]'
kubectl -n frontline rollout status deployment/web --timeout=120s
