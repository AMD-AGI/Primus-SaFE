---
sidebar_position: 2
title: Manage access & quota
---

# Manage access & quota

> **Status:** Draft · **Owner:** _unassigned_ · **Source:** `SaFE/docs/apis/user.md`,
> `api-key.md`, `public-key.md`, `secret.md`, `workspace.md`, `node-flavor.md`

How to hand out the credentials a workspace's members need, and control how much compute a
workspace can use. The underlying model lives in [Workspace](/concepts/workspace).

Adding teammates and granting them workspace access is covered separately in
[Add users & assign access](/administration/manage-users).

## Set quota for a workspace

A workspace's quota is **number of nodes × its node flavor**. As an admin you control it by:

- Binding the workspace to a **node flavor** (the per-node CPU/GPU/memory profile).
- Setting the node count, and adding/removing specific nodes — see
  [Manage nodes](/administration/manage-nodes).
- Choosing the **scopes** the workspace allows (`Train`, `Infer`, `Authoring`, `CICD`), the
  **queue policy** (`fifo`/`balance`), whether **preemption** is enabled, and per-scope
  **max runtime**.

In the console, all of these are set on the **Workspaces** create/edit form (**System →
Workspaces**).

<!-- screenshot: System → Workspaces → create/edit form (flavor, nodes, scopes, quota) — add image -->


> **Not yet covered (capture so we don't lose it):**
> - [ ] Concrete create/update workspace payload showing `flavorId`, `replica`, `scopes`,
>       `queuePolicy`, `enablePreempt`, `maxRuntime`.
> - [ ] How to read `totalQuota` / `usedQuota` / `availQuota` / `abnormalQuota`.

## API keys (for scripts, CI, agents)

API keys authenticate automation without a login session. Each key:

- Starts with `ak-` and is used as `Authorization: Bearer ak-...`.
- Has a TTL of 1–366 days, and an optional **IP whitelist** (IPs or CIDRs).
- Inherits the creating user's permissions.

```bash
curl -X POST https://<your-console>/api/v1/apikeys \
  -H "Authorization: Bearer <user-token>" \
  -H "Content-Type: application/json" \
  -d '{ "name": "ci-cd-pipeline", "ttlDays": 90, "whitelist": ["10.0.0.0/8"] }'
```

:::warning
The `apiKey` value is returned **only once**, at creation. Store it securely — it cannot be
retrieved again (only a masked `keyHint` is shown afterward). Deletion is a soft delete.
:::

## SSH public keys (to shell into pods)

Register an SSH **public** key to enable passwordless SSH into your workloads / dev boxes
(OpenSSH RSA, ECDSA, or Ed25519):

```bash
curl -X POST https://<your-console>/api/v1/publickeys \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{ "name": "my-laptop", "publicKey": "ssh-ed25519 AAAA..." }'
```

You can register multiple keys and enable/disable each one. See
[Interact with your job → SSH](/tasks/interact-with-your-job) for the connection command.

## Registry secret (to pull private images)

To run images from a private registry, create an **image** secret and bind it to the
workspace (in the console: **Secrets → Create**, type **image**). Image-type secrets become the
pod's `imagePullSecrets`. Passwords are Base64 encoded (`echo -n 'pw' | base64`):

<!-- screenshot: Secrets → Create dialog, type=image (empty form) — add image -->


```bash
curl -X POST https://<your-console>/api/v1/secrets \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "harbor-secret",
    "type": "image",
    "workspaceIds": ["prod-cluster-ai-team"],
    "params": [{ "server": "harbor.example.com", "username": "admin", "password": "<base64-pw>" }]
  }'
```

Secret types are `image` (registry auth), `ssh` (node login), and `general` (free-form
key/value, e.g. a `github_token`). A workspace-admin can bind a secret to a workspace so all
members can use it.

> **Not yet covered (capture so we don't lose it):**
> - [ ] **Audit logs** (`/api/v1/auditlogs`, admin-only) — who did what, for
>       security/compliance.
