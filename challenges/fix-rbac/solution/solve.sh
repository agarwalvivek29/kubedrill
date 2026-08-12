#!/usr/bin/env bash
set -euo pipefail
kubectl -n team patch role deployer --type=json \
  -p '[{"op":"add","path":"/rules/-","value":{"apiGroups":["apps"],"resources":["deployments"],"verbs":["create","get","list"]}}]'
