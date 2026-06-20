---
sidebar_position: 4
title: Develop with an Authoring dev box
---

# Develop with an Authoring dev box

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/workload.md`,
> `charts/.../authoring_template.yaml`, `webhooks/pkg/workload_webhook.go`, `image.md`,
> `ops-job.md`

**Authoring** is a personal, interactive dev box on the cluster — a long-running, single-node
environment you shell into and work in, with your GPUs, code, and workspace storage already
mounted. Think of it as a remote dev machine: use it to prototype, debug, and prepare your
environment before launching a full (often multi-node) [training job](/tasks/run-multi-node-training).

Unlike a batch job, an Authoring workload is **kept alive** for you: the platform runs it as a
single-node pod whose entrypoint is `sleep infinity`, with retries, hang-detection, and
failover turned off (it's an interactive session, not a job to complete). See the
[Authoring kind](/concepts/workload-types) concept for how it fits among workload types.

## 1. Start a dev box

Your workspace must include the **Authoring** scope (ask a workspace-admin if you don't see
it — [Workspace](/concepts/workspace)). Pick an image and the resources you need:

```bash
curl -X POST https://<your-console>/api/v1/workloads \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "displayName": "my-devbox",
    "workspaceId": "prod-cluster-ai-team",
    "groupVersionKind": { "kind": "Authoring", "version": "v1" },
    "images": ["harbor.example.com/ai/pytorch:2.0"],
    "resources": [{ "cpu": "16", "gpu": "1", "memory": "128Gi", "replica": 1 }]
  }'
```

## 2. Connect and work

Connect via WebShell or SSH and develop interactively — edit code, run scripts, debug. SSH
works with `scp` and VS Code Remote-SSH, so you can treat the dev box like a local machine.
See [Interact with your job](/tasks/interact-with-your-job) for the connection steps.

## 3. Save your environment as a custom image

Once you've set up the box (installed packages, configs, dependencies), use **Save Image** (an
`exportimage` OpsJob) to snapshot the container into a reusable image in the internal Harbor
registry — so you don't rebuild the environment next time. Saved images appear in
`GET /api/v1/images/custom` and the returned `imageName` is directly usable as the `image` for
your next PyTorchJob.

To pre-stage that image across nodes for faster startup, see
[Speed up workload startup](/tasks/speed-up-startup).

> **Not yet covered (capture so we don't lose it):**
> - [ ] Exact **Save Image** flow (console action and/or the `exportimage` OpsJob payload),
>       including the optional `label`.
> - [ ] **Resume** a stopped Authoring box.
> - [ ] Console screenshots for the dev-box + Save Image flow.
