#!/bin/sh
# Probe runs inside the cluster (namespace: retail). Passes when the webshop
# Service serves HTTP; retries briefly so it tolerates endpoints settling.
i=0
while [ "$i" -lt 20 ]; do
  if wget -q -O /dev/null --timeout=3 "http://webshop.retail.svc.cluster.local"; then
    echo "webshop service served HTTP 200"
    exit 0
  fi
  i=$((i+1))
  sleep 1
done
echo "webshop service did not respond after 20s (no ready endpoints?)"
exit 1
