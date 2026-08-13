# Solution

The container declares an environment variable sourced from a ConfigMap:

    env:
      - name: DATABASE_URL
        valueFrom:
          configMapKeyRef:
            name: app-config
            key: DB_URL      # ← wrong

But `app-config` defines `DATABASE_URL`, not `DB_URL`. Because the key doesn't
exist and the reference isn't marked `optional`, the kubelet can't build the
container's environment and parks every pod in `CreateContainerConfigError`, so
the Deployment never reaches its 2 available replicas.

Repoint the reference at the key that actually exists:

    kubectl -n billing patch deployment invoicer --type=json \
      -p '[{"op":"replace","path":"/spec/template/spec/containers/0/env/0/valueFrom/configMapKeyRef/key","value":"DATABASE_URL"}]'

The rollout proceeds and both replicas become available.

**Alternative:** you could add a `DB_URL` key to the ConfigMap, but the intended
fix is to correct the reference — the ConfigMap is the source of truth and the
key name `DATABASE_URL` is what the app expects.
