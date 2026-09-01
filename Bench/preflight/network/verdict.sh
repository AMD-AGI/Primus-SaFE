#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Retry bookkeeping and the final verdict for the network diagnosis.
#
# Split out of run.sh so the decision can be exercised without running a
# diagnosis -- see tests/test_verdict.sh. Sourcing this file runs nothing.
#
# Everything here turns on one idea: a *comparable* run. A run that ends with
# a test failure nobody can be blamed for did not finish. Some nodes were
# never tested, so its blame set says nothing about them, and intersecting it
# against the other runs is not a comparison -- it is a deletion. That is what
# made the retry actively harmful:
#
#   run 1: harness error, nobody blamed  -> seeded the intersection with {}
#   run 2: 10.0.0.7 fails all_reduce     -> nothing could be added any more
#   verdict: "All diagnosis tests passed"
#
# The retry found the bad node and the bookkeeping threw the answer away --
# the exact opposite of why the retry exists. So an unfinished run takes part
# in neither the seeding nor the intersecting. It is recorded separately and
# reported separately, because "these nodes are bad" and "the check did not
# finish" are two different things and the operator needs both.

# Nodes blamed by every comparable run.
#
# The `=()` is not decoration. `declare -A x` on its own leaves x unbound as
# far as `set -u` is concerned, so ${#x[@]} on a still-empty intersection --
# the state every clean run ends in -- aborts the shell. run.sh does not run
# under set -u today, which is the only reason this never showed.
declare -A unhealthy_nodes_intersection=()
# Whether any comparable run has seeded the intersection. Distinct from "the
# intersection is empty": no comparable run at all is not a clean bill of
# health, it is the absence of a result, and it must not print SUCCESS.
intersection_seeded=0
# Which runs failed without blaming a node. Cross-run on purpose: the
# per-run harness_failure flag is reset every run so a retry can disprove it,
# and that reset is precisely what left it unreadable at the end.
harness_failure_runs=()
# Nodes an *unfinished* run positively blamed. Kept because "this run did not
# finish" is a reason to distrust its silence, never a reason to discard what
# it did observe: a node that failed a test failed it. They are held apart
# from the intersection so they cannot short-circuit the confirmation the
# retries exist to provide, and are used only when no run ever completed --
# without that, a cluster where every retry both blamed a node and hit an
# unattributable failure printed no "Final unhealthy nodes" line at all, and
# Bench/run.sh handed the confirmed-bad node to the benchmark.
declare -A blamed_by_unfinished_runs=()

reset_verdict_state() {
  unset unhealthy_nodes_intersection
  declare -gA unhealthy_nodes_intersection
  unhealthy_nodes_intersection=()
  intersection_seeded=0
  harness_failure_runs=()
  unset blamed_by_unfinished_runs
  declare -gA blamed_by_unfinished_runs
  blamed_by_unfinished_runs=()
}

# record_run <run number> <harness_failure 0|1> [blamed node ...]
#
# Fold one run into the verdict state.
record_run() {
  local run="$1" harness_failure="$2"
  shift 2

  local node
  if [ "$harness_failure" -eq 1 ]; then
    harness_failure_runs+=("$run")
    # Not comparable -- see the header. This deliberately also keeps a run
    # that blamed some nodes *and* hit an unattributable failure out of the
    # intersection: the nodes it did not reach are absent from its blame set
    # for want of a test, not for want of a fault, and removing them on that
    # basis is the same bug in a smaller costume.
    #
    # Its positive findings are still kept, just not as confirmations.
    for node in "$@"; do
      blamed_by_unfinished_runs["$node"]=1
    done
    return
  fi

  if [ "$intersection_seeded" -eq 0 ]; then
    intersection_seeded=1
    for node in "$@"; do
      unhealthy_nodes_intersection["$node"]=1
    done
    return
  fi

  local -A blamed=()
  for node in "$@"; do
    blamed["$node"]=1
  done
  for node in "${!unhealthy_nodes_intersection[@]}"; do
    if [ -z "${blamed[$node]:-}" ]; then
      unset 'unhealthy_nodes_intersection[$node]'
    fi
  done
}

# print_verdict <log prefix>
#
# Print the verdict lines. Returns 0 only when a comparable run finished and
# blamed nobody. The three outcomes are independent, not a chain of elifs:
# a run that blamed nodes can also have been preceded by one that did not
# finish, and swallowing either half loses information the operator acts on.
print_verdict() {
  local prefix="$1"
  local ret=0
  local node

  # Which set of nodes the "Final unhealthy nodes" line reports.
  #
  # Normally the intersection: a node is condemned only once every run that
  # finished agreed on it, which is what the retries are for. When no run
  # finished there is no confirmation to be had, so fall back to everything
  # the unfinished runs actually observed failing. That is deliberately the
  # union and not an intersection -- with no working confirmation mechanism,
  # the conservative direction is to report more, and each of these nodes did
  # fail a test that ran.
  local -a blamed=()
  if [ "$intersection_seeded" -eq 1 ]; then
    for node in "${!unhealthy_nodes_intersection[@]}"; do
      blamed+=("$node")
    done
  else
    for node in "${!blamed_by_unfinished_runs[@]}"; do
      blamed+=("$node")
    done
  fi

  if [ ${#blamed[@]} -gt 0 ]; then
    ret=1
    local -a quoted=()
    for node in "${blamed[@]}"; do
      quoted+=("'$node'")
    done
    local joined="${quoted[0]}"
    local i
    for ((i = 1; i < ${#quoted[@]}; i++)); do
      joined+=", ${quoted[i]}"
    done
    echo "${prefix}[NETWORK] [ERROR] Final unhealthy nodes: [${joined}]"
  fi

  if [ "$intersection_seeded" -eq 0 ]; then
    ret=1
    echo "${prefix}[NETWORK] [ERROR] ❌ Diagnosis did not complete: no run finished without hitting a test failure that blamed no node. Any node listed above did fail a test, but nothing confirmed it and the rest of the cluster is unvalidated. See the log above."
  elif [ ${#harness_failure_runs[@]} -gt 0 ]; then
    # Reported even when nodes were blamed above. The blamed nodes come from
    # the runs that did finish; this says which ones did not, and therefore
    # which nodes went untested in them.
    echo "${prefix}[NETWORK] [WARNING] Run(s) ${harness_failure_runs[*]} hit a test failure that blamed no node. A later run completed, and the verdict above is that run's."
  fi

  if [ "$ret" -eq 0 ]; then
    echo "${prefix}[NETWORK] [SUCCESS] ✅ All diagnosis tests passed."
  fi

  return $ret
}
