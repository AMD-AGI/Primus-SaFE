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

Two phases: request nodes with `salloc`, then inside the allocation `run_slurm.sh` performs node and network preflight and prepares the benchmark.

Submit:
```bash
# Request 2 nodes on partition amd-tw for 4.5 hours (adjust as needed)
NNODES=2 PARTITION=amd-tw TIME=4:30:00 bash salloc_slurm.sh
```

`salloc_slurm.sh` will:
- Acquire node list via `salloc`
- Pass the node list to `run_slurm.sh`

`run_slurm.sh` will:
- Use containers on each node to run node and network preflight
- Filter out unhealthy nodes; keep healthy nodes for subsequent benchmarks
- Write outputs to `output/<TIMESTAMP>`

Environment variables (subset):
- `NNODES`: number of nodes (default 2)
- `PARTITION`: SLURM partition (default `amd-tw`)
- `TIME`: job time limit (default `4:30:00`)
- `PREFLIGHT_NODE_IMAGE` / `PREFLIGHT_NETWORK_IMAGE`: preflight images (defaults provided)
- `OUTPUT_PATH` / `SHARE_PATH`: specify or share output directory

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

## Key scripts and directories
- `run_bare_metal.sh`: bare-metal entrypoint; installs Docker, calls Ansible playbooks, streams logs
- `salloc_slurm.sh`: SLURM resource allocation entrypoint
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


