#!/usr/bin/env bash

FILE=$1

if ! command -v jq &> /dev/null; then
  echo "Please install 'jq' first (e.g. sudo apt install jq)"
  exit 1
fi

if [[ ! -f "$FILE" ]]; then
  echo "File not found: $FILE"
  exit 1
fi

printf "📊 Node Condition Readiness Latency (in milliseconds)\n $1 \n"
printf "%-15s %10s %10s %10s %10s %10s %10s\n" "Condition" "Min" "P50" "P95" "P99" "Max" "Avg"
printf "%s\n" "--------------------------------------------------------------------------"

jq -r '
  .[] | 
  "\(.quantileName) \(.min) \(.P50) \(.P95) \(.P99) \(.max) \(.avg)"' "$FILE" |
while read name min p50 p95 p99 max avg; do
  printf "%-15s %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f\n" \
    "$name" \
    "$(echo "$min / 1000000" | bc -l)" \
    "$(echo "$p50 / 1000000" | bc -l)" \
    "$(echo "$p95 / 1000000" | bc -l)" \
    "$(echo "$p99 / 1000000" | bc -l)" \
    "$(echo "$max / 1000000" | bc -l)" \
    "$(echo "$avg / 1000000" | bc -l)"
done
