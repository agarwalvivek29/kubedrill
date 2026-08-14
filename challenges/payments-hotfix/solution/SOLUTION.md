# Solution

A bad rollout overrode the container `command` with `sh -c 'exit 1'`, so every
pod exits immediately and the Deployment sits at 0 available replicas.

Fix it **in place** — remove the override so the image's default entrypoint runs:

    kubectl -n prod patch deployment payments --type=json \
      -p '[{"op":"remove","path":"/spec/template/spec/containers/0/command"}]'

The Deployment rolls out and reaches 3 available replicas.

## The rules of engagement (graded from the audit log)

- **`fix-in-place` (protect, fail):** deleting and recreating `payments` would
  "fix" it too — but it's forbidden. Deleting the Deployment fails the challenge
  outright, even if the replacement is healthy. Patch it in place.
- **`hands-off-system` (deny, −25):** touching any Deployment in `kube-system`
  (coredns, etc.) costs 25 points. There's no reason to — stay in `prod`.

Nothing blocks you live; the grader charges only *your* actions (controllers and
the engine are never blamed on you), so the scorecard reflects exactly what you
did.
