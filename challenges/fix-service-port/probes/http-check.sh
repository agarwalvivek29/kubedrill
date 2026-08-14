#!/bin/sh
# Passes when the catalog Service serves HTTP. Retries so it tolerates
# endpoints/routing settling.
i=0
while [ "$i" -lt 20 ]; do
  if wget -q -O /dev/null --timeout=3 "http://catalog.shop.svc.cluster.local"; then
    echo "catalog service served HTTP 200"
    exit 0
  fi
  i=$((i+1))
  sleep 1
done
echo "catalog service did not respond after 20s (wrong targetPort?)"
exit 1
