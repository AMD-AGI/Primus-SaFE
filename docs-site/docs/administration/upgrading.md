---
sidebar_position: 5
title: Upgrading
---

# Upgrading

Move an existing install to a new release with `upgrade.sh`. It is the day-2 counterpart to
`install.sh`: instead of prompting you again, it **reuses the answers** you gave at install time
(saved in `bootstrap/.env`) and upgrades the platform in place.

<!-- @test none: operational CLI procedure (no console UI, and destructive) — not agent-tested. -->

## Before you upgrade

- `install.sh` has been run successfully and produced a `.env` file in `SaFE/bootstrap/`.
- `helm` and `kubectl` are available with cluster-admin access.
- The repository and your `.env` are otherwise unchanged (the script reads `.env`).
- If you pull platform images from your **own registry**, build and push the new image tags first,
  and set `proxy_image_registry` (and `helm_registry`) in `.env`.
- **Back up** your `.env` and test the upgrade in a non-production environment first.

## Run it

```bash
cd Primus-SaFE/SaFE/bootstrap
bash upgrade.sh
```

## What it upgrades

The script is non-interactive and upgrades, in order:

1. **Admin plane** (`primus-safe`) — the apiserver, controllers, and webhooks. When the release is
   already installed it also replaces the **CRDs, RBAC role, and webhook** manifests, then runs
   `helm upgrade`. Control-plane replicas and resources are sized from your `cluster_scale`.
2. **Custom resources** (`primus-safe-cr`) — the seeded platform resources.
3. **Data plane** (`node-agent`) — the per-node agent on your GPU nodes (skipped if
   `install_node_agent=n`).

Unlike install, it does **not** re-create secrets or re-install the database/Grafana operators —
it only upgrades the components above using your existing configuration.

## Roll back

Each component is a Helm release, so you can roll back to the previous revision if needed:

```bash
helm history primus-safe -n primus-safe
helm rollback primus-safe <revision> -n primus-safe
```

For advanced `.env` tuning keys available at upgrade time, see the upgrade reference in the
repository (`SaFE/docs/installation/upgrade.md`).
