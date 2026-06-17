---
sidebar_position: 6
title: Beyond training (the LLM lifecycle)
---

# Beyond training (the LLM lifecycle)

> **Status:** TODO · **Owner:** _unassigned_
> **Purpose:** show that Primus-SaFE covers the whole LLM development lifecycle, not just a
> training run. Breadth over depth — point to detail, don't duplicate it.

:::note Content brief
- [ ] Framing: one platform, one `Workload` API, across the lifecycle
      (develop → train → fault-tolerant train → host inference).
- [ ] **Host an inference service** after training: a serving `Deployment` (e.g. vLLM) with a
      `service` + liveness/readiness — short how-to, reuse the workload API.
- [ ] **Fault-tolerant / elastic training** with **TorchFT** — one paragraph + link the blog.
- [ ] **Interactive development** with Authoring — link to
      [Develop with an Authoring dev box](/tasks/authoring-dev-box).
- [ ] Briefly name other kinds that exist (Ray, StatefulSet, CICD) and link the
      [Workload types](/concepts/workload-types) concept rather than detailing them.
- [ ] Keep small/experimental features out (e.g. model chat/playground).

**Source:** `SaFE/docs/apis/workload.md` (Deployment/TorchFT/RayJob examples, `service`,
liveness/readiness), TorchFT blog (external).
:::
