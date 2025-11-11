# Network Exporter

## Overview

The Network Exporter is a high-performance network monitoring module within the Primus-Lens system that leverages eBPF (Extended Berkeley Packet Filter) technology to capture and analyze TCP network traffic in Kubernetes clusters. It provides real-time visibility into network flows, connections, and performance metrics with minimal overhead.

## Features

- **eBPF-Based Monitoring**: Kernel-level network tracing with minimal performance impact
- **TCP Connection Tracking**: Monitors TCP connection establishment and termination events
- **TCP Flow Analysis**: Captures TCP flow data including packet sizes, RTT (Round-Trip Time), and throughput
- **Network Policy Enforcement**: IP address classification based on configurable policies
- **Traffic Direction Detection**: Automatically determines ingress/egress traffic and inbound/outbound direction
- **Prometheus Metrics Export**: Exposes network metrics in Prometheus format
- **Kubernetes-Aware**: Understands Kubernetes pod/service networks and internal hosts
- **Multi-Architecture Support**: Built for both AMD64 and ARM64 architectures
- **IPv4/IPv6 Support**: Handles both IPv4 and IPv6 network protocols
- **DNS Traffic Detection**: Identifies and tracks DNS-related traffic

## Architecture

### Core Components

#### 1. eBPF Programs (`pkg/bpf`)

##### TCP Connection Monitor (`tcpconn`)
- **Kernel Hooks**: 
  - `kprobe/tcp_close`: Captures TCP connection close events
  - `kprobe/tcp_connect`: Captures TCP connection establishment events
- **Data Captured**: Process ID, source/destination IPs, ports, address family
- **Event Types**: Connect and close events

##### TCP Flow Monitor (`tcpflow`)
- **Kernel Hook**: 
  - `tracepoint/tcp/tcp_probe`: Captures TCP packet transmission events
- **Data Captured**: Addresses, ports, data length, RTT, congestion window, socket state
- **Filtering**: Skips packets with zero data length

#### 2. Event Handler (`pkg/exporter`)

The handler orchestrates the entire network monitoring pipeline:

- **Event Collection**: Reads events from eBPF ring buffers
- **Direction Detection**: Determines traffic direction based on local IPs and listening ports
- **Policy Matching**: Classifies remote addresses using IP range policies
- **Metrics Aggregation**: Aggregates flow data and connection counts
- **Metrics Export**: Exposes data via Prometheus metrics

#### 3. Policy Manager (`pkg/policy`)

Manages IP address classification policies:

- **K8s Pod CIDR**: Kubernetes pod network ranges
- **K8s Service CIDR**: Kubernetes service network ranges
- **DNS Servers**: DNS server IP addresses
- **Internal Hosts**: Private network ranges (RFC 1918)
- **Localhost**: Loopback addresses
- **Docker Networks**: Docker bridge network ranges
- **Abnormal Flow Lists**: Blacklist/whitelist for anomaly detection

## Data Flow

```
┌─────────────────────────────────────────┐
│          Kernel Space                   │
│  ┌────────────────────────────────────┐ │
│  │ eBPF Programs                      │ │
│  │  - tcp_connect (kprobe)            │ │
│  │  - tcp_close (kprobe)              │ │
│  │  - tcp_probe (tracepoint)          │ │
│  └─────────────┬──────────────────────┘ │
│                │ Ring Buffer             │
└────────────────┼─────────────────────────┘
                 │
┌────────────────▼─────────────────────────┐
│          User Space                      │
│  ┌────────────────────────────────────┐  │
│  │ Event Readers                      │  │
│  │  - syncTcpConn()                   │  │
│  │  - syncTcpFlow()                   │  │
│  └─────────────┬──────────────────────┘  │
│                │                          │
│  ┌─────────────▼──────────────────────┐  │
│  │ Event Processors                   │  │
│  │  - consumeTcpConn()                │  │
│  │  - consumeTcpFlow()                │  │
│  └─────────────┬──────────────────────┘  │
│                │                          │
│  ┌─────────────▼──────────────────────┐  │
│  │ Aggregation & Cache                │  │
│  │  - tcpFlowCache (5s interval)      │  │
│  │  - tcpConnCache (5s interval)      │  │
│  └─────────────┬──────────────────────┘  │
│                │                          │
│  ┌─────────────▼──────────────────────┐  │
│  │ Metrics Generation                 │  │
│  │  - Direction Detection             │  │
│  │  - Policy Matching                 │  │
│  │  - Metrics Cache (10min TTL)       │  │
│  └─────────────┬──────────────────────┘  │
│                │                          │
│  ┌─────────────▼──────────────────────┐  │
│  │ Prometheus Metrics                 │  │
│  │  - /metrics endpoint               │  │
│  └────────────────────────────────────┘  │
└──────────────────────────────────────────┘
```

## Metrics

### Exported Prometheus Metrics

#### TCP Flow Egress
```
primus_lens_network_tcp_flow_egress{raddr="<remote_addr>",rport="<remote_port>",direction="<inbound|outbound>",type="<ip_source_type>"}
```
Tracks outbound TCP traffic to external hosts.

#### TCP Flow Ingress
```
primus_lens_network_tcp_flow_ingress{lport="<local_port>",raddr="<remote_addr>",direction="<inbound|outbound>",type="<ip_source_type>"}
```
Tracks inbound TCP traffic from external hosts.

#### Kubernetes Internal Traffic
```
primus_lens_network_k8s_tcp_flow{type="<k8sPod|k8sSvc>"}
```
Tracks TCP traffic within the Kubernetes cluster (pod-to-pod, pod-to-service).

#### DNS Traffic
```
primus_lens_network_flow_dns
```
Tracks DNS-related traffic.

#### TCP Round-Trip Time (RTT)
```
primus_lens_network_tcp_flow_rtt{raddr="<remote_addr>",direction="<inbound|outbound>"}
```
Histogram of TCP RTT measurements (in microseconds).

**RTT Buckets**: 50, 100, 200, 300, 500, 700, 1000, 1300, 1600, 2000, 2500, 3000, 4000, 5000, 7000, 10000 µs

### IP Source Types

- `k8sPod`: Kubernetes pod IP
- `k8sSvc`: Kubernetes service IP
- `dns`: DNS server
- `localhost`: Loopback address
- `docker`: Docker network
- `internalHosts`: Internal/private network
- `externalHosts`: External/public network
- `abnormalFlowBlackList`: Blacklisted IP
- `abnormalFlowWhiteList`: Whitelisted IP
- `unknown`: Unclassified
- `error`: Classification error

## Configuration

### Network Policy

The module automatically loads the default network policy from Kubernetes cluster configuration:

```go
type NetworkPolicy struct {
    InternalHosts     []string // RFC 1918 private networks
    K8SPod            []string // Kubernetes pod CIDR
    K8SSvc            []string // Kubernetes service CIDR
    Dns               []string // DNS server IPs
    AbnormalBlackList []string // Blacklist CIDRs
    AbnormalWhiteList []string // Whitelist CIDRs
    Localhost         []string // Loopback CIDRs
}
```

**Default Internal Hosts**:
- `10.0.0.0/8`
- `172.16.0.0/12`
- `192.168.0.0/16`

### eBPF Configuration

- **Ring Buffer Size**: 16 MB (2^24 bytes) per eBPF program
- **Event Channel Size**: 409,600 events
- **Flush Interval**: 5 seconds (temporary cache)
- **Metrics Refresh Interval**: 15 seconds
- **Port Scan Interval**: Configurable via `Netflow.ScanPortListenInterval`

## API Endpoints

### Debug Endpoints

#### GET /net-flow/tcp-listen
Returns the list of TCP ports currently in LISTEN state on the node.

**Response**:
```json
{
  "code": 0,
  "data": [80, 443, 8080, 10250],
  "message": "success"
}
```

#### GET /net-flow/tcp-file
Returns the raw contents of `/proc/net/tcp` for debugging purposes.

## Traffic Direction Logic

The module uses a sophisticated algorithm to determine traffic direction:

1. **Identify Local vs Remote**: Check if source/destination IPs are local
2. **Check Listening Ports**: Determine if source/destination ports are in LISTEN state
3. **Classify Traffic Type**:
   - **Ingress**: At least one side is listening (server-side traffic)
   - **Egress**: No listening ports involved (client-side traffic)
4. **Determine Direction**:
   - **Inbound**: Remote → Local
   - **Outbound**: Local → Remote

## Installation

### Prerequisites

- Linux kernel 5.8+ (for eBPF support)
- Kernel headers installed
- CAP_BPF, CAP_PERFMON, or CAP_SYS_ADMIN capabilities
- Go 1.24.5 or higher
- Clang/LLVM (for eBPF compilation)
- Kubernetes cluster

### Build

```bash
cd Lens/modules/exporters/network-exporter

# Build eBPF programs
export CFLAGS="-I../../../include"
go generate ./pkg/bpf/tcpconn
go generate ./pkg/bpf/tcpflow

# Build the exporter
go build -o network-exporter ./cmd/network-exporter
```

### Deployment

The module is typically deployed as a DaemonSet in Kubernetes to run on every node:

**Required Capabilities**:
- `CAP_BPF` (Linux 5.8+)
- `CAP_PERFMON` (Linux 5.8+)
- `CAP_NET_ADMIN` (for network inspection)
- Or `CAP_SYS_ADMIN` (legacy)

**Required Host Mounts**:
- `/host-proc/net/tcp` → `/proc/net/tcp` (for listening port detection)
- `/sys/kernel/debug` (for eBPF program loading)

**Example DaemonSet snippet**:
```yaml
securityContext:
  capabilities:
    add:
      - BPF
      - PERFMON
      - NET_ADMIN
  privileged: false
volumeMounts:
  - name: proc
    mountPath: /host-proc
    readOnly: true
  - name: sys-kernel-debug
    mountPath: /sys/kernel/debug
volumes:
  - name: proc
    hostPath:
      path: /proc
  - name: sys-kernel-debug
    hostPath:
      path: /sys/kernel/debug
```

## How It Works

### 1. eBPF Program Loading

On initialization:
- Loads eBPF bytecode into the kernel
- Attaches kprobes to `tcp_connect` and `tcp_close` functions
- Attaches tracepoint to `tcp/tcp_probe` event
- Creates ring buffers for event communication

### 2. Event Collection

- eBPF programs capture TCP events in kernel space
- Events are written to ring buffers (non-blocking)
- User-space readers continuously poll ring buffers
- Events are decoded and sent to processing channels

### 3. Event Processing

**TCP Connection Events**:
- Track connection establishment (`connect`)
- Track connection termination (`close`)
- Aggregate connection counts per flow

**TCP Flow Events**:
- Capture packet data length
- Capture RTT measurements
- Aggregate total bytes and RTT per flow

### 4. Aggregation & Flushing

- Events are temporarily cached (5-second window)
- Cache is flushed every 5 seconds
- Flows are classified by direction and remote address type
- Metrics are updated in the metrics cache

### 5. Metrics Export

- Prometheus metrics are refreshed every 15 seconds
- Metrics cache has a 10-minute TTL
- Scrapers collect metrics via `/metrics` endpoint

## Performance Considerations

### eBPF Overhead
- **Minimal CPU impact**: < 1% CPU overhead on typical workloads
- **No packet copying**: Zero-copy event collection
- **Kernel-space filtering**: Reduces data transferred to user space

### Event Buffering
- **Large ring buffers**: 16 MB prevents event loss under load
- **Buffered channels**: 409,600 event capacity
- **Drop metrics**: Track dropped events when buffers are full

### Aggregation Strategy
- **5-second batching**: Reduces per-event processing overhead
- **In-memory caching**: Fast lookups without database queries
- **TTL-based expiry**: Automatic cleanup of stale entries

## Troubleshooting

### eBPF Program Load Failures

**Error**: `failed to load bpf objects`

**Causes**:
- Missing kernel headers
- Insufficient capabilities
- Kernel version too old (< 5.8)

**Solutions**:
```bash
# Check kernel version
uname -r

# Install kernel headers (Ubuntu/Debian)
apt-get install linux-headers-$(uname -r)

# Install kernel headers (RHEL/CentOS)
yum install kernel-devel-$(uname -r)

# Verify eBPF support
bpftool feature probe
```

### High Event Drop Rate

**Symptom**: `primus_lens_bpf_event_chan_drop` metric is increasing

**Causes**:
- Event channel buffer full
- Processing goroutines blocked
- High network traffic volume

**Solutions**:
- Increase channel buffer size in `InitChan()`
- Reduce flush interval
- Scale horizontally (more nodes)

### Missing Metrics

**Symptom**: No metrics for certain flows

**Causes**:
- Traffic filtered by policy (K8s internal, DNS, etc.)
- Zero data length packets
- Local-to-local traffic

**Solutions**:
- Check policy configuration
- Verify IP ranges in network policy
- Use debug endpoints to inspect listening ports

### Permission Denied

**Error**: `operation not permitted`

**Solution**: Ensure the pod has required capabilities:
```yaml
securityContext:
  capabilities:
    add:
      - BPF
      - PERFMON
      - NET_ADMIN
```

## Limitations

1. **TCP Only**: Does not monitor UDP, ICMP, or other protocols
2. **No Payload Inspection**: Only captures metadata (no packet payloads)
3. **Kernel Dependency**: Requires Linux kernel 5.8+ for unprivileged eBPF
4. **Node-Level Visibility**: Deployed per-node, not per-pod
5. **RTT Measurement**: Based on kernel's SRTT estimate, not actual measurement

## Dependencies

### Core Dependencies
- `github.com/cilium/ebpf`: eBPF library for Go
- `github.com/yl2chen/cidranger`: CIDR range matching
- `github.com/AMD-AGI/Primus-SaFE/Lens/core`: Core Primus-Lens functionality
- `k8s.io/api`: Kubernetes API types
- `sigs.k8s.io/controller-runtime`: Kubernetes controller framework

### Build-Time Dependencies
- Clang/LLVM
- Linux kernel headers
- `bpf2go` code generator

## Contributing

This module is part of the AMD-AGI Primus-SaFE project. For contributions, please follow the project's contribution guidelines.

## Security Considerations

- eBPF programs are verified by the kernel verifier for safety
- No arbitrary code execution possible
- Ring buffers have bounded memory usage
- Metrics do not expose sensitive payload data
- Network policies prevent leakage of internal traffic patterns

## Future Enhancements

- UDP and ICMP protocol support
- Application-layer protocol detection (HTTP, gRPC, etc.)
- Network policy anomaly detection
- Historical flow analysis and storage
- Integration with service mesh metrics
- Enhanced DNS query tracking

## License

This module is part of the Primus-SaFE project and follows the project's licensing terms.

## Related Modules

- **Primus-Lens Core**: Provides base functionality and shared libraries
- **GPU Resource Exporter**: Tracks GPU resource allocation
- **Telemetry Processor**: Processes and aggregates metrics from exporters
- **System Tuner**: Uses network metrics for optimization decisions

