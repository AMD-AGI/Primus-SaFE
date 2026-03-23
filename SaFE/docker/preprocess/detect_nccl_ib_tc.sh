#!/bin/sh
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# POSIX sh; avoid bashisms (local, pipefail, (( )), &>, nested functions).
set -eu

# Only run Pensando / QoS detection for multi-node 8-GPU distributed training workloads.
# Requires env: NNODES, GPUS_PER_NODE, WORKLOAD_KIND (exported by launcher.sh / workload).
should_run_nccl_ib_tc_detect() {
	_nnodes="${NNODES:-1}"
	case "$_nnodes" in
		'' | *[!0-9]*) return 1 ;;
	esac
	[ "$_nnodes" -gt 1 ] || return 1
	[ "${GPUS_PER_NODE:-}" = "8" ] || return 1
	case "${WORKLOAD_KIND:-}" in
		PyTorchJob|UnifiedJob|TorchFT|RayJob) return 0 ;;
		*) return 1 ;;
	esac
}

if ! should_run_nccl_ib_tc_detect; then
	echo "# Skip NCCL_IB_TC detection: requires NNODES>1, GPUS_PER_NODE=8, WORKLOAD_KIND in {PyTorchJob,UnifiedJob,TorchFT,RayJob}" >&2
	exit 0
fi

# Both already set: nothing to do (launcher may eval stdout; keep stdout empty).
if [ -n "${NCCL_IB_TC:-}" ] && [ -n "${NCCL_IB_FIFO_TC:-}" ]; then
	echo "INFO: NCCL_IB_TC and NCCL_IB_FIFO_TC already set, skipping detect_nccl_ib_tc.sh" >&2
	exit 0
fi

# When USING_AINIC=1 (e.g. after in-launcher AINIC build), add 1 to each TC (40 184 -> 41 185).
emit_nccl_ib_tc_pair() {
	_emit_a="$1"
	_emit_b="$2"
	if [ "${USING_AINIC:-}" = "1" ]; then
		_emit_a=$((_emit_a + 1))
		_emit_b=$((_emit_b + 1))
	fi
	printf '%s %s\n' "$_emit_a" "$_emit_b"
}

# Args: qos blob, priority number. Prints single DSCP value or empty.
extract_dscp_for_priority() {
	_eq_qos="$1"
	_eq_prio="$2"
	printf '%s\n' "$_eq_qos" \
		| grep -v "bitmap" \
		| grep "DSCP" \
		| grep "==> priority : ${_eq_prio}\$" \
		| head -1 \
		| sed 's/.*DSCP[^:]*: *//' \
		| sed 's/ *==> .*//' \
		| tr -d ' '
}

is_pensando() {
	_ib_dev=""
	for dev in /sys/class/infiniband/*; do
		[ -e "$dev" ] || continue
		_ib_dev=$(basename "$dev")
		break
	done
	[ -z "$_ib_dev" ] && return 1

	if printf '%s\n' "$_ib_dev" | grep -qi "ionic"; then
		return 0
	fi

	_ca_type=$(ibstat "$_ib_dev" 2>/dev/null | grep "CA type:" | head -1 || true)
	printf '%s\n' "$_ca_type" | grep -qi "Pensando"
}

detect_pensando_tc() {
	if ! command -v nicctl >/dev/null 2>&1; then
		echo "WARN: nicctl not found, using known Pensando defaults" >&2
		emit_nccl_ib_tc_pair 104 192
		return 0
	fi

	_qos_output=$(nicctl show qos 2>/dev/null) || {
		echo "WARN: nicctl show qos failed, using defaults" >&2
		emit_nccl_ib_tc_pair 104 192
		return 0
	}

	_pfc_prio=$(printf '%s\n' "$_qos_output" | grep "PFC no-drop priorities" | head -1 | awk '{print $NF}')

	if [ -z "$_pfc_prio" ]; then
		echo "WARN: Could not determine PFC priority, using defaults" >&2
		emit_nccl_ib_tc_pair 104 192
		return 0
	fi

	_data_dscp=$(extract_dscp_for_priority "$_qos_output" "$_pfc_prio")

	if ! printf '%s\n' "$_data_dscp" | grep -qE '^[0-9]+$'; then
		echo "WARN: Could not parse DSCP for PFC priority $_pfc_prio, using defaults" >&2
		emit_nccl_ib_tc_pair 104 192
		return 0
	fi

	_strict_prio=$(printf '%s\n' "$_qos_output" | grep -i "strict" | head -1 | awk '{print $1}')
	_fifo_dscp=""
	if [ -n "$_strict_prio" ] && printf '%s\n' "$_strict_prio" | grep -qE '^[0-9]+$'; then
		_fifo_dscp=$(extract_dscp_for_priority "$_qos_output" "$_strict_prio")
	fi

	if ! printf '%s\n' "$_fifo_dscp" | grep -qE '^[0-9]+$'; then
		echo "WARN: Could not find strict-priority DSCP, using same as data" >&2
		_fifo_dscp="$_data_dscp"
	fi

	emit_nccl_ib_tc_pair "$((_data_dscp * 4))" "$((_fifo_dscp * 4))"
}

if ! is_pensando; then
	echo "# Not a Pensando AINIC cluster, no NCCL_IB_TC override needed" >&2
	exit 0
fi

_result=$(detect_pensando_tc)
_merged=$(printf '%s\n' "$_result" | tail -n 1)
if ! printf '%s\n' "$_merged" | grep -qE '^[0-9]+[[:space:]]+[0-9]+$'; then
	echo "WARN: detect_nccl_ib_tc: invalid TC pair from detection" >&2
	exit 0
fi
_v1=$(printf '%s\n' "$_merged" | awk '{print $1}')
_v2=$(printf '%s\n' "$_merged" | awk '{print $2}')
# Stdout must be eval-safe in launcher: only export lines for variables still unset.
if [ -z "${NCCL_IB_TC:-}" ]; then
	printf 'export NCCL_IB_TC=%s\n' "$_v1"
fi
if [ -z "${NCCL_IB_FIFO_TC:-}" ]; then
	printf 'export NCCL_IB_FIFO_TC=%s\n' "$_v2"
fi
echo "INFO: detect_nccl_ib_tc: NCCL_IB_TC=${NCCL_IB_TC:-$_v1} NCCL_IB_FIFO_TC=${NCCL_IB_FIFO_TC:-$_v2}" >&2
exit 0
