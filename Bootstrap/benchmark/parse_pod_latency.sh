#!/usr/bin/env bash
DATA=$1
echo "📊 ${DATA} "
latencies=$(jq -r '.[].podReadyLatency' "$DATA" | sort -n)
N=$(echo "$latencies" | wc -l)
MIN=$(echo "$latencies" | head -n1)
MAX=$(echo "$latencies" | tail -n1)
SUM=$(echo "$latencies" | awk '{s+=$1} END{printf "%f", s}')
MEAN=$(echo "scale=2; $SUM / $N" | bc)

percentile() {
  p=$1
  idx=$(( (N * p + 99) / 100 ))
  echo "$latencies" | sed -n "${idx}p"
}

MEDIAN=$(percentile 50)
P75=$(percentile 75)
P90=$(percentile 90)

printf "Count : %d\n" "$N"
printf "Min    : %d ms\n" "$MIN"
printf "Max    : %d ms\n" "$MAX"
printf "Mean   : %s ms\n" "$MEAN"
printf "Median : %s ms\n" "$MEDIAN"
printf "P75    : %s ms\n" "$P75"
printf "P90    : %s ms\n" "$P90"
