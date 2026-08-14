# Solution

Get onto the node and read the incident note:

    kubedrill node-shell control-plane
    # cat /root/INCIDENT.txt

It points at the cause: the `web` container's `command` was overridden to exit
1. Restore it (in place):

    kubectl -n edge patch deployment web --type=json \
      -p '[{"op":"remove","path":"/spec/template/spec/containers/0/command"}]'

`web` rolls out to 2 available replicas.

## Advisory rules on node-access challenges

This challenge grants a **root shell on the node**. Root on a node can tamper
with the very audit log kubedrill grades from, so integrity rules here are
**advisory**: the `fix-in-place` protect rule is still evaluated and any
violation is shown on your scorecard for learning, but it does **not** change
your score or fail the run. kubedrill says so on the scorecard rather than
pretending the grade is tamper-proof.
