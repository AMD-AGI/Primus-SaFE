#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# BabelStream is a benchmarking program designed to evaluate memory bandwidth performance.

dpkg -l | grep -q openmpi-bin
if [ $? -ne 0 ]; then
  apt-get update >/dev/null && apt-get -y install openmpi-bin openmpi-common libopenmpi-dev >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR]: failed to install openmpi" >&2
    exit 1
  fi
fi

REPO_URL="https://github.com/UoB-HPC/BabelStream.git"
DIR_NAME="BabelStream"
if [ ! -d "$DIR_NAME" ]; then
  dpkg -l | grep -q git
  if [ $? -ne 0 ]; then
    apt-get update >/dev/null && apt-get -y install git >/dev/null
    if [ $? -ne 0 ]; then
      echo "[ERROR]: failed to install git" >&2
      exit 1
    fi
  fi

  git clone "$REPO_URL" >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR]: failed to clone $REPO_URL" >&2
    exit 1
  fi
fi
cd "$DIR_NAME" || { echo "[ERROR]: unable to access $DIR_NAME" >&2; exit 1; }

if [ ! -f build/hip-stream ]; then
  dpkg -l | grep -q cmake
  if [ $? -ne 0 ]; then
    apt-get update >/dev/null && apt-get -y install cmake >/dev/null
    if [ $? -ne 0 ]; then
      echo "[ERROR]: failed to install cmake" >&2
      exit 1
    fi
  fi

  cmake -Bbuild -H. -DMODEL=hip -DCMAKE_CXX_COMPILER=hipcc >/dev/null && cmake --build build >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR]: failed to make hip-stream" >&2
    exit 1
  fi
fi

if [ ! -f wrapper.sh ]; then
  echo '#!/bin/bash
# Use the mpirank to manage the device:
./build/hip-stream --device $OMPI_COMM_WORLD_RANK -n 50 -s 268435456' > wrapper.sh
  chmod u+x wrapper.sh
fi

LOG_FILE="/tmp/babel_stream.log"
mpiexec -n 8 --allow-run-as-root wrapper.sh >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[ERROR]: mpiexec failed with exit code: $EXIT_CODE" >&2
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
    echo "[BabelStream] [INFO] $func average: $formatted_avg > $threshold"
  else
    echo "[BabelStream] [ERROR] $func average: $formatted_avg <= $threshold" >&2
    all_passed=1
  fi
done
rm -f $LOG_FILE
if [[ "$all_passed" -eq 1 ]]; then
  exit 1
fi
echo "[BabelStream] [SUCCESS] tests passed"
exit 0