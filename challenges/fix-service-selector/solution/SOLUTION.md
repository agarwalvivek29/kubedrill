# Solution

A Service finds its backing pods by matching its `spec.selector` against pod
labels. Here the Service selects `app=frontend-v2`, but the Deployment labels its
pods `app=frontend`. Nothing matches, so the endpoints controller writes an empty
Endpoints object and traffic to the Service goes nowhere — even though the pods
are perfectly healthy.

Point the selector back at the label the pods actually carry:

    kubectl -n web patch service frontend --type=merge \
      -p '{"spec":{"selector":{"app":"frontend"}}}'

Within a second or two the endpoints controller repopulates `endpoints/frontend`
with the pod IPs and the Service serves traffic again.

**Diagnosis tip:** `kubectl -n web get endpoints frontend` showing `<none>` while
the pods are Ready almost always means a selector/label mismatch — compare
`kubectl -n web get svc frontend -o wide` against `kubectl -n web get pods
--show-labels`.
