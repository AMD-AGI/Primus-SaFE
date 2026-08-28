---
sidebar_position: 3
title: Develop & interact with your jobs
---

# Develop & interact with your jobs

Once a workload is running — a training job, or an **Authoring** dev box — you can watch it, read
its logs, and **shell into the pod**. All of this goes through the platform, **scoped to your
pod**: you get an interactive shell on GPU hardware without any cluster or `kubeconfig` access.

## Authoring: a personal dev box

**Authoring** is a personal, long-running **dev box** on the cluster — a single-node pod kept
alive for you (its entry point is `tail -f /dev/null`), with GPUs, your code, and workspace storage
mounted. Use it to prototype, debug, and prepare your environment before launching a full
(often multi-node) [training job](/tasks/run-multi-node-training). Because it's an interactive
session rather than a job to complete, retries, failover, and hang detection are off.

It's the easiest way to give someone hands-on GPU access **without granting them access to the
cluster** — they work inside their own pod, nothing more.

### Start a dev box (console)

Your workspace must include the **Authoring** scope (ask a workspace admin if you don't see it —
see [Workspace](/concepts/workspace)).

1. Go to **Workloads → Authoring** and click **Create Authoring**.
2. Set a **name**, an **image** (**Select** a registered image or **Custom** to type any pullable
   reference), and the **resources** (CPU, GPU, memory, ephemeral storage). A dev box is always a
   single node, so `replica` is fixed at 1. A custom reference without a registry host defaults to
   `docker.io`; you can also import images into the self-hosted Harbor registry (the **Images**
   tab) for faster pulls — see [Speed up workload startup](/tasks/speed-up-startup).
3. **Submit.** When it reaches `Running`, connect to it (below).

![Create Authoring form](/img/screenshots/authoring-create-form.png)

<!-- @test
scope: page
mode: behavior
priority: P0
personas: [member]
preconditions: [running-cluster, workspace-with-authoring-scope]
do: follow "Start a dev box (console)" to create an Authoring dev box (use a pullable image); when it reaches Running, open its WebShell terminal from the console and run a command (e.g. `hostname`)
expect:
  - the dev box appears in the Authoring list and reaches Running
  - WebShell opens an interactive terminal inside the pod and the command returns output (you can shell into the pod)
cleanup: stop or delete the dev box via its row action
-->

### Save your environment as an image

Once you've set up the box (packages, configs, dependencies), use **Save Image** to snapshot the
container into a reusable image in the registry — so you don't rebuild the environment next time.
The saved image is then selectable as the **image** for your next job or dev box.

<!-- @test todo:
  - "Document the exact Save Image console flow and Resume-a-stopped-box flow, then add behavior steps for them."
-->

## Connect to a running pod

Both WebShell and SSH go through the API server, which `exec`s into your container. Sessions are
**authenticated and audit-logged**, and **scoped to your pod** — no cluster access required. This
works for an Authoring dev box and for any running training/inference job pod.

### WebShell (browser)

Open a terminal on a pod directly from the console (the workload's detail page → **WebShell**).
It's a WebSocket terminal supporting `bash` / `sh` / `zsh`. Limits: up to 10 concurrent sessions
per user, auto-disconnect after 30 minutes idle, and no file upload/download (use volume mounts).

### SSH (terminal, port 2222)

1. Register your SSH **public** key once — see
   [Manage access & quota → SSH public keys](/administration/manage-access-and-quota).
2. Connect through the SaFE SSH gateway on **port 2222**, using the connect command shown on the
   workload's detail page:

```bash
ssh <user>.<podId>.<workspace>@<gateway-host> -p 2222
```

This also works with `scp` and VS Code Remote-SSH, so you can treat a dev box (or any job pod)
like a remote development machine.

#### Port forwarding

Both directions of standard SSH port forwarding are supported:

- `-L` (**local forward**) — reach a port inside the pod from your machine, e.g. a notebook or a
  training dashboard: `ssh -L 8888:127.0.0.1:8888 <user>.<podId>.<workspace>@<gateway-host> -p 2222`
- `-R` (**remote forward**) — let processes inside the pod reach a service on your machine. The
  listen socket is created **inside your pod**, exactly as it would be on a normal dev box.

The common use for `-R` is routing pod egress through a proxy on your laptop, so that pod-resident
tools (`git`, `gh`, `pip`) reach services that only allowlist your local network:

```ssh-config
Host my-dev-box
  HostName <gateway-host>
  User <user>.<podId>.<workspace>
  Port 2222
  ServerAliveInterval 60
  RemoteForward 127.0.0.1:7890 127.0.0.1:7890
```

With a real proxy listening on `127.0.0.1:7890` on your machine, inside the pod:

```bash
export HTTPS_PROXY=socks5://127.0.0.1:7890
curl -x socks5h://127.0.0.1:7890 -I https://api.github.com/
```

The two ports do not have to match — `RemoteForward 127.0.0.1:10800 127.0.0.1:7890`
would make the same proxy reachable inside the pod as `127.0.0.1:10800` instead.

Requirements and limits for `-R`:

- It is on by default. A platform administrator can turn it off for a cluster with
  `ssh.reverse_forward.enable: false`, in which case `-R` is refused.
- The workload image must contain `socat` — the pod-side listener is built from it.
- The pod-side listener may only bind `127.0.0.1`, so no other workload can use your tunnel.
- The listen port must fall inside the configured range (`1024`–`65535` by default, so that a
  forward cannot shadow a privileged service inside your pod), and a session may hold at most 8
  forwards. An administrator may narrow that range further.
- Server-allocated ports (`RemoteForward 0 …`) are not supported; ask for an explicit port.
- The proxy on your machine must be running, and the listener disappears as soon as the SSH session
  disconnects.

If all you need is access to a service that allowlists source IPs, consider asking for the cluster
egress CIDR to be allowlisted instead — it needs no session to stay open.

<!-- @test todo:
  - "Also verify terminal SSH (port 2222): register an SSH public key, then connect with the command from the workload detail. This needs an SSH client outside the browser, so it is not part of the UI-only run."
-->

## Status & logs

Open the workload's **detail page** in the console to watch its phase
(`Pending` → `Running` → `Succeeded` / `Failed`, also `Stopped`), see per-pod status and the
nodes in use, and read each pod's logs.

The same data is available over the REST API for automation — the workload detail
(`GET /api/v1/workloads/{workloadId}`) returns each pod's `phase`, `nodeName`, a ready-to-use SSH
command, and the `conditions` explaining scheduling decisions; a separate endpoint returns pod
logs.

## Dashboards

The platform ships Grafana dashboards for cluster- and job-level metrics (GPU utilization,
memory, network, temperatures); per-workload, it also reports `avgGpuUsage`. Open them from the
console.

## Get results out

<!-- @test todo:
  - "Document where results go: checkpoints/outputs on workspace PFS, the dump-log and download flows, and any S3 export. Then state the user-facing steps."
-->

Checkpoints and outputs are written to your workspace storage. (The full results/export flow —
PFS layout, log dump, and artifact download — is being documented; see
[Storage & data](/concepts/storage-and-data).)
