#!/usr/bin/env bash
# The readiness probe was aimed at port 8080; the container serves on 80.
# Point the probe back at 80 and wait for the rollout to go ready.
set -euo pipefail
kubectl -n shop patch deployment storefront --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/httpGet/port","value":80}]'
kubectl -n shop rollout status deployment/storefront --timeout=120s
