#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1

errors=""
for script in *.sh; do
  if [[ "$script" == "run.sh" ]]; then
    continue
  fi
  if [[ "$TEST_LEVEL" != "FULL" ]]; then
    case "$script" in
      "rvs_performance.sh"|"rvs_p2p_throughput.sh"|"transfer_bench_p2p.sh")
        echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Skipping $script (requires TEST_LEVEL=FULL)"
        continue
        ;;
    esac
  fi
  echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Executing script: $script"
  output=$(timeout --signal=TERM --kill-after=3s 900s bash "$script" 2>&1)
  exit_code=$?
  last_line=""
  if [ -n "$output" ]; then
    while IFS= read -r line; do
      echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [${script}] $line"
      last_line=$(echo "$line" | tr -d '\n')
    done <<< "$output"
  fi

  if [ $exit_code -eq 0 ]; then
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [${script}] [SUCCESS] tests passed"
  elif [ -n "$last_line" ]; then
    if [ -n "$errors" ]; then
      errors+=" | "
    fi
    errors+="[$(date +'%Y-%m-%d %H:%M:%S')] [$script] $last_line"
  fi
done

if [ -n "$errors" ]; then
  echo "$errors" >&2
  exit 1
fi