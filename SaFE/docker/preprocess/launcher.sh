#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# When this script is container PID 1, reparented zombies (e.g. vLLM workers after python
# exits) must be reaped here. If PID 1 is sh (common with /bin/sh -c exec ...), re-exec
# under bash when available and reap on SIGCHLD; images without bash keep plain sh.
if [ "$$" -eq 1 ] && [ -z "${BASH_VERSION:-}" ]; then
  if [ -x /usr/bin/bash ]; then
    exec /usr/bin/bash "$0" "$@"
  elif [ -x /bin/bash ]; then
    exec /bin/bash "$0" "$@"
  fi
fi
if [ "$$" -eq 1 ] && [ -n "${BASH_VERSION:-}" ]; then
  trap 'while wait -n 2>/dev/null; do :; done' CHLD
fi

input="$1"

# Record the files the container started with. Saving the container as an image measures
# deletions against this record; it goes to the shared volume, which is never part of a
# saved image. An earlier container's record is removed here, before anything else, and the
# record is marked as in progress, so that saving meanwhile is refused as "still
# recording". The record itself starts after this launcher's bootstrap below: the
# bootstrap deletes and rewrites files of the image (apt-get does), and a record that
# listed them first would make saving delete them from the saved image as if the user had.
# A failure only disables saving. A workload that sets SAFE_SAVE_IMAGE_RECORD=0 skips the
# record, and cannot be saved; that is noted, so that saving it says why.
record_base=""
if [ -x /shared-data/save-image ]; then
  rm -f /shared-data/save-image.base /shared-data/save-image.run /shared-data/save-image.norecord
  case "${SAFE_SAVE_IMAGE_RECORD:-1}" in
    0|false|off)
      : > /shared-data/save-image.norecord
      ;;
    *)
      : > /shared-data/save-image.base.partial
      record_base=1
      ;;
  esac
fi

export NODE_RANK="${PET_NODE_RANK:-${NODE_RANK}}"
export NNODES="${PET_NNODES:-${NNODES}}"

# Build AINIC driver if either input is provided. build_ainic.sh accepts
# AINIC_DRIVER_VERSION and/or PATH_TO_AINIC_TAR_PACKAGE; one of them is
# sufficient (the script derives the version from the tarball filename
# when only PATH_TO_AINIC_TAR_PACKAGE is set).
if [ -n "${AINIC_DRIVER_VERSION}" ] || [ -n "${PATH_TO_AINIC_TAR_PACKAGE}" ]; then
  /bin/sh /shared-data/build_ainic.sh
  if [ $? -ne 0 ]; then
    echo "ERROR: Failed to build AINIC (AINIC_DRIVER_VERSION=${AINIC_DRIVER_VERSION:-<unset>}, PATH_TO_AINIC_TAR_PACKAGE=${PATH_TO_AINIC_TAR_PACKAGE:-<unset>}). Please check input or remove installation."
    exit 1
  fi
  export USING_AINIC=1
  echo "INFO: AINIC support enabled (USING_AINIC=1)"
fi

# Pensando AINIC: NCCL_IB_TC / NCCL_IB_FIFO_TC (logic in detect_nccl_ib_tc.sh; stdout is eval-safe export lines only).
if [ -f /shared-data/detect_nccl_ib_tc.sh ] && [ -x /bin/sh ]; then
    eval "$(/bin/sh /shared-data/detect_nccl_ib_tc.sh)" || true
fi

/bin/sh /shared-data/build_bnxt.sh
/bin/sh /shared-data/build_authoring.sh

# The record of the image's files (see above) runs in the background at the lowest CPU and
# I/O priority, so that nothing below waits for it, and writes the paths as it lists them,
# so its memory does not grow with the image. What the user changes is told apart by the
# time the entry point file is written below, not by when the record ran.
if [ -n "$record_base" ]; then
  (
    low=""
    if command -v nice >/dev/null 2>&1; then low="nice -n 19"; fi
    if command -v ionice >/dev/null 2>&1 && ionice -c 2 -n 7 true 2>/dev/null; then low="$low ionice -c 2 -n 7"; fi
    GOMAXPROCS=1 $low /shared-data/save-image record ||
      echo "WARN: LAUNCHER: cannot record the image's files; this container cannot be saved as an image" >&2
  ) &
fi

if [ -z "$input" ]; then
    exit 0
fi

# The entry point is written to the working directory. A container running as a non-root
# user may not be able to write there, so a temporary file is used instead.
run_file=".run.sh"
if ! ( : > "$run_file" ) 2>/dev/null; then
    run_file=$(mktemp "${TMPDIR:-/tmp}/run.XXXXXX" 2>/dev/null) || run_file="${TMPDIR:-/tmp}/.run.$$.sh"
fi
if ! echo "$input" | base64 -d > "$run_file"; then
    echo "ERROR: LAUNCHER: cannot write the entry point to $run_file (uid=$(id -u), cwd=$(pwd))" >&2
    exit 1
fi
chmod +x "$run_file"
# Saving the container as an image needs this file's change time as of now: record it,
# with where the file is, since it is not always the working directory's .run.sh. Its
# change time later is not the boundary: a chmod or chown of it by the user moves it.
if [ -x /shared-data/save-image ]; then
    case "$run_file" in
        /*) run_path="$run_file" ;;
        *) run_path="$(pwd)/$run_file" ;;
    esac
    /shared-data/save-image mark "$run_path" ||
        echo "WARN: LAUNCHER: cannot record where the entry point is; this container cannot be saved as an image" >&2
fi
if [ -x /usr/bin/bash ]; then
    /usr/bin/bash -o pipefail "$run_file" &
elif [ -x /bin/bash ]; then
    /bin/bash -o pipefail "$run_file" &
else
    /bin/sh "$run_file" &
fi
pid1=$!

if [ "${ENABLE_SUPERVISE}" = "true" ]; then
    chmod +x "/shared-data/run_check.sh"
    /bin/sh /shared-data/run_check.sh &
    pid2=$!
    
    while true; do
        kill -0 $pid1 2>/dev/null
        if [ $? -ne 0 ]; then
            wait $pid1
            exit_code=$?
            echo "=== LAUNCHER: run.sh exited with code $exit_code ===" >&2
            exit $exit_code
        fi

        if [ -n "$pid2" ]; then
            kill -0 $pid2 2>/dev/null
            if [ $? -ne 0 ]; then
                wait $pid2
                exit_code=$?
                if [ $exit_code -ne 0 ]; then
                    echo "=== LAUNCHER: run_check.sh exited with code $exit_code ===" >&2
                    exit $exit_code
                else
                    pid2=""
                fi
            fi
        fi
        sleep 1
    done
else
    wait $pid1
    exit_code=$?
    echo "=== LAUNCHER: run.sh exited with code $exit_code ===" >&2
    exit $exit_code
fi