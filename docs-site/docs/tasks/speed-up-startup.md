---
sidebar_position: 5
title: Speed up workload startup
---

# Speed up workload startup

A large container image pulled at launch can slow down — or stall — how fast a job starts,
especially for **multi-node** jobs where every node pulls the same image. Two console tools cut
that cost: **import** an image into the platform's registry, and **preheat** (pre-pull) it onto
your nodes ahead of time.

This page is written to serve two audiences at once:

- **For you (the reader):** every step says *what* to click and *what a healthy result looks
  like*, so you can follow it from an empty Images page to a ready, pre-staged image.
- **For an AI agent:** the steps are concrete and ordered, and each one states its own pass/fail
  outcome in plain language — so you can hand this page to an agent and have it perform the task
  and judge success without any hidden instructions.

There is no separate test file and no invisible annotation on this page: **the prose you read is
the test an agent runs.** The only thing kept elsewhere is bookkeeping (priority, and any known
product bug), in the run contract `docs-site/AGENTS.md`.

## Before you start

You need the following. If any is missing, this walkthrough is **blocked** — stop and get it
first, don't work around it:

- A **running cluster** you can reach through the console.
- The **in-cluster (Harbor) registry** enabled — both import and preheat stage images into it.
- For the preheat flow, a **workspace with GPU quota** whose nodes are the preheat target.

> **Agent:** confirm the registry is available before Step 1 (the **Artifacts → Images** page and
> its **Import Image** / **Preheat** controls exist). If the in-cluster registry add-on is absent,
> report **BLOCKED** (missing `harbor-registry`) rather than failing a step. Give any imported
> image a unique suffix so re-runs don't collide.

## Import an image into the registry

Importing copies an image into the platform's **internal registry**, so later jobs pull it from
inside the cluster instead of repeatedly fetching it from an external (and possibly slow or
private) source.

### Steps

1. Go to **Artifacts → Images** and click **Import Image**.
2. Enter the **Source** — the full image address. A small public image such as
   `docker.io/library/busybox:latest` imports fast and is ideal for a smoke test.
3. For a **private** source, pick an image **Secret** (created under
   [Manage access & quota → Registry secret](/administration/manage-access-and-quota)).
4. **Confirm.** Import runs asynchronously; the image moves from `importing` to `ready` (large
   images take a while). Once `ready`, select it as your workload's image.

![Import Image form](/img/screenshots/image-import-form.png)

Here is what each outcome means — this is the pass/fail for the import flow:

- **Healthy (pass):** the image appears in the **Images** list and its status advances from
  `importing` to `ready` (poll — import is async). On a **Create PyTorch Job** form, the image
  **Select** picker now lists the imported image.
- **If instead you see** the import stuck in `importing` indefinitely, or an error status, or the
  image never appears in the picker (fail): the source couldn't be pulled (typo, private source
  without a secret) or the registry is misconfigured. Fix the source/secret and retry.

> **Agent:** perform the steps, fill the table below, show it to the user, and report **PASS**
> only if the image reached `ready` and appears in the Create PyTorch Job picker. Then **run
> cleanup**: delete the imported image via its row action in the Images list, so the run is
> repeatable.

| Check | Healthy result | Found |
|---|---|---|
| Image appears in Images list | yes | _fill in_ |
| Import status | reaches `ready` (poll; async) | _fill in_ |
| Image listed in Create PyTorch Job picker | yes | _fill in_ |
| Cleanup (imported image deleted) | done | _fill in_ |

## Preheat an image onto your nodes

Preheating **pre-pulls** an image onto every node in a workspace (via a short-lived DaemonSet), so
when a job lands the image is already on disk and there is **no pull at start**. Do this before a
big or time-sensitive multi-node run.

### Steps

1. Open the **Preheat** tab on the **Images** page.
2. Start a preheat of a `ready` image (e.g. the busybox you just imported) for your test
   workspace.
3. Watch its progress advance (`0% → 100%`).

You can also enable **preheat as part of submitting a workload** (in the Create form's advanced
options) to prepare the image in advance with the job.

Here is what each outcome means — this is the pass/fail for the preheat flow:

- **Healthy (pass):** a preheat job appears and its progress advances to **100% / status
  Completed** across the workspace's nodes.
- **If instead you see** the preheat stall well short of 100%, or fail (fail): a node couldn't
  pull the image or has no room — check that the image is `ready` and the target workspace has
  schedulable nodes.

> **Agent:** perform the steps, fill the table, and report **PASS** only if preheat reached 100% /
> Completed. No cleanup is required — preheat is a one-shot job, and the pre-pulled image can stay.

| Check | Healthy result | Found |
|---|---|---|
| Preheat job appears | yes | _fill in_ |
| Progress | advances to 100% / Completed | _fill in_ |

## Import vs. preheat vs. node cache

| Approach | What it does | Use when |
|----------|--------------|----------|
| **Node cache** (default) | The first job on a node pulls the image; later jobs on that same node reuse it. | Stable nodes, no action needed. |
| **Import** | Caches the image in the in-cluster registry. | Pulling from a slow/external/private registry, or to share a private image. |
| **Preheat** | Pre-pulls the image to **all** of a workspace's nodes ahead of time. | Right before a large or time-sensitive (multi-node) launch. |

For saving your *own* environment as a reusable image, see
[Develop & interact → Save your environment as an image](/tasks/interact-with-your-job#save-your-environment-as-an-image).
