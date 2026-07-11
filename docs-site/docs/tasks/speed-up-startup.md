---
sidebar_position: 5
title: Speed up workload startup
---

# Speed up workload startup

A large container image pulled at launch can slow down — or stall — how fast a job starts,
especially for **multi-node** jobs where every node pulls the same image. Two console tools cut
that cost: **import** an image into the platform's registry, and **preheat** (pre-pull) it onto
your nodes ahead of time.

## Import an image into the registry

Importing copies an image into the platform's **internal registry**, so later jobs pull it from
inside the cluster instead of repeatedly fetching it from an external (and possibly slow or
private) source.

1. Go to **Artifacts → Images** and click **Import Image**.
2. Enter the **Source** — the full image address (e.g. `docker.io/rocm/pytorch:latest`).
3. For a **private** source, pick an image **Secret** (created under
   [Manage access & quota → Registry secret](/administration/manage-access-and-quota)).
4. **Confirm.** Import runs asynchronously; the image moves from `importing` to `ready` (large
   images take a while). Once `ready`, select it as your workload's image.

![Import Image form](/img/screenshots/image-import-form.png)

<!-- @test
scope: page
mode: behavior
priority: P2
personas: [member]
preconditions: [running-cluster, harbor-registry]
do: Artifacts > Images > Import Image; Source = docker.io/library/busybox:latest (small, fast); Confirm
expect:
  - the image appears in the Images list and its status becomes "ready" (poll; import is async)
  - on a Create PyTorch Job form, the image Select picker now lists the imported busybox
cleanup: delete the imported image via its row action in the Images list
-->

## Preheat an image onto your nodes

Preheating **pre-pulls** an image onto every node in a workspace (via a short-lived DaemonSet), so
when a job lands the image is already on disk and there is **no pull at start**. Open the
**Preheat** tab on the **Images** page to start a preheat for a workspace and watch its progress
(`0% → 100%`). Do this before a big or time-sensitive multi-node run.

You can also enable **preheat as part of submitting a workload** (in the Create form's advanced
options) to prepare the image in advance with the job.

<!-- @test
scope: page
mode: behavior
priority: P2
personas: [member]
preconditions: [running-cluster, harbor-registry, workspace-with-quota]
do: on Images > Preheat, start a preheat of the ready busybox image for the test workspace
expect:
  - a preheat job appears and its progress advances to 100% / status Completed
cleanup: none required (preheat is a one-shot job; the pre-pulled image can stay)
-->

## Import vs. preheat vs. node cache

| Approach | What it does | Use when |
|----------|--------------|----------|
| **Node cache** (default) | The first job on a node pulls the image; later jobs on that same node reuse it. | Stable nodes, no action needed. |
| **Import** | Caches the image in the in-cluster registry. | Pulling from a slow/external/private registry, or to share a private image. |
| **Preheat** | Pre-pulls the image to **all** of a workspace's nodes ahead of time. | Right before a large or time-sensitive (multi-node) launch. |

For saving your *own* environment as a reusable image, see
[Develop & interact → Save your environment as an image](/tasks/interact-with-your-job#save-your-environment-as-an-image).
