# Solution

The fault overrode the container `command` with `sh -c 'exit 1'`, so the
container exits immediately and the pod enters CrashLoopBackOff. Remove the
override so nginx's default entrypoint runs:

    kubectl -n retail patch deployment webshop --type=json \
      -p '[{"op":"remove","path":"/spec/template/spec/containers/0/command"}]'

The deployment rolls out and reaches 3 available replicas.
