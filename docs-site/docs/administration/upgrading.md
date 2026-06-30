---
sidebar_position: 5
title: Upgrading
---

# Upgrading

Move an existing install to a new release. The **console** (System → Deploy) is the preferred
way — it stages the new image versions, routes the change through admin approval, and keeps a
history you can roll back from. The `upgrade.sh` **script** is the equivalent for environments
without the console or for fully scripted runs.

<!-- @test
scope: page
mode: verify
priority: P2
targets: [console]
personas: [admin]
do: open System > Deploy in the console (read-only — do NOT create, approve, or roll back a deployment)
expect:
  - the Deployment Management page lists deployment requests with Status, Rollback From, Approver, and Approval Result columns
  - a Safe / Lens toggle selects which plane the request targets
  - Create Deployment opens a form to pick component image versions and a description, and states that the request needs admin approval before execution
-->

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

## From the script — `upgrade.sh`

`upgrade.sh` is the day-2 counterpart to `install.sh`: instead of prompting you again, it
**reuses the answers** you gave at install time (saved in `bootstrap/.env`) and upgrades the
platform in place. Use it when the console isn't available, or when you want a fully
non-interactive upgrade.

<!-- @test none: operational CLI procedure (no console UI, and destructive) — not agent-tested. -->

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
