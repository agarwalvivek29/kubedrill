#!/usr/bin/env bash
set -euo pipefail
kubectl -n retail patch deployment webshop --type=json \
  -p '[{"op":"remove","path":"/spec/template/spec/containers/0/command"}]'
kubectl -n retail rollout status deployment/webshop --timeout=120s
