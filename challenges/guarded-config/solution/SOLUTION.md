# Solution

The container's env var references ConfigMap key `DB_URL`, which doesn't exist;
the `warehouse` ConfigMap defines `DATABASE_URL`. Because the key is missing and
the reference isn't `optional`, the kubelet parks every pod in
`CreateContainerConfigError`.

Fix the **reference** — point it at the key that exists:

    kubectl -n analytics patch deployment reporting --type=json \
      -p '[{"op":"replace","path":"/spec/template/spec/containers/0/env/0/valueFrom/configMapKeyRef/key","value":"DATABASE_URL"}]'

Both replicas become available.

## The live guardrail

The `warehouse` ConfigMap is protected by an **enforced** rule
(`protect … enforce: true`). kubedrill installs a ValidatingAdmissionPolicy that
**blocks you at admission** if you try to delete it:

    $ kubectl -n analytics delete configmap warehouse
    Error from server: ... kubedrill: the warehouse ConfigMap is protected ...

The engine that sets up and resets the challenge is exempt from the policy, so
the environment is managed normally — only *your* attempt to remove the
protected object is blocked. The fix is always to correct the wiring, not to
delete the data.
