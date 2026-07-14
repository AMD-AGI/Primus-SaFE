---
sidebar_position: 1
title: Overview
slug: /
---

# Primus-SaFE

This page is the top-level orientation to the platform: what Primus-SaFE is, why it
exists, and where to go next. It is reference/explanation — there is no procedure to run
here, just the map that points you at the rest of the docs.

It is written to serve two audiences at once:

- **For you (the reader):** a plain-language overview of the platform and a set of
  "where to start" links.
- **For an AI agent:** the named sections and navigation targets below are concrete
  enough to confirm by presence. This is an overview page, so an agent **verifies** that
  the documented structure exists — it does not perform a task.

There is no separate test file and no invisible annotation on this page: the prose you
read is all there is. The only thing kept elsewhere is bookkeeping (priority, and any
known product bug), in the run contract `docs-site/AGENTS.md`.

**Primus-SaFE** (Stability and Fault Endurance) is AMD's full-stack platform for
large-scale model training and inference on AMD GPU clusters. It runs on top of Kubernetes
and the ROCm software stack, turning a fleet of GPU servers into a resilient, self-monitoring
environment so teams can focus on their models rather than the underlying infrastructure.

Training large models at scale is difficult less because of any single job than because of
everything around it: nodes fail, networks fluctuate, and one unhealthy GPU can stall an
entire distributed run. Primus-SaFE is designed to protect your **goodput** — the share of
GPU time that goes into useful training — by validating hardware before a job runs, detecting
faults as they occur, and recovering automatically.

> **What an agent verifies here:** open the console and confirm the documented overview
> renders — the **Why Primus-SaFE** bullets, the **What you get** capability table, and the
> **Where to start** navigation table are all present, and each "Where to start" link
> resolves to its demo page (Architecture, Getting Started, Concepts, Tasks,
> Administration). This is presence checking only — nothing here is created or mutated.

## Why Primus-SaFE

- **Goodput first.** Preflight health checks keep unhealthy nodes out of production, and
  automatic failover with checkpoint resume recovers a job after a node or GPU fails —
  instead of losing hours of progress.
- **Intelligent scheduling.** Multi-priority queues with preemption, gang scheduling for
  distributed jobs, and topology-aware placement that respects network locality.
- **Full-stack observability.** Cluster- and job-level metrics, logs, and dashboards that
  make it straightforward to see what is happening and diagnose issues.
- **Multi-tenant by design.** Workspaces isolate quota, storage, and access, allowing many
  teams to share a cluster safely.
- **Operate it your way.** A web console, a REST API, and a natural-language agent interface
  all drive the same platform.

## What you get

| Capability | What it means for you |
|------------|------------------------|
| Training & inference workloads | Run PyTorchJob / Job distributed training, interactive dev boxes, and model serving |
| Automatic fault tolerance | Node-level health monitoring, fault detection, and failover/retry |
| Preflight validation | Benchmark and health-check nodes before they run production jobs |
| Multi-tenancy | Workspaces with quota, isolation, and role-based access |
| Agentic operations | Operate the cluster in natural language, or connect your own agent over MCP |

## Where to start

| You want to… | Go to |
|--------------|-------|
| Understand how the pieces fit | [Architecture](/architecture) |
| Install it and run a job | [Getting Started](/getting-started/prerequisites) |
| Learn the core resources | [Concepts](/concepts/workspace) |
| Run & manage workloads | [Tasks](/tasks/run-single-node-training) |
| Operate the cluster | [Administration](/administration/manage-access-and-quota) |

## The Primus family

Primus-SaFE is the stability and platform layer of AMD's Primus stack:

- **[Primus-LM](https://github.com/AMD-AGI/Primus)** — an end-to-end training framework
  (Megatron, TorchTitan, and more) that runs on top of Primus-SaFE for stable scheduling.
- **[Primus-Turbo](https://github.com/AMD-AGI/Primus-Turbo)** — high-performance operators
  and modules (FlashAttention, GEMM, collectives) optimized for AMD GPUs.
- **Primus-SaFE** — this project: cluster sanity checks, topology-aware scheduling, fault
  tolerance, and stability for the Kubernetes and Slurm ecosystems.

:::note
Primus-SaFE is licensed under Apache 2.0 and is under active development.
:::
