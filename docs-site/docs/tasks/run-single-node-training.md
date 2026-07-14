---
sidebar_position: 1
title: Run a single-node training job
---

# Run a single-node training job

This page is a hands-on walkthrough for submitting a training workload that runs on a **single
node** (one pod, using up to all of that node's GPUs). For a job spanning several nodes, see
[Run a multi-node distributed job](/tasks/run-multi-node-training).

It is written to serve two audiences at once:

- **For you (the reader):** every step says *what* to click and *what a healthy result looks
  like*, so you can follow it from an empty workspace to a running job.
- **For an AI agent:** the steps are concrete and ordered, and each one states its own pass/fail
  outcome in plain language — so you can hand this page to an agent and have it perform the task
  and judge success without any hidden instructions.

There is no separate test file and no invisible annotation on this page: **the prose you read is
the test an agent runs.** The only thing kept elsewhere is bookkeeping (priority, and any known
product bug), in the run contract `docs-site/AGENTS.md`.

## Before you start

You need two things. If either is missing, this walkthrough is **blocked** — stop and get it
first, don't work around it:

- Access to a **workspace with GPU quota** — this is the job's target. See
  [Workspace](/concepts/workspace).
- A **container image** the cluster can pull — a public image is fine (e.g.
  `docker.io/rocm/pytorch:latest`), or one from your registry.

> **Agent:** confirm both preconditions before Step 1. Select a workspace in the top-left
> selector; if none has quota, report **BLOCKED** (missing `workspace-with-quota`) rather than
> failing a step. Do not create a workspace yourself — that is an admin task this page does not
> cover.

## Step 1 — Open the submit form

1. Select your **workspace** in the top-left workspace selector.
2. Go to **Workloads → Training → PyTorch** and click **Create PyTorch Job**.

You should now be on the **Create PyTorch Job** form, with sections for **Basic information** and
**Resources**. If the **PyTorch** tab or the **Create PyTorch Job** button isn't there, the
workspace is missing the **Train** scope — that's the sign to stop and ask a workspace admin,
not a problem with these steps.

![Create PyTorch Job form](/img/screenshots/pytorch-create-form.png)

## Step 2 — Fill in Basic information

- **Name** — a name for the job. Use a unique name so repeated runs don't collide. (An agent
  running the test suite names created resources per the cleanup convention in the run contract.)
- **Image** — click **Select** to pick a registered image, or **Custom** to type any pullable
  reference (e.g. `docker.io/rocm/pytorch:latest`).
- **Entry point** — the command to run, e.g. `python train.py` (a trivial command such as
  `sleep 60` is fine for a smoke test — the point is that the platform *accepts and schedules*
  the job).
- **Priority** — Low / Medium / High (higher priorities may require permission; leave Low).

## Step 3 — Size it onto one node (Resources)

This is where you place the job on a node:

- Set the per-replica **GPU**, **CPU**, **memory**, and **ephemeral storage**. Each field shows
  the range your workspace flavor and free quota allow — **stay inside those ranges**. To use a
  whole GPU node set **GPU** to the node's full count (e.g. `8`); for a small dev job request
  fewer.
- Keep the mode on **replicas** and set **replicas = 1** — that is what makes this a single-node
  job (one pod). (Switching to **nodes** mode lets you target a specific node instead.)
- **(Optional) Excluded nodes** — keep the job off specific machines.

A healthy form at this point shows every resource value **within** the allowed range with no red
validation errors. If a field is outlined red or shows "exceeds quota", you asked for more than
the workspace has — lower it; that is a you-asked-too-much signal, not a platform fault.

## Step 4 — Submit and read the result

Leave **Advanced** at its defaults for a first run and click **Submit**.

Here is what each outcome means — this is the pass/fail for the whole walkthrough:

- **Healthy (pass):** the job appears in the **PyTorch** list. If the list looks empty, switch
  the filter to **All**. Its phase is **Pending** or **Running** — the scheduler accepted it and
  is placing (or has placed) the pod. Open the row to watch status and logs; see
  [Develop & interact with your jobs](/tasks/interact-with-your-job).
- **Rejected / Failed (fail):** the submission was *not* accepted. This is almost always an
  input problem, and the phase or an error toast usually says which: an image that can't be
  pulled, no quota left in the workspace, or a node taint blocking placement. Fix the input and
  resubmit — see [Troubleshooting](/troubleshooting).

> **Agent:** fill the table below from what you observe, show it to the user, and report
> **PASS** only if the job reached Pending/Running. Then **run cleanup**: delete the workload you
> created via its row action, so the run is repeatable.
>
> Known product drift to account for (do **not** count as a new failure): the console phase can
> stay **Pending** even after the job has actually run to completion. If Pending persists,
> optionally note the true state from `kubectl` as a ground-truth aside — but the UI-visible
> pass condition here is only "accepted, not Rejected/Failed".

| Check | Healthy result | Found |
|---|---|---|
| Job appears in PyTorch list | yes (filter = All if needed) | _fill in_ |
| Phase after submit | Pending or Running | _fill in_ |
| Validation errors on the form | none | _fill in_ |
| Cleanup (workload deleted) | done | _fill in_ |

## Which node the job lands on

The scheduler picks a node with enough free GPUs (it is gang- and topology-aware). You steer
placement with: **replicas vs. nodes** mode (a single-node job is `replicas = 1`); **GPU count
per replica** (requesting all of a node's GPUs effectively dedicates it); **Excluded nodes**; and
the **workspace flavor**, which already pins jobs to one hardware type (e.g. MI300X). See
[Workspace](/concepts/workspace).

## Doing the same over the REST API

The console is the primary path; for scripts and CI the same job is a `POST /api/v1/workloads`:

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
    "resources": [{ "cpu": "128", "gpu": "8", "memory": "1024Gi", "ephemeralStorage": "300Gi", "replica": 1 }],
    "env": { "NCCL_DEBUG": "INFO" }
  }'
```

`entryPoints` is **base64-encoded** (`cHl0aG9uIHRyYWluLnB5` is `python train.py`). The response
returns the generated `workloadId`.

## Your image

Your job runs the container `image` you specify: a **public image**, or one from your
**registry** (attach a registry secret for private images — see
[Manage access & quota](/administration/manage-access-and-quota)). Without a registry host the
image is pulled from `docker.io`. To make large images start faster, import and pre-stage them —
see [Speed up workload startup](/tasks/speed-up-startup).

## Next

- Watch it, read logs, shell in → [Develop & interact with your jobs](/tasks/interact-with-your-job).
- Scale across nodes → [Run a multi-node distributed job](/tasks/run-multi-node-training).
