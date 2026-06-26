---
sidebar_position: 1
title: Prerequisites
---

# Prerequisites

What you need before installing Primus-SaFE, and which install path to take. If you already
run Kubernetes, you can skip straight to [Install](/getting-started/install). If you are
starting from bare-metal servers, you will provision a cluster first.

<!-- @test none: static reference page (tooling/requirements). Nothing for the agent to exercise. -->

## Tooling

Install these on the machine you will run the installer from (it needs access to the target
cluster's API):

| Tool | Version | Used for |
|------|---------|----------|
| `kubectl` | matching your cluster | Talking to the Kubernetes API |
| `helm` | 3+ | Installing the Primus-SaFE charts |
| Cluster admin access | — | A kubeconfig with cluster-admin (the installer creates CRDs, namespaces, and RBAC) |

## Cluster requirements

- **Kubernetes 1.21+** — Primus-SaFE runs on any conformant cluster at this version or newer.
- **A default StorageClass** — used for the platform's persistent state (the installer
  defaults to `local-path`; the StorageClass must already exist).
- **GPU nodes** — AMD GPU nodes for running workloads. The GPU operator is installed as a
  cluster add-on.
- **High-speed networking for multi-node jobs** — RDMA / InfiniBand interfaces if you intend
  to run distributed training across nodes.
- **Shared filesystem (optional)** — a CSI volume for workspace persistent storage (PFS). You
  can enable this at install time or leave it disabled to start.

## Choose your starting point

| You have… | Start with |
|-----------|------------|
| Bare-metal servers, no Kubernetes | [Bootstrap](/getting-started/install) to provision a cluster, then install |
| An existing Kubernetes 1.21+ cluster | [Install](/getting-started/install) directly |

## Supported platforms

AMD GPUs with the ROCm stack are the primary target. The scheduling and platform core is
vendor-agnostic Kubernetes, but the health checks and [Primus-Bench](/getting-started/install)
benchmarks are ROCm-specific.

:::note
AMD GPUs with ROCm are the primary, validated target. Treat other platforms (e.g. NVIDIA or
managed cloud) as community/experimental for now.
:::

<!-- @test todo:
  - "Add minimum CPU/memory sizing per control-plane scale (small/medium/large)."
  - "Document the expected RDMA NIC naming so this page can state it precisely."
-->
