---
sidebar_position: 1
title: Run a single-node training job
---

# Run a single-node training job

The canonical how-to for submitting a training workload that runs on a **single node** (one pod,
using up to all of that node's GPUs). For a job spanning several nodes, see
[Run a multi-node distributed job](/tasks/run-multi-node-training).

The **web console** is the primary way to submit; a REST API is available for automation.

## Before you start

- You have access to a **workspace** with GPU quota — this is your job's target. See
  [Workspace](/concepts/workspace).
- You have a **container image** to run — a public image, or one in your registry (see
  [Your image](#your-image)).

## Submit from the web console

1. Select your **workspace** in the top-left workspace selector.
2. Go to **Workloads → Training → PyTorch** and click **Create PyTorch Job**.
3. **Basic information**
   - **Name** — a name for the job.
   - **Image** — click **Select** to pick a registered image, or **Custom** to type any pullable
     image reference (e.g. `docker.io/rocm/pytorch:latest`).
   - **Entry point** — the command to run, e.g. `python train.py`.
   - **Priority** — Low / Medium / High (higher priorities may require permission).
4. **Resources** — this is where you size the job onto a node:
   - Set the per-replica **GPU**, **CPU**, **memory**, and **ephemeral storage**. To use a whole
     GPU node, set **GPU** to the node's full GPU count (e.g. `8`); for a smaller dev job, request
     fewer. Each field shows the range allowed by your workspace flavor and free quota.
   - Keep the mode on **replicas** and set **replicas = 1** for a single node (one pod). Or click **nodes** to select a specific node as the target.
   - **(Optional) Excluded nodes** — keep the job off specific nodes. **Include/Exclude** nodes simulates the behavior of Slurm jobs.
5. **(Optional) Advanced** — auto-recovery / failover, hang detection, a run timeout, and other
   settings. Leave the defaults for a first run.
6. **Submit.** The job appears in the PyTorch list; open its row to watch status and logs — see
   [Interact with your job](/tasks/interact-with-your-job).

![Create PyTorch Job form](/img/screenshots/pytorch-create-form.png)

<!-- @test none: the submit-a-PyTorchJob flow is exercised by getting-started/first-training-job; this page is the detailed console reference, not a separate test. -->

### Specifying which node a job uses

The scheduler places your pod on a node that has enough free GPUs (it is gang- and
topology-aware). You influence placement with:

- **Replicas vs. nodes mode** — *replicas* sizes the job by pod count + per-replica resources;
  *nodes* allocates whole nodes. A single-node job is `replicas = 1`.
- **GPU count per replica** — requesting all of a node's GPUs effectively dedicates that node to
  the job.
- **Excluded nodes** — steer the job away from specific nodes.
- **Workspace flavor** — the workspace is already bound to one node flavor, so jobs only land on
  that hardware type (e.g. MI300X). See [Workspace](/concepts/workspace).

## Submit via the REST API (automation)

For scripts and CI, submit the same job with `POST /api/v1/workloads`:

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

## Your image

Your job runs the container `image` you specify. You can:

- Use a **public image** (e.g. an upstream PyTorch/ROCm image), or
- Push to your **registry** and reference it, attaching a **registry secret** for private images.
  See [Manage access & quota → Registry secret](/administration/manage-access-and-quota).

If you don't prefix the image with a registry host, it is pulled from `docker.io` by default; you
can also pull from Quay or any other OCI-compatible registry by giving the full reference.

Importing an image into the self-hosted Harbor registry — the **Images** tab in the side panel —
makes it slightly faster to pull than fetching over the open internet. To make large images start
faster, import and pre-stage them — see [Speed up workload startup](/tasks/speed-up-startup).

## Next

- Watch it, read logs, shell in, get results → [Interact with your job](/tasks/interact-with-your-job).
- Scale it across nodes → [Run a multi-node distributed job](/tasks/run-multi-node-training).
