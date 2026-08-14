# Solution

The pod is Pending because the deployment's volume references a PVC named
`ledger-old`, which doesn't exist. `kubectl -n records get pvc` shows the real,
Bound claim is `ledger`. Point the volume back at it:

    kubectl -n records patch deployment archiver --type=json \
      -p '[{"op":"replace","path":"/spec/template/spec/volumes/0/persistentVolumeClaim/claimName","value":"ledger"}]'

The pod binds the claim, schedules, and the deployment reaches 1 available
replica.

**Diagnosis tip:** a pod stuck Pending with `persistentvolumeclaim "…" not found`
is a name mismatch between the pod's volume and the actual PVC — compare
`kubectl get pvc` against the volume's `claimName`.
