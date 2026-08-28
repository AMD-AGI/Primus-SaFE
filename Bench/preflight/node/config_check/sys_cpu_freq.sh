#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# The check runs as a *shell* inside the host namespaces, fed over stdin.
#
# It used to be a bare `nsenter ... -- grep -q performance \
# /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor`, which had two bugs:
#
#  1. nsenter execs grep directly -- there is no shell on the host side -- so
#     the glob was expanded by *this container's* shell against the container's
#     /sys. Containers frequently cannot see cpu*/cpufreq at all; when that
#     happens the pattern stays unexpanded, grep is handed a literal file named
#     "cpu*", exits 2, and the check reports the governor as wrong. A correctly
#     configured host was indistinguishable from a misconfigured one.
#
#  2. `grep -q` succeeds if *any* of the listed files matches, so a node with
#     one core in performance and the rest in powersave passed.
#
# Feeding the script over stdin (rather than `sh -c '...'`) keeps it readable:
# no nested-quote escaping, and the glob is expanded by the host's shell.
# Plain POSIX sh -- the host is not guaranteed to have bash.

nsenter --target 1 --mount --uts --ipc --net --pid -- sh <<'REMOTE'
total=0
bad=0
seen=""
for f in /sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_governor; do
  [ -e "$f" ] || continue        # POSIX sh leaves an unmatched glob literal
  total=$((total + 1))
  g=$(cat "$f" 2>/dev/null)
  [ "$g" = "performance" ] && continue
  bad=$((bad + 1))
  case " $seen " in
    *" $g "*) ;;
    *) seen="$seen $g" ;;
  esac
done

if [ "$total" -eq 0 ]; then
  echo "cpufreq is not available on this host (no cpu*/cpufreq/scaling_governor)"
  exit 1
fi

if [ "$bad" -gt 0 ]; then
  echo "cpufreq is not configured to 'performance' mode ($bad/$total CPUs, found:$seen)"
  exit 1
fi

exit 0
REMOTE
