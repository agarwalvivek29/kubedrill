# Solution

The pods are stuck `Init:CrashLoopBackOff` because the `prepare` init container
exits with status 1. An init container must complete successfully before the main
container starts, so the pod never reaches Running.

Look at the init container's logs, then restore its command to one that succeeds:

    kubectl -n pipeline logs <pod> -c prepare        # shows: workspace missing; exit 1
    kubectl -n pipeline patch deployment builder --type=json \
      -p '[{"op":"replace","path":"/spec/template/spec/initContainers/0/command","value":["sh","-c","echo prepared"]}]'

The init container completes, the main container starts, and the deployment
reaches 2 available replicas.

**Diagnosis tip:** `Init:*` pod states point at an init container — inspect it
with `kubectl logs <pod> -c <init-name>`, not the main container.
