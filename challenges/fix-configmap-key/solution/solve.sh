#!/usr/bin/env bash
# The env var references ConfigMap key DB_URL, which doesn't exist; the actual
# key is DATABASE_URL. Repoint the configMapKeyRef and let the rollout complete.
set -euo pipefail
kubectl -n billing patch deployment invoicer --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/containers/0/env/0/valueFrom/configMapKeyRef/key","value":"DATABASE_URL"}]'
kubectl -n billing rollout status deployment/invoicer --timeout=120s
