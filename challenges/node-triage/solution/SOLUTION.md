# Solution

Get onto the node and read the kubelet's triage log:

    kubedrill node-shell control-plane
    # cat /root/kubelet-triage.log

It states the cause plainly: the readiness probe targets `:9999`, but the
container listens on `:80`, so every probe is refused and no pod is marked Ready.
Fix the probe port — in place:

    kubectl -n frontline patch deployment web --type=json \
      -p '[{"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/httpGet/port","value":80}]'

The pods pass readiness and the deployment reaches 2 ready replicas.

## Advisory rules on node-access challenges

This challenge grants a root shell on the node. Root on a node can tamper with
the audit log kubedrill grades from, so integrity rules here are **advisory**:
the `fix-in-place` protect rule is still evaluated and any violation is shown for
learning, but it does not change your score or fail the run. kubedrill says so on
the scorecard rather than pretending the grade is tamper-proof.
