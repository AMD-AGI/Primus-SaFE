---
sidebar_position: 5
title: Upgrading
---

# Upgrading

> **Status:** TODO · **Owner:** _unassigned_
> **Purpose:** how to upgrade an existing install.

:::note Content brief
- [ ] `upgrade.sh` flow (reuses `.env`, update image tags first).
- [ ] What gets upgraded (admin plane, CRDs, RBAC, webhooks, node-agent).
- [ ] Version compatibility / upgrade-path matrix + rollback.

**Source:** `SaFE/docs/installation/upgrade.md`.

> **Not yet covered — operator/automation features (capture so we don't lose them):**
> - [ ] **CD / continuous deployment** (`/api/v1/cd`) and **GitHub workflow** integration.
> - [ ] **Fault injection** (`/api/v1/faults`) — testing/chaos.
>
> Other day-2 topics now have homes: pre-flight/OpsJobs and monitoring →
> [Pre-flight & in-flight monitoring](/administration/preflight-and-monitoring);
> node cordon/drain/reboot → [Manage nodes](/administration/manage-nodes).
:::
