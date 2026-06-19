---
sidebar_position: 1
title: Run a single-node training job
---

# Run a single-node training job

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/workload.md`

The canonical "how to submit a workload" reference. This covers a single node (one pod, up to
all of its GPUs); for a job spanning several nodes see
[Run a multi-node distributed job](/tasks/run-multi-node-training).

You can submit the same job three ways: the **web console**, the **REST API**, or the
**agent**. Pick one; they all create the same `Workload`.

## Before you start

- You can sign in to the console (or have an API key — see
  [Manage access & quota](/administration/manage-access-and-quota)).
- You belong to a **workspace** with GPU quota — your job's `workspaceId`. See
  [Workspace](/concepts/workspace).
- You have a **container image** to run (a public image, or one in your registry — see
  [Your image](#your-image) below).

## Option A — Web console

1. Open the console and select your **workspace**.
2. Create a new **workload** and choose the type **PyTorchJob** (training).
3. Set the **image**, the **entrypoint** (e.g. `python train.py`), and the **resources**
   (CPU, GPU, memory) for the single replica.
4. (Optional) add **environment variables** (e.g. `NCCL_DEBUG=INFO`).
5. Submit, then watch it on the workload's detail page — see
   [Interact with your job](/tasks/interact-with-your-job).

> **Not yet covered (needs assets):**
> - [ ] Step-by-step screenshots of the console submit flow.

## Option B — REST API

Submit with `POST /api/v1/workloads`. A minimal single-node PyTorchJob:

```bash
curl -X POST https://<your-console>/api/v1/workloads \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "displayName": "my-training-job",
    "workspaceId": "cluster-workspace",
    "groupVersionKind": { "kind": "PyTorchJob", "version": "v1" },
    "images": ["harbor.example.com/ai/pytorch:2.0"],
    "entryPoints": ["cHl0aG9uIHRyYWluLnB5"],
    "resources": [{
      "cpu": "128",
      "gpu": "8",
      "memory": "1024Gi",
      "ephemeralStorage": "300Gi",
      "replica": 1
    }],
    "env": { "NCCL_DEBUG": "INFO" }
  }'
```

The response returns the generated workload ID:

```json
{ "workloadId": "my-training-job-abc12" }
```

Key fields:

| Field | Notes |
|-------|-------|
| `groupVersionKind.kind` | `PyTorchJob` for training (also `Deployment`, `StatefulSet`, `Authoring`, `TorchFT`, …). |
| `images` / `entryPoints` / `resources` | Index-aligned arrays — one entry per role. |
| `entryPoints` | **Base64-encoded.** `cHl0aG9uIHRyYWluLnB5` is `python train.py` (`echo -n 'python train.py' \| base64`). |
| `resources[].gpu`, `replica` | GPUs per replica and number of replicas; CPU in cores, memory like `"1024Gi"`. |
| `priority` | `0` low, `1` medium, `2` high (higher priorities may need permission). |
| `useWorkspaceStorage` | Mounts the workspace storage into the job (default `true`). |

The full field list lives in the workload API (`SaFE/docs/apis/workload.md`).

## Your image

Your job runs the container `image` you specify. You can:

- Use a **public image** (e.g. an upstream PyTorch/ROCm image), or
- Push to your **registry** and reference it, attaching a **registry secret** for private
  images. See [Manage access & quota → Registry secret](/administration/manage-access-and-quota).

To make large images start faster, import and pre-stage them — see
[Speed up workload startup](/tasks/speed-up-startup).

## Next

- Watch it, read logs, shell in, get results → [Interact with your job](/tasks/interact-with-your-job).
- Scale it across nodes → [Run a multi-node distributed job](/tasks/run-multi-node-training).
