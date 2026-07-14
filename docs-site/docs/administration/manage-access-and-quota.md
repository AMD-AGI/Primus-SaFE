---
sidebar_position: 2
title: Manage access & quota
---

# Manage access & quota

How to hand out the credentials a workspace's members need, and control how much compute a
workspace can use. The underlying model lives in [Workspace](/concepts/workspace).

Adding teammates and granting them workspace access is covered separately in
[Add users & assign access](/administration/manage-users).

This page serves two audiences at once:

- **For you (the reader):** each section says what to set and what a healthy result looks like,
  from quota knobs to minting credentials.
- **For an AI agent:** each section is a real admin action with its own healthy result — set a
  workspace's quota, mint an API key, register an SSH key, create a registry secret — so you can
  perform any of them on a target cluster. The **API-key golden path** is written out step by step
  as the worked example. No hidden test layer — bookkeeping lives in the run contract
  `docs-site/AGENTS.md`.

## Set quota for a workspace

A workspace's quota is **number of nodes × its node flavor**. As an admin you control it by:

- Binding the workspace to a **node flavor** (the per-node CPU/GPU/memory profile).
- Setting the node count, and adding/removing specific nodes — see
  [Manage nodes](/administration/manage-nodes).
- Choosing the **scopes** the workspace allows (`Train`, `Infer`, `Authoring`, `CICD`), the
  **queue policy** (`fifo`/`balance`), whether **preemption** is enabled, and per-scope
  **max runtime**.

In the console, all of these are set on the **Workspaces** create/edit form (**System →
Workspaces**): open a workspace, adjust its flavor / node count / scopes / queue policy /
preemption / max-runtime, and save.

**Healthy (pass):** the saved change is reflected on the workspace — the new flavor, node count,
or scopes appear on its detail, and its `totalQuota` updates to `nodes × flavor`. **If instead**
the save is rejected or the quota doesn't change, the edit didn't take effect.

> **Agent:** editing a workspace changes a live tenant's capacity — perform it only on a
> workspace the operator has designated for testing, and **ask before editing** a real one.
> Whether the regression run mutates here or only presence-checks the form controls is set in the
> run contract's test-scope exclusions.

## API keys (for scripts, CI, agents) — golden path {#api-keys-for-scripts-ci-agents}

API keys authenticate automation without a login session. Each key:

- Starts with `ak-` and is used as `Authorization: Bearer ak-...`.
- Has a TTL of 1–366 days, and an optional **IP whitelist** (IPs or CIDRs).
- Inherits the creating user's permissions.

This is the executable walkthrough an agent performs end to end: create a key in the console,
confirm it is shown only as a masked hint, then delete it.

### Before you start

- A **running cluster** you can reach, signed in as any member.

> **Agent:** confirm you can reach the console and are signed in before Step 1. If not, report
> **BLOCKED** (missing `running-cluster`) rather than failing a step. **Never print the key
> value** — treat it as a secret; only confirm the masked hint.

### Step 1 — Create a key

In the console, go to **API Keys → Create**, set a name, a TTL (1–366 days), and an optional IP
allowlist. Use a unique name so repeated runs don't collide (an agent names it per the run
contract's cleanup convention), and a short TTL. The key value is shown **once** at creation —
copy it then (do not print it).

The equivalent API call:

```bash
curl -X POST https://<your-console>/api/v1/apikeys \
  -H "Authorization: Bearer <user-token>" \
  -H "Content-Type: application/json" \
  -d '{ "name": "ci-cd-pipeline", "ttlDays": 90, "whitelist": ["10.0.0.0/8"] }'
```

:::warning
The `apiKey` value is displayed **only once**, at creation. Store it securely — it cannot be
retrieved again (only a masked `keyHint` is shown afterward).
:::

### Step 2 — Confirm it landed (masked)

Return to the **API Keys** list.

- **Healthy (pass):** the new key appears in the list, shown **only as a masked hint** — never
  the full secret.
- **If instead** the key is missing from the list, creation didn't take effect; if the full
  secret is displayed in the list, that's a security defect — either is a fail.

### What you should see

> **Agent:** fill the table, show it, and report **PASS** only if the key appears masked. Then
> **run cleanup**: delete the created API key via its row action.

| Check | Healthy result | Found |
|---|---|---|
| New key appears in API Keys list | yes | _fill in_ |
| Key shown as masked hint (not full secret) | yes | _fill in_ |
| Cleanup (key deleted) | done | _fill in_ |

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
