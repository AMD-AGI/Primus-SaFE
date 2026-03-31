#!/bin/bash
# Parallel wget speed test - saturates the link with multiple concurrent downloads.
# Usage: ./speedtest_parallel.sh [JOBS] [ROUNDS]
#   JOBS  = number of parallel wget processes (default: 4)
#   ROUNDS = how many times to run a full set of parallel downloads (default: 3)

SPEEDTEST_URL="${SPEEDTEST_URL:-http://speedtest.newark.linode.com/100MB-newark.bin}"
SPEEDTEST_THREADS_PER_NODE="${SPEEDTEST_THREADS_PER_NODE:-1}"
SPEEDTEST_ROUNDS="${SPEEDTEST_ROUNDS:-10}"
SPEEDTEST_TARGET_IP="${SPEEDTEST_TARGET_IP:-50.116.57.237}"


echo "Speed test: $SPEEDTEST_THREADS_PER_NODE parallel jobs × $SPEEDTEST_ROUNDS rounds — $SPEEDTEST_URL"
echo "---"

total_bytes=0
start=$(date +%s.%N)

for round in $(seq 1 "$SPEEDTEST_ROUNDS"); do
  pids=()
  for j in $(seq 1 "$SPEEDTEST_THREADS_PER_NODE"); do
    wget -q --timeout=300 --connect-timeout=30 -O /dev/null "$SPEEDTEST_URL" &
    pids+=($!)
  done
  for pid in "${pids[@]}"; do
    wait "$pid"
  done
  # 100MB per successful download
  total_bytes=$((total_bytes + SPEEDTEST_THREADS_PER_NODE * 100 * 1024 * 1024))
  #echo "Round $round completed at: $(date +%H:%M:%S)"
done

end=$(date +%s.%N)
elapsed=$(awk "BEGIN { printf \"%.2f\", $end - $start }")
total_mb=$(awk "BEGIN { printf \"%.2f\", $total_bytes / 1024 / 1024 }")
mbps=$(awk "BEGIN { printf \"%.2f\", $total_bytes * 8 / 1024 / 1024 / $elapsed }")

echo "Downloaded: ${total_mb} MB in ${elapsed}s"
echo "Average speed: ${mbps} Mbps"

