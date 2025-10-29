#!/usr/bin/env bash

DATA=$1
echo "📊 ${DATA}"

stats=$(jq -r '.[] | select(.quantileName == "Ready") |
               "min=\(.min) avg=\(.avg) P50=\(.P50) P95=\(.P95) P99=\(.P99) max=\(.max)"' \
               "$DATA")

if [[ -z "$stats" ]]; then
  echo "not found"
  exit 1
fi

eval "$stats"
echo "Min   : ${min} ms"
echo "Median: ${P50} ms"
echo "Avg   : ${avg} ms"
echo "P95   : ${P95} ms"
echo "P99   : ${P99} ms"
echo "Max   : ${max} ms"
