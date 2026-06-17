---
sidebar_position: 4
title: Pre-flight & in-flight monitoring
---

# Pre-flight & in-flight monitoring

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/ops-job.md`
> (`preflight`), `fault.md`, `workload.md` (`isSupervised`, `avgGpuUsage`), `Bench/README.md`

The goal is **goodput** — the share of GPU time that goes into useful training. You protect it
on two timelines: validate hardware *before* a production job runs, and watch health *while* it
runs so faults are caught and recovered automatically.

## Before the job: pre-flight checks

A pre-flight check runs as a `preflight` **OpsJob** that executes a test container against a
cluster, a workspace, or specific nodes, then writes a report you can read back.

```bash
curl -X POST https://<your-console>/api/v1/opsjobs \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "preflight-check",
    "type": "preflight",
    "inputs": [ { "name": "cluster", "value": "prod-cluster" } ],
    "image": "harbor.example.com/tools/preflight:latest",
    "entryPoint": "YmFzaCAtYyAnLi9ydW4uc2gnCg==",
    "resource": { "cpu": "8", "memory": "32Gi" },
    "securityOperation": true,
    "isTolerateAll": true,
    "timeoutSecond": 7200
  }'
```

Key options:

- **Target** (`inputs`) — exactly one of `node` (repeatable), `cluster`, `workspace`, or
  `node.host` takes effect. Non-admin users must scope to a `workspaceId`.
- **`entryPoint`** is Base64-encoded.
- **`securityOperation: true`** skips nodes that currently have workloads, so a check won't
  disturb running jobs.
- **`isTolerateAll`** lets the check run on tainted nodes.

Track it like any OpsJob (`GET /api/v1/opsjobs/{jobId}`); when it succeeds, the report location
shows up in the job's `outputs` (e.g. `{ "name": "report", "value": "s3://.../report.json" }`).

### Primus-Bench (standalone)

For deeper hardware/performance benchmarking outside the platform's OpsJob flow (bare-metal,
SLURM, or Kubernetes), use **Primus-Bench**. See `Bench/README.md`.

> **Not yet covered:** a recommended baseline pre-flight image/entryPoint, and how to read the
> report contents (pass/fail criteria).

## During the job: in-flight monitoring

### Dashboards

Grafana dashboards expose cluster-level and per-job health (GPU utilization, memory, network,
temperatures). Per-workload, the platform also surfaces `avgGpuUsage`.

> **Not yet covered:** where Grafana is exposed (URL/route), how it's enabled in
> `charts/primus-safe/values.yaml`, and which dashboards ship by default.

### Automatic health and recovery

The **Node Agent** continuously monitors each node. When it detects a problem it raises a
**Fault**, which taints the affected node so the scheduler stops using it; fault-tolerant
workloads then fail over to healthy capacity. This is the core of
[Fault tolerance](/concepts/fault-tolerance).

- List active faults: `GET /api/v1/faults`.
- A faulted node shows as unavailable with a reason in the [node list](/administration/manage-nodes#inspect-capacity).

### Hang detection

Set `isSupervised` on a workload to enable hang detection (a job making no progress is caught
even when nothing has crashed).

> **Not yet covered (capture so we don't lose it):**
> - [ ] Fault types and lifecycle; how to **clear** a fault and untaint the node
>       (`fault.md`).
> - [ ] What `isSupervised` does on detection (restart? alert?) and how to enable it from the
>       console.
> - [ ] Alerting/notification routing (link to user notification settings).
