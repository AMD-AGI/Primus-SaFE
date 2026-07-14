---
sidebar_position: 9
title: FAQ
---

# FAQ

Short answers to common questions; each links to the page with the detail. This is a
Q&A reference — there is no procedure to run here.

It is written to serve two audiences at once:

- **For you (the reader):** quick answers with a pointer to the page that goes deeper.
- **For an AI agent:** this is reference material, not product behavior, so it is **n/a**
  for a behavior run. An agent only confirms the documented Q&A renders.

There is no separate test file and no invisible annotation on this page: the prose you
read is all there is. The only thing kept elsewhere is bookkeeping (priority, and any
known product bug), in the run contract `docs-site/AGENTS.md`.

> **What an agent verifies here:** confirm the documented FAQ entries render — each
> question below appears with its answer and its detail link resolves. This is
> presence checking only; there is no behavior to perform.

### Do I need a separate control plane and data plane?

No. Most teams run a **single cluster** where both sit together. One control plane can manage
several GPU clusters as a **fleet** at larger scale. See [Architecture](/architecture) and
[Install](/getting-started/install).

### Does it support NVIDIA or managed cloud?

AMD GPUs with **ROCm** are the primary, validated target. Other platforms are
community/experimental for now. See [Prerequisites](/getting-started/prerequisites).

### Can I run two different GPU types under one platform?

Yes. Model each hardware type as a **node flavor**; a workspace binds to one flavor, so jobs land
on matching hardware. For physically separate pools, run them as separate clusters in a fleet. See
[Workspace](/concepts/workspace).

### Where do my data and checkpoints live?

In your **workspace storage**, backed by the cluster's StorageClass. Write checkpoints there so
automatic failover can resume from them. See [Storage & data](/concepts/storage-and-data).

### Can I use it without the web console?

Yes. Everything is available over the **REST API** (`/api/v1/...`), and an **API key** lets scripts
and CI act without an interactive login. See
[Manage access & quota](/administration/manage-access-and-quota).

### What's the security model?

Local or SSO/OIDC identities, platform **roles** (`default`, `system-admin`,
`system-admin-readonly`) plus per-workspace access, and **audit logs**. See
[Add users & assign access](/administration/manage-users).

### How do I upgrade to a new release?

Re-run the installer's day-2 counterpart, which reuses your saved configuration. See
[Upgrading](/administration/upgrading).
