#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# BabelStream is a benchmarking program designed to evaluate memory bandwidth performance.

REPO_URL="https://github.com/UoB-HPC/BabelStream.git"
cd /opt
git clone "$REPO_URL" >/dev/null
if [ $? -ne 0 ]; then
  echo "failed to clone babel_stream " >&2
  exit 1
fi

cd "./BabelStream" || exit 1
cmake -Bbuild -H. -DMODEL=hip -DCMAKE_CXX_COMPILER=hipcc >/dev/null && cmake --build build >/dev/null
if [ $? -ne 0 ]; then
  echo "failed to make babel_stream " >&2
  exit 1
fi
echo "#!/bin/bash
# Use the mpirank to manage the device:
/opt/BabelStream/build/hip-stream --device \$OMPI_COMM_WORLD_RANK -n 50 -s 268435456" > wrapper.sh
chmod u+x wrapper.sh

DIR_NAME="/opt/BabelStream"
LOG_FILE="/tmp/babel_stream.log"
/usr/bin/mpiexec -n 8 --allow-run-as-root $DIR_NAME/wrapper.sh >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "mpiexec failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

declare -A thresholds=(
  ["Copy"]=4177285
  ["Mul"]=4067069
  ["Add"]=3920853
  ["Triad"]=3885301
  ["Dot"]=3660781
)
copy_sum=0 mul_sum=0 add_sum=0 triad_sum=0 dot_sum=0
copy_count=0 mul_count=0 add_count=0 triad_count=0 dot_count=0
grep -A5 '^Function' "$LOG_FILE" | awk '
$1 == "Copy" {
  copy_sum += $2;
  copy_count += 1;
}
$1 == "Mul" {
  mul_sum += $2;
  mul_count += 1;
}
$1 == "Add" {
  add_sum += $2;
  add_count += 1;
}
$1 == "Triad" {
  triad_sum += $2;
  triad_count += 1;
}
$1 == "Dot" {
  dot_sum += $2;
  dot_count += 1;
}
END {
  if (copy_count > 0)   print "Copy "   copy_sum / copy_count;
  if (mul_count > 0)    print "Mul "    mul_sum / mul_count;
  if (add_count > 0)    print "Add "    add_sum / add_count;
  if (triad_count > 0)  print "Triad "  triad_sum / triad_count;
  if (dot_count > 0)    print "Dot "    dot_sum / dot_count;
}' | while read -r func avg; do
  formatted_avg=$(echo "$avg" | awk '{printf "%f", $1}')
  threshold=${thresholds[$func]}
  is_greater=$(echo "$formatted_avg > $threshold" | bc -l)
  if [[ "$is_greater" -eq 1 ]]; then
    echo "[INFO] $func average: $formatted_avg > $threshold"
  else
    echo "$func average($formatted_avg) < threshold($threshold)" >&2
    rm -f $LOG_FILE
    exit 1
  fi
done
rm -f $LOG_FILE