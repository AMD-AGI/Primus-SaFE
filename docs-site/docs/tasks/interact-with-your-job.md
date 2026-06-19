---
sidebar_position: 3
title: Interact with your job
---

# Interact with your job

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/workload.md`,
> `webshell.md`, `public-key.md`, `ops-job.md`

Once a job is running, you'll want to watch its status, read its logs, shell into a pod,
see dashboards, and pull results out. The console exposes all of this; the same data is
available over the API.

## Status

Track a workload's phase through `Pending` → `Running` → `Succeeded` / `Failed` (also
`Stopped`). Get full detail — per-pod status, the nodes in use, and timing — with:

```bash
curl -H "Authorization: Bearer <token>" \
  https://<your-console>/api/v1/workloads/{workloadId}
```

The response lists each pod's `phase`, `nodeName`, and a ready-to-use `sshCommand`, plus
`conditions` that explain scheduling/dispatch decisions.

## Logs

Read a pod's logs (most recent first via `tailLines`):

```bash
curl -H "Authorization: Bearer <token>" \
  "https://<your-console>/api/custom/workloads/{workloadId}/pods/{podId}/logs?tailLines=500"
```

## Shell in

Two ways to get an interactive shell — both go through the API server, which `exec`s into the
container (sessions are authenticated and audit-logged).

### WebShell (browser)

Open a terminal on a pod directly from the console. It's a WebSocket terminal supporting
`bash` / `sh` / `zsh`. Limits: up to 10 concurrent sessions per user, auto-disconnect after
30 minutes idle, and no file upload/download (use volume mounts).

### SSH (terminal, port 2222)

1. Register your SSH **public** key once — see
   [Manage access & quota → SSH public keys](/administration/manage-access-and-quota).
2. Connect through the SaFE SSH gateway on **port 2222**, using the `sshCommand` from the
   workload details response:

```bash
ssh <user>.<podId>.<workspace>@<gateway-host> -p 2222
```

This also works with `scp` and VS Code Remote-SSH for editing files in the pod.

## Dashboards

The platform ships Grafana dashboards for cluster- and job-level metrics (the detail response
also reports `avgGpuUsage` per workload). Open the dashboards from the console.

## Get results out

> **Not yet covered (capture so we don't lose it):**
> - [ ] Checkpoints/outputs written to workspace **PFS** (link to [Storage](/concepts/storage-and-data)).
> - [ ] **Dump logs** to storage (`dumplog` OpsJob) and **download** artifacts via the
>       `download` OpsJob (S3 → path).
> - [ ] **Notifications** (`user-notification-settings`, email relay) on job/node events.
> - [ ] **Primus-Lens** observability surface — confirm whether it's user-facing and link it.
