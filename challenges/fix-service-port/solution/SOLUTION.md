# Solution

The pods are Ready and the Service has endpoints, so the selector is correct. The
problem is the port mapping: the Service forwards `port: 80` to `targetPort: 8080`,
but nginx listens on `80` — so kube-proxy sends traffic to a closed port and every
connection is refused.

Point `targetPort` back at the container's port:

    kubectl -n shop patch service catalog --type=json \
      -p '[{"op":"replace","path":"/spec/ports/0/targetPort","value":80}]'

`curl catalog.shop` now returns 200.

**Diagnosis tip:** endpoints present but no traffic almost always means a
`targetPort` mismatch (or the container isn't actually listening on that port).
Compare `kubectl get svc -o yaml` against the pod's `containerPort`.
