---
sidebar_position: 1
title: Prerequisites
---

# Prerequisites

What you need before installing Primus-SaFE, and which install path to take. If you already
run Kubernetes, you can skip straight to [Install](/getting-started/install). If you are
starting from bare-metal servers, you will provision a cluster first.

This page is a static reference, but it is written to serve two audiences at once: a **human
reader** planning an install, and an **AI agent** that reads the same page. There is no
executable procedure here and no hidden test layer — the only checkable thing is that the
documented requirement tables and controls are present. Bookkeeping (priority, known product
bugs) lives in the run contract `docs-site/AGENTS.md`.

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
- **Shared filesystem (optional)** — a high-performance Parallel File System (PFS) to support a production system.

## Network ports

Open the following ports so the cluster and its users can communicate:

| Port | Protocol | Purpose | Exposure |
|------|----------|---------|----------|
| `443` | TCP | HTTPS access to the console | External |
| `80` | TCP | HTTP (redirects/ingress) | External |
| `2222` | TCP | WebShell / SSH into pods | External |
| `6443` | TCP | Kubernetes API server | External (optional) |
| `22` | TCP | SSH between cluster nodes | Internal — no need to expose externally |
| `2379`, `2380` | TCP | etcd peer/client communication | Internal — must be reachable between control-plane nodes, no need to expose externally |

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

*Exact control-plane sizing per scale (small/medium/large) and RDMA NIC naming depend on your
hardware; use the requirement tables above as the baseline. An agent presence-checks those tables
rather than asserting specific sizing or NIC-name values.*

## What an agent verifies here

This is a reference page — there are no steps to perform. An agent confirms only that the key
named artifacts are **present and readable**: the **Tooling** table (`kubectl`, `helm`,
cluster-admin access), the **Cluster requirements** list (Kubernetes 1.21+, a default
StorageClass, GPU nodes), the **Network ports** table (including `2222` for WebShell/SSH), and
the **Choose your starting point** table. No values need to be exercised against a live cluster.
