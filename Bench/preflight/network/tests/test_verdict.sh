#!/usr/bin/env bash
#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Pins the retry bookkeeping in ../verdict.sh. Every case here is a verdict
# the network preflight can reach; the first two are the ones that were wrong.
#
# Run: bash tests/test_verdict.sh

set -u
cd "$(dirname "${BASH_SOURCE[0]}")"
source ../verdict.sh

fails=0

# check <name> <expected ret> <expected substring>... -- reads the recorded
# verdict output from $out.
check() {
  local name="$1" want_ret="$2"; shift 2
  local pat
  if [ "$ret" -ne "$want_ret" ]; then
    echo "FAIL $name: ret=$ret, want $want_ret"
    echo "$out" | sed 's/^/      /'
    fails=$((fails + 1))
    return
  fi
  for pat in "$@"; do
    if ! grep -qF -- "$pat" <<<"$out"; then
      echo "FAIL $name: output does not contain: $pat"
      echo "$out" | sed 's/^/      /'
      fails=$((fails + 1))
      return
    fi
  done
  echo "ok   $name"
}

refute() {
  local name="$1" pat="$2"
  if grep -qF -- "$pat" <<<"$out"; then
    echo "FAIL $name: output should not contain: $pat"
    echo "$out" | sed 's/^/      /'
    fails=$((fails + 1))
  fi
}

verdict() { out=$(print_verdict ""); ret=$?; }

# --- the regression this file exists for -------------------------------------
# run 1 fails with nobody to blame, run 2 finds the bad node. Seeding the
# intersection from run 1 made run 2's answer unaddable, and the run-scoped
# harness flag was clear by then, so this printed "All diagnosis tests passed"
# and handed 10.0.0.7 to the benchmark.
reset_verdict_state
record_run 1 1
record_run 2 0 "10.0.0.7"
verdict
check "harness failure in run 1 does not swallow run 2's finding" 1 \
  "Final unhealthy nodes: ['10.0.0.7']"
refute "harness failure in run 1 does not swallow run 2's finding" "All diagnosis tests passed"

# --- no run ever finished ----------------------------------------------------
reset_verdict_state
record_run 1 1
record_run 2 1
verdict
check "no comparable run is not a pass" 1 "Diagnosis did not complete"
refute "no comparable run is not a pass" "All diagnosis tests passed"

# --- a transient harness error the retry disproves ---------------------------
# The point of MAX_RETRY. Run 2 completed and blamed nobody, so this is a pass,
# but run 1 is still on the record.
reset_verdict_state
record_run 1 1
record_run 2 0
verdict
check "a retry that completes clean is a pass, with the hiccup noted" 0 \
  "All diagnosis tests passed" "Run(s) 1 hit a test failure that blamed no node"

# --- both facts get reported, not one or the other ---------------------------
# The old code reported the harness failure only in an elif under "intersection
# is empty", so a blamed node hid it completely and Bench/run.sh's
# "Diagnosis did not complete" grep could never fire alongside one.
reset_verdict_state
record_run 1 0 "10.0.0.5"
record_run 2 1
verdict
check "blamed nodes and an unfinished run are both reported" 1 \
  "Final unhealthy nodes: ['10.0.0.5']" \
  "Run(s) 2 hit a test failure that blamed no node"

# --- an unfinished run's positive findings are not thrown away ---------------
# Every retry both blamed a node and hit an unattributable failure. Nothing was
# ever comparable, so nothing seeded the intersection -- and reporting only
# "did not complete" here meant Bench/run.sh matched no node list and handed
# the node that demonstrably failed a test straight to the benchmark.
reset_verdict_state
record_run 1 1 "10.0.0.7"
record_run 2 1 "10.0.0.7"
verdict
check "a node blamed only by unfinished runs is still reported" 1   "Final unhealthy nodes: ['10.0.0.7']" "Diagnosis did not complete"

# With nothing to confirm against, report the union rather than the overlap.
reset_verdict_state
record_run 1 1 "10.0.0.7"
record_run 2 1 "10.0.0.8"
verdict
check "with no completed run the fallback is the union" 1 "Diagnosis did not complete"
check "with no completed run the fallback is the union (7)" 1 "10.0.0.7"
check "with no completed run the fallback is the union (8)" 1 "10.0.0.8"

# A completed run stays authoritative: it is the confirmation mechanism, and an
# unfinished run's blame must not slip past it.
reset_verdict_state
record_run 1 1 "10.0.0.7"
record_run 2 0
verdict
check "a completed clean run outranks an unfinished run's blame" 0 "All diagnosis tests passed"
refute "a completed clean run outranks an unfinished run's blame" "10.0.0.7"

# --- ordinary intersection behaviour -----------------------------------------
reset_verdict_state
record_run 1 0 "10.0.0.5" "10.0.0.6"
record_run 2 0 "10.0.0.5"
verdict
check "only nodes blamed by every completed run survive" 1 "Final unhealthy nodes: ['10.0.0.5']"
refute "only nodes blamed by every completed run survive" "10.0.0.6"

reset_verdict_state
record_run 1 0 "10.0.0.5"
record_run 2 0 "10.0.0.9"
verdict
check "runs that disagree exonerate both nodes" 0 "All diagnosis tests passed"

reset_verdict_state
record_run 1 0
verdict
check "a single clean run passes" 0 "All diagnosis tests passed"
refute "a single clean run passes" "WARNING"

# --- an unfinished run must not delete a finding either ----------------------
# run 2 blamed nobody because it never got far enough to test 10.0.0.5, not
# because 10.0.0.5 is well.
reset_verdict_state
record_run 1 0 "10.0.0.5"
record_run 2 1
record_run 3 0 "10.0.0.5"
verdict
check "an unfinished run neither seeds nor removes" 1 "Final unhealthy nodes: ['10.0.0.5']"

# --- nothing ran at all ------------------------------------------------------
reset_verdict_state
verdict
check "zero runs is not a pass" 1 "Diagnosis did not complete"

# --- hostnames, not just IPs -------------------------------------------------
reset_verdict_state
record_run 1 0 "crsuse2-m2m-182"
verdict
check "hostname-shaped nodes survive the round trip" 1 "Final unhealthy nodes: ['crsuse2-m2m-182']"

echo
if [ "$fails" -eq 0 ]; then
  echo "all verdict tests passed"
else
  echo "$fails verdict test(s) failed"
fi
exit $((fails > 0))
