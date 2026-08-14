#!/usr/bin/env bash
# The PVC asks for StorageClass 'fast-ssd', which doesn't exist. storageClassName
# is immutable, so recreate the claim with the cluster's real class, 'standard'.
set -euo pipefail
kubectl -n warehouse delete pvc bulk --wait=true
kubectl -n warehouse apply -f - <<'YAML'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: bulk
  namespace: warehouse
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: standard
  resources:
    requests:
      storage: 128Mi
YAML
# Nudge the pending pod to re-evaluate its (now satisfiable) claim.
kubectl -n warehouse rollout restart deployment/loader
kubectl -n warehouse rollout status deployment/loader --timeout=120s
