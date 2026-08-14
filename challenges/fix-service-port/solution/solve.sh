#!/usr/bin/env bash
# The Service forwards to targetPort 8080; nginx listens on 80. Fix the mapping.
set -euo pipefail
kubectl -n shop patch service catalog --type=json \
  -p '[{"op":"replace","path":"/spec/ports/0/targetPort","value":80}]'
