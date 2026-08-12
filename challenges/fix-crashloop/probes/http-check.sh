#!/bin/sh
# Probe runs inside the cluster (namespace: retail). Passes when the webshop
# Service serves HTTP; fails with a reason otherwise.
if wget -q -O /dev/null --timeout=5 "http://webshop.retail.svc.cluster.local"; then
  echo "webshop service served HTTP 200"
  exit 0
fi
echo "webshop service did not respond (no ready endpoints?)"
exit 1
