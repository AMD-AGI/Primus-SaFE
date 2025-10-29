#!/usr/bin/env bash

FILE=$1

if ! command -v jq &> /dev/null; then
  echo "Please install 'jq' first (e.g., sudo apt install jq)"
  exit 1
fi

if [[ ! -f "$FILE" ]]; then
  echo "File not found: $FILE"
  exit 1
fi

printf "nodeReadyLatency per node (in milliseconds) \n 📊 $1 \n"
echo "----------------------------------------------"
jq -r '.[] | "\(.nodeName) \(.nodeReadyLatency / 1000000)"' "$FILE" |
sort -k2 -n | tee /tmp/_node_latency_ms.txt

echo
echo "📈 Summary Statistics"
count=$(wc -l < /tmp/_node_latency_ms.txt)
total=$(awk '{sum += $2} END {print sum}' /tmp/_node_latency_ms.txt)
avg=$(awk -v n=$count -v t=$total 'BEGIN {printf "%.2f", t/n}')
min=$(awk 'NR==1 {print $2}' /tmp/_node_latency_ms.txt)
max=$(awk 'END {print $2}' /tmp/_node_latency_ms.txt)

echo "Total nodes     : $count"
echo "Average latency : $avg ms"
echo "Minimum latency : $min ms"
echo "Maximum latency : $max ms"
