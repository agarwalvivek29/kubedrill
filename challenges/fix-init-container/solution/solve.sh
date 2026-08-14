#!/usr/bin/env bash
# The init container 'prepare' was changed to exit 1, so the pod never leaves
# Init. Restore it to a successful command.
set -euo pipefail
kubectl -n pipeline patch deployment builder --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/initContainers/0/command","value":["sh","-c","echo prepared"]}]'
kubectl -n pipeline rollout status deployment/builder --timeout=120s
