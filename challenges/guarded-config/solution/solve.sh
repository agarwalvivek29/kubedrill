#!/usr/bin/env bash
# Fix the reference (key DB_URL -> DATABASE_URL). The warehouse ConfigMap is
# protected by a live admission policy, so the fix is to correct the wiring —
# never to delete/recreate the ConfigMap.
set -euo pipefail
kubectl -n analytics patch deployment reporting --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/containers/0/env/0/valueFrom/configMapKeyRef/key","value":"DATABASE_URL"}]'
kubectl -n analytics rollout status deployment/reporting --timeout=120s
