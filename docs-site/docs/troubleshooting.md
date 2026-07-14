---
sidebar_position: 8
title: Troubleshooting
---

# Troubleshooting

A symptom → likely cause → fix runbook. Each entry links to the page with the full detail.
This is a diagnostic reference — there is no single procedure to run top to bottom.

It is written to serve two audiences at once:

- **For you (the reader):** find your symptom, read the likely cause, follow the fix link.
- **For an AI agent:** this is a runbook reference, not product behavior, so it is **n/a**
  for a behavior run. An agent only confirms the documented entries render.

There is no separate test file and no invisible annotation on this page: the prose you
read is all there is. The only thing kept elsewhere is bookkeeping (priority, and any
known product bug), in the run contract `docs-site/AGENTS.md`.

> **What an agent verifies here:** confirm the documented runbook renders — each symptom
> heading below appears with its causes/fix and any detail links resolve. This is presence
> checking only; there is no behavior to perform.

## A job stays `Pending` and never runs

Usual causes:

- **No free quota** — the workspace's nodes are fully used. Check capacity and free up or add
  nodes ([Manage nodes](/administration/manage-nodes)).
- **Gang scheduling can't place all pods** — a multi-node job needs all replicas to fit at once.
  Reduce replicas/GPUs or free capacity.
- **Nodes are tainted/unhealthy** — the node agent tainted a node whose health check failed (see
  below), so the scheduler skips it.

For a quick functional test on hardware with expected taints, submit with `isTolerateAll: true`.

## A node shows unhealthy / tainted

The node agent runs health checks and applies a `primus-safe.<id>` taint when one fails — often on
a cluster that lacks the hardware a monitor expects (e.g. a WekaFS, NFS, or RDMA check on a node
that has none). Identify the failing monitor and either fix the hardware or disable that monitor —
see [Manage nodes → health monitors](/administration/manage-nodes). Automatic faults are covered in
[Pre-flight & in-flight monitoring](/administration/preflight-and-monitoring).

## OOM or NCCL errors in a job

- **OOM** — raise the job's `memory` (and `sharedMemory` for data loaders).
- **NCCL/RCCL can't connect (multi-node)** — the network interface or RDMA device names don't match
  the cluster default. Confirm `NCCL_SOCKET_IFNAME` / `NCCL_IB_HCA` (set at
  [install](/getting-started/install)) and override them per job if a node type differs (see
  [Run a multi-node distributed job](/tasks/run-multi-node-training)).

## Failover didn't resume from a checkpoint

Automatic recovery resumes from your **latest checkpoint**, so checkpoints must be written to
**workspace storage** (not a pod-local path). See [Fault tolerance](/concepts/fault-tolerance) and
[Storage & data](/concepts/storage-and-data).

## WebShell (browser terminal) shows "Disconnected"

You open a running job or Authoring dev box, choose **SSH → Open WebShell**, and the terminal
immediately shows **"Disconnected"** and never connects.

**Most common cause — a self-signed / untrusted TLS certificate on the console.** A fresh install
serves the console over HTTPS with a self-signed certificate. Your browser lets you click through
the warning to load the **page**, but WebShell opens a **WebSocket** (`wss://`) back to the
console, and browsers **silently refuse** a WebSocket to an untrusted certificate — there is no
"proceed anyway" prompt for WebSockets. The socket never opens, so you see "Disconnected."

Confirm it: open the WebShell tab → browser **DevTools → Network** → the `…/webshell` request
fails immediately with **no `101 Switching Protocols`**. That points to the certificate, not the
product.

Fix (in order of preference):

- **Serve a browser-trusted certificate** for the console domain (from a CA your client machines
  already trust), instead of the default self-signed one. WebShell then connects with no per-user
  setup.
- **Trust the certificate on your machine** — import the console's certificate/CA into your OS or
  browser trust store, then reopen WebShell.
- **Use SSH on port 2222** to reach the pod from your own terminal instead — see
  [Develop & interact with your jobs → SSH](/tasks/interact-with-your-job).

**Second cause — the terminal shows `[Connected]` then immediately `[Disconnected]` (looping).**
Here the WebSocket *does* connect (so the certificate is fine), but the shell exec fails instantly
— usually because WebShell defaulted to **`bash`, which isn't installed in your image** (many
minimal/BusyBox-based images ship only `sh`). In the WebShell dialog set **Terminal Shell** to
**`sh`** and reconnect. (The apiserver log shows `exec: "bash": executable file not found in $PATH`.)
Rarer connect-then-drop causes: the workload isn't **Running** yet, or the chosen container isn't
ready — confirm the workload phase.

## A workload's phase looks stuck

The console workload **phase can lag** the real pod state (e.g. it may read `Pending` after the pod
is already `Running` or finished). Confirm the true state from the workload's pod/detail before
assuming a failure.

## Install fails on a brand-new cluster

A fresh cluster needs the OpenSearch placeholder secret created before `install.sh` — see the
prerequisite note on the [Install](/getting-started/install) page.

## Pre-flight / Bench can't reach nodes

Bench and node management connect over SSH. Confirm passwordless SSH from the deploy host to every
node and that the registered **SSH secret** is correct (see
[Install](/getting-started/install) and [Manage nodes](/administration/manage-nodes)).
