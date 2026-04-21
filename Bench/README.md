## PrimusBench

PrimusBench is a set of scripts for multi-node system preflight checks and benchmarking. It supports both bare-metal (Ansible-driven) and SLURM-based runs, covering network connectivity, node health checks, I/O benchmarks, and key system metrics such as computation-communication overlap and kernel launch overhead.

### Features
- **Preflight**: SSH reachability, node configuration, network connectivity
- **I/O Benchmarks**: fio, IOR (optional)
- **System Benchmarks**: Computation-Communication Overlap, Kernel Launch Overhead
- **Three run modes**: Bare metal via Ansible, SLURM cluster, or Kubernetes with PyTorchJob

---

## Prerequisites
- Target nodes run a basic Linux environment (Ubuntu 22.04 or equivalent recommended)
- Nodes are mutually reachable; the control node can SSH to targets (passwordless or auto-configured by scripts)
- Bare-metal mode requires Ansible on the control node; SLURM mode requires `salloc`/`srun`
- ROCm/PyTorch dependencies are typically provided by the container image

---

## Quick Start

### Running on Bare Metal

1) Prepare host inventory
Create or edit `hosts.ini` at the repo root:

```1:4:hosts.ini
[all]
host1
host2
```

Hostnames or IPs are supported; ensure they resolve.

2) Run
The entry script will:
- Install Docker across nodes via Ansible
- Start preflight checks and benchmarks, writing logs to `outputs/<TIMESTAMP>`

Common environment variables:
- `IMAGE`: Benchmark image (default: `primussafe/primusbench:202510210028`)
- `INVENTORY_FILE`: Ansible inventory file (default: `hosts.ini`)
- `IO_BENCHMARK_MOUNT`: Enable and mount a directory for I/O benchmarks (optional)

Run:
```bash
bash run_bare_metal.sh
```

Key steps in `run_bare_metal.sh`:
- Install Docker: `playbooks/bare_metal/install_docker.yaml`
- Run benchmarks: `playbooks/bare_metal/bench.yaml`
- Logs directory: `outputs/<TIMESTAMP>` (main log `primusbench.log`)

### Running on SLURM

The SLURM mode automatically handles resource allocation and benchmark execution. By default, `run_slurm.sh` will use `salloc` to allocate resources before running the benchmarks.

#### Default Usage (Automatic Allocation):
```bash
# Automatically allocate 2 nodes and run benchmarks (default behavior)
bash run_slurm.sh

# Allocate 4 nodes with custom settings
NNODES=4 PARTITION=gpu TIME=2:00:00 bash run_slurm.sh
```

#### Within Existing SLURM Allocation:
```bash
# If already in a SLURM job, skip auto-allocation
bash run_slurm.sh --no-allocate
```

#### Excluding Problem Nodes:
```bash
# Use default exclude list (configured in script)
bash run_slurm.sh

# Custom exclude list
EXCLUDE_NODES="chi[2770-2772]" bash run_slurm.sh

# Don't exclude any nodes
EXCLUDE_NODES="" bash run_slurm.sh
```

`run_slurm.sh` will:
- Automatically allocate nodes via `salloc` (unless --no-allocate is used)
- Display excluded nodes if configured
- Use `srun` to execute containers on each node
- Run node and network preflight checks
- Filter out unhealthy nodes and keep healthy ones for benchmarks
- Generate comprehensive health reports
- Write outputs to `outputs/<TIMESTAMP>`

Environment variables (configured in `config.sh`, can be overridden):
- `NNODES`: Number of nodes (default: 2)
- `PARTITION`: SLURM partition (default: configured in config.sh)
- `TIME`: Job time limit (default: 4:30:00)
- `CPUS_PER_TASK`: CPUs per task (default: 128)
- `EXCLUDE_NODES`: Nodes to exclude from allocation
- `IMAGE`: Container image for benchmarks
- `MASTER_PORT`: Master port for distributed operations
- See `config.sh` for complete configuration options

### Running on Kubernetes with PyTorchJob

For Kubernetes clusters with Kubeflow PyTorchJob operator, you can run distributed benchmarks using the provided PyTorchJob specification.

Prerequisites:
- Kubernetes cluster with [Kubeflow Training Operator](https://github.com/kubeflow/training-operator) installed
- Access to a shared storage (hostPath or PVC) for workspace and outputs
- Container image available in your cluster registry

Steps:

1) Prepare the PyTorchJob manifest
Edit `kubernetes/pytorchjob.yaml` and replace the following placeholders:
- `<host_path to be replaced, shared directory>`: Path to shared workspace directory on nodes
- `<pvc_name to be replaced， io_benchmark_pvc>`: Name of PVC for I/O benchmarks (if using PVC)
- Adjust `replicas` count for Master and Worker as needed
- Update resource limits (GPUs, memory, CPU) according to your cluster

2) Configure environment variables
Key variables in the YAML (adjust as needed):
- `NCCL_SOCKET_IFNAME` / `GLOO_SOCKET_IFNAME`: Network interface name
- `NCCL_IB_HCA`: RDMA device names (comma-separated)
- `IO_BENCHMARK_MOUNT`: Mount point for I/O benchmarks
- Container image: Update `image` field with your image tag

3) Deploy the PyTorchJob
```bash
kubectl apply -f kubernetes/pytorchjob.yaml
```

4) Monitor the job
```bash
# Check job status
kubectl get pytorchjob network

# View logs from master pod
kubectl logs -f <master-pod-name>

# View logs from worker pods
kubectl logs -f <worker-pod-name>
```

5) Collect results
Results will be written to the shared workspace directory specified in the volumeMounts.

Important notes:
- The job uses `hostNetwork: true` for optimal RDMA performance
- Privileged security context is required for device access
- Ensure all nodes have the necessary RDMA devices and drivers installed
- Worker replicas count determines the number of worker nodes (adjust based on cluster size)

---

## Network Preflight on bare metal (no container required)

The network preflight tool `preflight/network/binary_diagnose.py` runs `rccl-tests` (`all_reduce_perf` / `alltoall_perf`) across a node list using MPICH, compares the measured `algbw` against an analytical threshold, and then recursively bisects the allocation in parallel to pinpoint the faulty node(s). It automates the manual "bisect the cluster when NCCL hangs" procedure.

This section describes how to run it directly on a SLURM-only or bare-metal cluster, with no Docker, no Podman, and no Ansible. The only hard requirement is that each node has working ROCm / RCCL / RDMA drivers plus the binaries listed below, and that the nodes can SSH to each other.

### Prerequisites on every node

`binary_diagnose.py` shells out to MPICH and pre-built `rccl-tests` binaries at hard-coded paths (`preflight/network/binary_diagnose.py` lines 24-32):

- `/opt/mpich/bin/mpirun`
- `/opt/rccl-tests/build/all_reduce_perf`
- `/opt/rccl-tests/build/alltoall_perf`
- `/opt/rocm/lib` on `LD_LIBRARY_PATH`
- Passwordless SSH between all nodes on the port passed via `--ssh_port` (default `22`)

The repo ships build scripts that populate those exact paths. Run once per node (or bake into your cluster image):

```bash
cd Bench/preflight/install
bash install_linux_tools.sh
bash install_ucx.sh
bash install_mpich.sh
bash install_rccl_test.sh
```

If you are using AMD AINIC / ANP hardware (for example an AMD Pensando Pollara cluster), also run `install_ainic_driver.sh` and `install_amd_anp.sh`; otherwise for Broadcom NICs run `install_bnxt_driver.sh`. See `preflight/install/install.sh` for the full dependency order.

### Discovering the network parameters

`binary_diagnose.py` needs three site-specific values: the front-end NIC, the list of RDMA HCAs, and the GID index. Run the commands below on any one node; the values are the same across a homogenous cluster.

#### `--socket_ifname` (front-end NIC / `NCCL_SOCKET_IFNAME`)

This is the plain Ethernet / TCP interface that NCCL uses for its bootstrap handshake. It is not the RDMA interface. It must be the interface whose IP is on the route to the job's `MASTER_ADDR`; if it isn't, NCCL prints `Socket IFNAME does not match route-to-master interface (may hang init_process_group)` and `init_process_group` may hang.

```bash
ip route get 8.8.8.8
```

Example output: `8.8.8.8 via 10.1.0.1 dev enp193s0f0np0 src 10.1.2.34 …` → use `enp193s0f0np0`.

If your cluster has no default route, resolve against the master node IP instead:

```bash
ip route get <master_node_ip>
ip -o -4 addr show          # lists all interfaces with IPs
```

#### `--ib_hca` (RDMA HCA list / `NCCL_IB_HCA`)

This is the comma-separated list of RDMA device names RCCL will use for the high-bandwidth GPU-to-GPU plane (typically one HCA per GPU, i.e. 8 entries on an 8-GPU node).

```bash
ibv_devices              # canonical list of RDMA device names
ls /sys/class/infiniband # same info via sysfs, works without user-space tools
rdma link show           # shows device, port state, and the associated netdev
```

Example `ibv_devices` output:

```
device                 node GUID
------              ----------------
ionic_0             0000000000000001
ionic_1             0000000000000002
...
ionic_7             0000000000000008
```

→ `--ib_hca ionic_0,ionic_1,ionic_2,ionic_3,ionic_4,ionic_5,ionic_6,ionic_7` (no spaces).

Common device-name families: `ionic_*` (AMD Pensando AINIC, including the Pollara 400 AI NIC), `bnxt_re*` (Broadcom), `mlx5_*` (NVIDIA/Mellanox ConnectX). Use exactly what `ibv_devices` prints.

Verify every port is `ACTIVE` before running the test - a DOWN port is one of the things this tool is designed to catch, but it's faster to rule it out first:

```bash
for p in /sys/class/infiniband/*/ports/1/state; do echo "$p: $(cat $p)"; done
```

#### `--ib_gid_index` (RoCE GID index / `NCCL_IB_GID_INDEX`)

For RoCE you want the GID index whose type is `RoCE v2`.

```bash
for dev in /sys/class/infiniband/*; do
  name=$(basename "$dev")
  for i in $(seq 0 7); do
    gid=$(cat "$dev/ports/1/gids/$i" 2>/dev/null)
    typ=$(cat "$dev/ports/1/gid_attrs/types/$i" 2>/dev/null)
    [ -n "$gid" ] && [ "$gid" != "0000:0000:0000:0000:0000:0000:0000:0000" ] \
      && echo "$name gid[$i]=$gid type=$typ"
  done
done
```

Rule of thumb baked into this repo (`preflight/network/run.sh` lines 47-53):

- `bnxt_re*` (Broadcom) → `--ib_gid_index 3`
- `ionic_*` / AINIC (including AMD Pensando Pollara) → `--ib_gid_index 1` (sometimes `0`)

### Running `binary_diagnose.py`

Create a hosts file listing one IP (or resolvable hostname) per line - one entry per node you want to test:

```bash
cat > /tmp/hosts <<EOF
10.0.0.1
10.0.0.2
10.0.0.3
10.0.0.4
EOF
```

If you are inside a SLURM allocation, you can generate it automatically:

```bash
srun -N "$SLURM_NNODES" --ntasks-per-node=1 bash -c 'hostname -I | awk "{print \$1}"' \
  > /tmp/hosts
```

Then run the diagnosis from any one node (typically the SLURM rank-0 / login node that has SSH to all workers):

```bash
cd Bench/preflight/network

python3 -u binary_diagnose.py \
    --nodes_file   /tmp/hosts            \
    --socket_ifname <front_end_nic>      \
    --ib_hca        <ib_hca_list>        \
    --ib_gid_index  <gid_index>          \
    --ssh_port      22                   \
    --rccl_test_type 1
```

Flag reference (see `preflight/network/binary_diagnose.py` lines 603-619 for the full `argparse` surface):

- `--rccl_test_type`: `0` = `all_reduce_perf`, `1` = `alltoall_perf`. Use `1` to reproduce the Cohere-style all-to-all stress that tends to expose hangs.
- `--socket_ifname`, `--ib_hca`, `--ib_gid_index`: the values discovered above.
- `--ssh_port`: SSH port used for `mpirun`'s launcher and for hostname lookups (default `22`).
- `--enable_ainic true`: use the AMD ANP network plugin and its optimized library paths (only for AINIC hardware, e.g. AMD Pensando Pollara).
- `--max_concurrent`: cap on the number of parallel bisection branches (default `8`).
- `--rccl_debug`: passed through as `NCCL_DEBUG` (`VERSION` / `INFO` / `DEBUG`).
- Env vars `BNIC`, `BXGMI`, `GPU_PRODUCT`: tune the pass/fail bandwidth threshold (`preflight/network/binary_diagnose.py` lines 175-191). For example `BNIC=48 BXGMI=315 GPU_PRODUCT=MI355X`.

### Expected output

On success, stdout ends with:

```
[RESULT] ✅ all passed, obtained through alltoall_perf
```

On failure, the tool prints the confirmed-bad nodes using `hostname(ip)` labels:

```
[RESULT] unhealthy nodes: [nodeA(10.0.0.1), nodeB(10.0.0.2)], obtained through alltoall_perf
```

Process exit code is `0` on pass and `1` on failure, so you can wrap it in a SLURM job or cron without parsing logs.

---

## Key scripts and directories
- `run_bare_metal.sh`: bare-metal entrypoint; installs Docker, calls Ansible playbooks, streams logs
- `run_slurm.sh`: SLURM execution with automatic resource allocation
- `run_slurm.sh`: preflight and network checks inside the SLURM job
- `run.sh`: container entrypoint; runs I/O benchmarks, node/network preflight, system benchmarks, and collects results
- `playbooks/`: Ansible playbooks (bare-metal install and benchmarks)
- `kubernetes/`: Kubernetes manifests including PyTorchJob specifications
- `benchmarks/`: benchmark implementations and build scripts
- `preflight/`: SSH, node, and network preflight components
- `Dockerfile`: builds an image with necessary dependencies and tools

---

## Outputs and logs
Default output locations:
- Bare metal: `outputs/<TIMESTAMP>`
- SLURM: `output/<TIMESTAMP>` (or `SHARE_PATH/output/<TIMESTAMP>`)
- Kubernetes: Written to shared workspace volume specified in PyTorchJob manifest

Important files (examples):
- `primusbench.log`: main runtime log (streamed by bare-metal entry)
- `io_benchmarks.log`: I/O benchmark log (when enabled)
- `preflight_node.log` / `preflight_network.log`: preflight results
- `overlap_results.json` / `kernel_overhead_results.json`: system benchmark results

---

## Container image
Default images are set via environment variables in the scripts:
- Benchmark image: `IMAGE` (used by `run_bare_metal.sh` / Ansible playbooks)
- Preflight images: `PREFLIGHT_NODE_IMAGE`, `PREFLIGHT_NETWORK_IMAGE` (SLURM)

### Build the image
- Build from this repo:
```bash
docker build -t primussafe/primusbench:{{TAG}} .
```
---

## FAQ
- **Ansible connection failures**: Verify hostnames/IPs in `hosts.ini` resolve; confirm SSH port and passwordless access; set `INVENTORY_FILE` to a custom inventory if needed.
- **Logs not generated**: In bare-metal mode, main log is `outputs/<TIMESTAMP>/primusbench.log`; if missing, check bare-metal or install phase logs first.
- **Nodes marked unhealthy**: Inspect `preflight_node.log` and `preflight_network.log`; fix reported issues (ports, drivers, clocks, DNS, bandwidth/latency anomalies, etc.).
- **I/O benchmarks not running**: Set `IO_BENCHMARK_MOUNT` to the target mount point.

---

## License
This project is licensed under the terms described in the `LICENSE` file.


