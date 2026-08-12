# Solution
Re-add the deployments rule the fault stripped:

    kubectl -n team patch role deployer --type=json \
      -p '[{"op":"add","path":"/rules/-","value":{"apiGroups":["apps"],"resources":["deployments"],"verbs":["create","get","list"]}}]'
