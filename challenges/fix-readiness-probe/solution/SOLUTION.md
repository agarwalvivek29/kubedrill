# Solution

The pods run fine, but each container's **readiness probe** does an HTTP GET on
port `8080` while nginx serves on port `80`. Every probe gets a connection
refused, so the kubelet never marks a pod Ready — `readyReplicas` stays at 0 and
the Deployment rollout hangs.

Point the probe back at the port the container actually listens on:

    kubectl -n shop patch deployment storefront --type=json \
      -p '[{"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/httpGet/port","value":80}]'

The patch triggers a new rollout; the pods pass their readiness checks and the
Deployment reaches 3 ready replicas.

**Why not just delete the pods?** The probe is defined in the Deployment's pod
template, so recreated pods would inherit the same broken probe. The fix has to
happen in the template.
