# Solution
The fault added a nodeSelector (`kubedrill.io/pool: gpu`) that no node
satisfies, so the pods can't schedule. Remove it:

    kubectl -n batch patch deployment worker --type=json \
      -p '[{"op":"remove","path":"/spec/template/spec/nodeSelector"}]'
