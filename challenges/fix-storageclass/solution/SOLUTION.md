# Solution

`kubectl -n warehouse get pvc bulk` shows it Pending. The claim asks for
`storageClassName: fast-ssd`, but `kubectl get storageclass` shows the only class
is `standard` (kind's default local-path provisioner). Because a PVC's
`storageClassName` is **immutable**, you can't patch it — recreate the claim:

    kubectl -n warehouse delete pvc bulk
    kubectl -n warehouse apply -f - <<'YAML'
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata: { name: bulk, namespace: warehouse }
    spec:
      accessModes: [ReadWriteOnce]
      storageClassName: standard
      resources: { requests: { storage: 128Mi } }
    YAML
    kubectl -n warehouse rollout restart deployment/loader

The pending pod re-evaluates its now-satisfiable claim, binds, and schedules.

**Lesson:** the mistake here is a StorageClass name that doesn't exist on the
cluster. Always check `kubectl get storageclass` — and remember the field can't
be edited in place.
