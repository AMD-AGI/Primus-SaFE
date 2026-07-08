# SaFE observability exporters

Node-level metric exporters vendored into Primus-SaFE so the observability
stack does **not** depend on the (not-yet-public) Primus-Robust image pipeline.
They are the same exporters Robust runs, so the metric names/labels are
identical and switching back to Primus-Robust later is a drop-in.

| Exporter | eBPF? | In go.work? | Built via |
|----------|-------|-------------|-----------|
| `gpu-exporter` | no | yes | plain Go (`go build`) — produces `gpu_utilization`, `gpu_socket_power_watts`, `gpu_pcie_bandwidth_mbs`, `gpu_temperature_*`, `gpu_memory_used_percent`, XGMI |
| `rdma-exporter` | yes | no | its `Dockerfile` (eBPF/clang toolchain) — produces `rdma_comm_tx_bytes_total`, `rdma_comm_tx_ops_total`, `rdma_qp_*` |
| `network-exporter` | yes | no | its `Dockerfile` (eBPF/clang toolchain) — TCP/IP flow metrics |

## Source move notes

- Module paths were rewritten from
  `github.com/AMD-AGI/Primus-Robust-Internal/tools/lens/*` to
  `github.com/AMD-AIG-AIMA/SAFE/observability-exporters/*`.
- `gpu-exporter` has no cgo/eBPF, so it is in `go.work` and builds with the rest
  of the workspace.
- `rdma-exporter` and `network-exporter` are eBPF-based (cgo + `bpf2go`), so they
  stay standalone modules (not in `go.work`) and are built only via their
  `Dockerfile`s, which carry the clang/libbpf toolchain — same as upstream.

## Images (remaining CI step)

The `primus-safe-observability` umbrella currently references the
`primussafe/<exporter>:latest` images. Now that the source lives here, SaFE CI
should build and push these from this directory so there is no runtime
dependency on Robust's registry. Deploy charts live in
`charts/primus-safe-observability/charts/<exporter>`.
