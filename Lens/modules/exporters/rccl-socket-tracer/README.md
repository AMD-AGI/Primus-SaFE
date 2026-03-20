# rccl-socket-tracer

eBPF-based tool for diagnosing RCCL/NCCL bootstrap socket connection failures in distributed GPU training.

## What it traces

| Layer | Probes | What it catches |
|-------|--------|-----------------|
| **L1: TCP lifecycle** | `inet_sock_set_state`, `tcp_reset`, `tcp_retransmit_skb` | TCP state transitions (SYN_SENT→CLOSE), RST packets, retransmissions |
| **L2: Syscall latency** | `sys_enter/exit_connect`, `accept4`, `sendto`, `recvfrom` | connect() timeout/refused, accept() failures, slow I/O |
| **L3: RCCL** | (optional uprobe on librccl.so) | RCCL-level connect/accept correlation |

## Output format

Per-pod, per-PID trace files:

```
/var/log/rccl-tracer/
  <pod-name>/
    trace-pid386.log    # container PID 386 (local_rank 0)
    trace-pid387.log    # container PID 387 (local_rank 1)
    ...
```

Each line:
```
[06:07:46.123] CONNECT        pid=12345 tid=12345 comm=python3 dst=10.158.170.213:50182
[06:07:46.234] CONNECT_DONE   pid=12345 tid=12345 comm=python3 duration=111.0ms errno=111(ECONNREFUSED)
[06:07:46.345] TCP_STATE      pid=12345 tid=12345 comm=python3 10.158.160.1:45678→10.158.170.213:50182 SYN_SENT→CLOSE
[06:07:46.456] TCP_RETX       pid=12345 tid=12345 comm=python3 10.158.160.1:45678→10.158.170.213:50182
```

## Quick start

```bash
# Build and push
make build push

# Deploy to all GPU nodes
make deploy

# Check trace files on a specific node
make logs NODE=uswslocpm2m-106-2111

# Collect traces from a node after a failure
make collect NODE=uswslocpm2m-106-2111 DEST=./traces
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `TRACE_OUTPUT_DIR` | `/var/log/rccl-tracer` | Where to write per-pod trace files |
| `TRACE_NAMESPACE_FILTER` | `control-plane-` | Only trace pods in namespaces containing this substring |
| `TRACE_SLOW_THRESHOLD_MS` | `100` | Report send/recv calls slower than this |

## How to read failure traces

After a training job fails, find the first-to-fail pod from OpenSearch, then check its trace:

```bash
# Example: worker-22 on node 106-1066 failed first
make collect NODE=uswslocpm2m-106-1066 DEST=./traces

# Look at the pod's traces
ls traces/uswslocpm2m-106-1066/primus-dsv3-perf-26ktv-worker-22/

# Find the connect failures
grep "ECONNREFUSED\|ETIMEDOUT\|TCP_RESET\|CLOSE" traces/.../trace-pid386.log
```

What to look for:
- `CONNECT_DONE ... ECONNREFUSED` → remote port not listening yet (race condition)
- `CONNECT_DONE ... ETIMEDOUT` → remote host unreachable or AINIC issue
- `TCP_STATE ... SYN_SENT→CLOSE` → connection attempt failed
- `TCP_RETX` → packet loss on the network path
- `TCP_RESET` → remote side actively rejected
- `SEND_SLOW / RECV_SLOW` → I/O stall during RCCL handshake
