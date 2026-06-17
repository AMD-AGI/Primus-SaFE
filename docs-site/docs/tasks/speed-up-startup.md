---
sidebar_position: 5
title: Speed up workload startup
---

# Speed up workload startup

> **Status:** TODO · **Owner:** _unassigned_
> **Purpose:** get workloads running faster by staging images ahead of time — the same
> "don't pull a huge image at job start" problem you hit in a Slurm environment.

:::note Content brief
- [ ] The problem: large images pulled at launch slow down (or stall) job start, especially
      multi-node where every node pulls.
- [ ] **Import an image** into the internal Harbor registry (`POST /api/v1/images:import`,
      optional `secretId` for private sources); track progress; image becomes `ready`.
- [ ] **Preheat / prewarm** an image onto every node in a workspace (`prewarm` OpsJob → a
      DaemonSet pre-pulls it); check progress via `GET /api/v1/images/prewarm`.
- [ ] The workload **`preheat`** flag — prepare the image in advance as part of submission.
- [ ] **Configure image registries** for import sources
      (`/api/v1/image-registries`, default registry).
- [ ] Guidance: when to import vs preheat vs rely on node cache.

**Source:** `SaFE/docs/apis/image.md` (import, prewarm list), `image-registry.md`,
`ops-job.md` (`prewarm` type), `workload.md` (`preheat` field).
:::
