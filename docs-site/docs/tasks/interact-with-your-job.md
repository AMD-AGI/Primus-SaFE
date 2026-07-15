---
sidebar_position: 3
title: Develop & interact with your jobs
---

# Develop & interact with your jobs

Once a workload is running — a training job, or an **Authoring** dev box — you can watch it, read
its logs, and **shell into the pod**, all through the platform and **scoped to your pod**: an
interactive shell on GPU hardware with no cluster or `kubeconfig` access.

Like the other task pages, the prose is the walkthrough for **both** a human and an
agent, with expectations stated inline and no invisible test layer. Bookkeeping lives in the run
contract `docs-site/AGENTS.md`.

This page has one golden path an agent should perform end to end — **create an Authoring dev box
and shell into it** (Steps 1–3) — plus reference sections a human reads but the agent only checks
for presence.

## Authoring: a personal dev box

**Authoring** is a personal, long-running **dev box** on the cluster — a single-node pod kept
alive for you (entry point `sleep infinity`) with GPUs, your code, and workspace storage mounted.
Use it to prototype and debug before launching a full [training job](/tasks/run-multi-node-training).
Because it's an interactive session, retries, failover, and hang detection are off. It's the
easiest way to give someone hands-on GPU access **without** granting them cluster access.

### Before you start

Your workspace must include the **Authoring** scope. If you don't see the Authoring tab, that's
the signal to ask a workspace admin — see [Workspace](/concepts/workspace).

> **Agent:** the golden path needs `workspace-with-authoring-scope`. If the Authoring tab is
> absent, report **BLOCKED** rather than failing a step.

### Step 1 — Create the dev box

1. Go to **Workloads → Authoring** and click **Create Authoring**.
2. Set a **name** (use a unique suffix for repeatable runs), an **image** (**Select** a registered
   image or **Custom** for any pullable reference), and **resources** (CPU, GPU, memory, ephemeral
   storage). A dev box is always single-node, so `replica` is fixed at 1.
3. Click **Submit**.

![Create Authoring form](/img/screenshots/authoring-create-form.png)

### Step 2 — Wait for Running

The dev box appears in the **Authoring** list. A healthy box reaches **Running** within a minute
or two (longer on a first, cold image pull). If it stays **Pending** well past the pull, that's
the same signal as elsewhere — no quota or no schedulable node — not a step you did wrong.

### Step 3 — Shell in and prove it

When it's **Running**, open its detail page and click **WebShell**. What the outcomes mean —
this is the pass/fail:

- **Healthy (pass):** WebShell opens an interactive terminal **inside the pod**, and a command
  such as `hostname` returns output. That output is the proof you have a real shell on the pod's
  GPU host.
- **Fail:** the terminal never connects, or shows an auth/permission error instead of a prompt.

> **Agent:** perform Steps 1–3, fill the table, and **PASS** only if WebShell returned live
> command output from inside the pod. Then **clean up**: stop or delete the dev box via its row
> action.

| Check | Healthy result | Found |
|---|---|---|
| Dev box appears in Authoring list | yes | _fill in_ |
| Phase | reaches Running | _fill in_ |
| WebShell opens a terminal in the pod | yes | _fill in_ |
| A command (e.g. `hostname`) returns output | yes | _fill in_ |
| Cleanup (box stopped/deleted) | done | _fill in_ |

### Save your environment as an image

Once the box is set up (packages, configs, dependencies), use **Save Image** to snapshot the
container into a reusable image in the registry, selectable as the **image** for your next job or
dev box. *(An agent should verify the **Save Image** control is present.)*

## Connect to a running pod

Both WebShell and SSH go through the API server, which `exec`s into your container. Sessions are
**authenticated and audit-logged** and **scoped to your pod** — no cluster access. This works for
an Authoring dev box and for any running training/inference job pod.

### WebShell (browser)

Open a terminal on a pod from the console (workload detail → **WebShell**): a WebSocket terminal
supporting `bash` / `sh` / `zsh`. Limits: up to 10 concurrent sessions per user, auto-disconnect
after 30 minutes idle, and no file upload/download (use volume mounts).

:::note WebShell shows "Disconnected"?
Because WebShell uses a WebSocket, a browser will silently refuse it if the console is served with
an untrusted (self-signed) TLS certificate — even though the page itself loaded. Use a
browser-trusted certificate for the console (or trust the cert on your machine), or fall back to
SSH below. See [Troubleshooting → WebShell shows "Disconnected"](/troubleshooting).
:::

### SSH (terminal, port 2222)

1. Register your SSH **public** key once — see
   [Manage access & quota → SSH public keys](/administration/manage-access-and-quota).
2. Connect through the SaFE SSH gateway on **port 2222** using the command shown on the workload's
   detail page:

```bash
ssh <user>.<podId>.<workspace>@<gateway-host> -p 2222
```

This also works with `scp` and VS Code Remote-SSH — you connect from your own terminal, so a
running box behaves like a remote development machine.

**Healthy (pass):** the connect command shown on the detail page opens a shell on the pod from
your terminal. **If instead** the connection is refused, check that your SSH **public** key is
registered and enabled.

## Status & logs

Open the workload's **detail page** to watch its phase (`Pending` → `Running` →
`Succeeded` / `Failed`, also `Stopped`), see per-pod status and the nodes in use, and read each
pod's logs. The same data is on the REST API (`GET /api/v1/workloads/{workloadId}`) for
automation.

## Dashboards

The platform ships Grafana dashboards for cluster- and job-level metrics (GPU utilization,
memory, network, temperatures); per-workload it also reports `avgGpuUsage`. Open them from the
console.

## Get results out

Checkpoints and outputs are written to your workspace storage — see
[Storage & data](/concepts/storage-and-data) for the PFS layout and export options.
