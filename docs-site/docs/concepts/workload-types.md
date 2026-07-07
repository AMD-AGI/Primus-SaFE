---
sidebar_position: 2
title: Workload types
---

# Workload types

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/workload.md`

A **Workload** is the unit of work you submit. You pick a *kind* via `groupVersionKind.kind`;
the platform manages its full lifecycle. All kinds share the same submit flow — see
[Run a single-node training job](/tasks/run-single-node-training).

## The common kinds

| Kind | Use it for | Workspace scope |
|------|------------|-----------------|
| **PyTorchJob** | Distributed (multi-node) PyTorch training | Train |
| **TorchFT** | Fault-tolerant training with elastic replica groups | Train |
| **Deployment** | Long-running inference / serving | Infer |
| **StatefulSet** | Stateful services | Infer |
| **Authoring** | An interactive dev box (see below) | Authoring |
| **AutoscalingRunnerSet** | GitHub Actions CI/CD runners | CICD |

## Authoring (the dev-box kind)

**Authoring** is a first-class workload kind for interactive development: a single-replica pod
that the platform keeps alive so you can work in it like a remote machine. You reach it via
the console's WebShell or over SSH. The hands-on how-to is in
[Tasks → Develop with an Authoring dev box](/tasks/authoring-dev-box).

## Which type should I pick?

- Running a training script across one or more nodes → **PyTorchJob**.
- Want training that survives node loss by scaling replica groups → **TorchFT**.
- Serving a model or API that stays up → **Deployment**.
- Need an interactive environment to develop and debug → **Authoring**.
- Wiring up CI runners → **AutoscalingRunnerSet**.

## The lifecycle the platform manages

Once submitted, the Job Manager handles a workload through:

1. **Queue** — admitted into the workspace queue (per its queue policy).
2. **Schedule** — placed with gang semantics and topology awareness; higher priority can
   preempt where enabled.
3. **Run** — pods start on GPU nodes; status, logs, and metrics are tracked.
4. **Recover** — on failure, automatic retry / failover up to `maxRetry`
   (see [Fault tolerance](/concepts/fault-tolerance)).

Phases you'll see: `Pending` → `Running` → `Succeeded` / `Failed`, plus `Stopped`, and
`Updating` / `NotReady` for Deployment/StatefulSet-style kinds.

## Common settings

Most kinds accept the same core fields:

| Setting | Notes |
|---------|-------|
| `resources` | Per-role CPU / GPU / memory / storage and `replica` count. |
| `images` / `entryPoints` | Index-aligned with `resources`; entrypoints are Base64-encoded. |
| `priority` | `0` low / `1` medium / `2` high. |
| `env` | Environment variables (e.g. `NCCL_DEBUG`). |
| `timeout`, `maxRetry` | Run-time cap and retry limit. |
| `dependencies` | Other workloads that must finish first. |
| `secrets` | Image-pull and general secrets to attach. |

The full field reference lives in the workload API (`SaFE/docs/apis/workload.md`).

> **Not yet covered (in code, not yet user-documented):** additional kinds exist —
> **RayJob**, **MonarchJob**, and advanced serving (DynamoDeployment, InferaDeployment),
> plus a Sandbox scope. They are listed here so the table isn't silently incomplete; treat
> them as advanced/experimental until documented.
