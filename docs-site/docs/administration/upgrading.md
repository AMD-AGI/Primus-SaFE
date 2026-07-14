---
sidebar_position: 5
title: Upgrading
---

# Upgrading

Move an existing install to a new release. The **console** (System → Deploy) is the preferred
way — it stages the new image versions, routes the change through admin approval, and keeps a
history you can roll back from. The `upgrade.sh` **script** is the equivalent for environments
without the console or for fully scripted runs.

This page serves two audiences at once:

- **For you (the reader):** each section says what to click (console) or run (script) and what a
  healthy result looks like.
- **For an AI agent:** the upgrade flow is a real, executable procedure with its own healthy
  result — on a target platform you can create a deployment request, have it approved, and roll it
  back. No hidden test layer — bookkeeping lives in the run contract
  `docs-site/AGENTS.md`.

> **Agent — upgrading mutates the live platform.** Create/approve/roll back only on a platform the
> operator has told you is safe to change, and **ask first** otherwise. Whether the shared
> docs-as-test regression run skips this (and presence-checks the Deployment Management surface
> instead) is defined in the run contract's *test-scope exclusions*, not on this page.

## From the console — Deployment Management (preferred)

In the console, open **System → Deploy**. The **Deployment Management** page lists every
deployment request with its status, who approved it, and what it rolled back from. The **Safe /
Lens** toggle selects which plane a request targets.

![System → Deploy — Deployment Management](/img/screenshots/deploy-list.png)

To upgrade:

1. Click **Create Deployment**.
2. Under **Image Versions**, choose a **component** and enter the target version (e.g. `v1.2.3`
   or `latest`); use **Add Component** for each service you're moving. Leave a component out to
   keep its current version.
3. *(Optional)* Adjust **Environment Config** — click **Load Current** to start from the live
   values, then edit `KEY=value` lines. Leave it empty to keep the current configuration.
4. Enter a **Description** (required) and click **Create**.
5. An admin **approves** the request from its row; only then does it execute. Track progress in
   the **Status** column.

![Create Deployment Request](/img/screenshots/deploy-create-form.png)

> **Roll back:** create a deployment from a previous one — the originating request is recorded in
> the **Rollback From** column so you can return the platform to an earlier version.

:::note Approval gate
Requests require admin approval before they run (the install-time `cd_require_approval` setting).
A created request sits idle until an admin approves it.
:::

### Verify the upgrade outcome

After creating and approving a deployment, confirm it took effect:

- **Healthy (pass):** the request moves to a succeeded **Status**, the targeted components report
  their new versions, and the platform stays healthy (admin-plane pods `Running`, console
  reachable). A rollback returns the components to the version recorded in **Rollback From**.
- **If instead** the request stalls, errors, or the components keep their old versions, the
  upgrade didn't take effect — inspect the request status and the admin-plane pod logs.

When *not* performing an upgrade (e.g. a read-only environment or the regression run per the run
contract), the same page is a cheap presence check:

- **Healthy (pass):** the **Deployment Management** page lists requests with **Status**,
  **Rollback From**, **Approver**, and **Approval Result** columns; a **Safe / Lens** toggle
  selects the target plane; and **Create Deployment** opens a form to pick component image versions
  and a description, stating that the request needs **admin approval** before execution.
- **If instead** the page or any of those controls is missing, that's a fail.

> **Agent:** before you start, confirm a running cluster reachable as **admin** (else **BLOCKED**).
> If the operator has cleared an upgrade on a safe platform, perform Create → approve → verify and
> clean up by rolling back; otherwise presence-check the surface above and report PASS/FAIL.

| Check | Healthy result | Found |
|---|---|---|
| Deployment list columns | Status, Rollback From, Approver, Approval Result | _fill in_ |
| Plane selector | Safe / Lens toggle present | _fill in_ |
| Create Deployment form | picks component versions + description | _fill in_ |
| Approval gate stated | form says request needs admin approval | _fill in_ |

## From the script — `upgrade.sh`

`upgrade.sh` is the day-2 counterpart to `install.sh`: instead of prompting you again, it
**reuses the answers** you gave at install time (saved in `bootstrap/.env`) and upgrades the
platform in place. Use it when the console isn't available, or when you want a fully
non-interactive upgrade.

This is the CLI equivalent of the console flow above: on a host with the repo and the install-time
`.env`, it upgrades the platform non-interactively. An agent can run it on a target the operator
has cleared for upgrade (it is destructive — ask first on anything shared).

### Before you upgrade

- `install.sh` has been run successfully and produced a `.env` file in `SaFE/bootstrap/`.
- `helm` and `kubectl` are available with cluster-admin access.
- The repository and your `.env` are otherwise unchanged (the script reads `.env`).
- If you pull platform images from your **own registry**, build and push the new image tags
  first, and set `proxy_image_registry` (and `helm_registry`) in `.env`.
- **Back up** your `.env` and test the upgrade in a non-production environment first.

### Run it

```bash
cd Primus-SaFE/SaFE/bootstrap
bash upgrade.sh
```

### What it upgrades

The script is non-interactive and upgrades, in order:

1. **Admin plane** (`primus-safe`) — the apiserver, controllers, and webhooks. When the release
   is already installed it also replaces the **CRDs, RBAC role, and webhook** manifests, then
   runs `helm upgrade`. Control-plane replicas and resources are sized from your `cluster_scale`.
2. **Custom resources** (`primus-safe-cr`) — the seeded platform resources.
3. **Data plane** (`node-agent`) — the per-node agent on your GPU nodes (skipped if
   `install_node_agent=n`).

Unlike install, it does **not** re-create secrets or re-install the database/Grafana operators —
it only upgrades the components above using your existing configuration.

### Roll back

Each component is a Helm release, so you can roll back to the previous revision if needed:

```bash
helm history primus-safe -n primus-safe
helm rollback primus-safe <revision> -n primus-safe
```

For advanced `.env` tuning keys available at upgrade time, see the upgrade reference in the
repository (`SaFE/docs/installation/upgrade.md`).
