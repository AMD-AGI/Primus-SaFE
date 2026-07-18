---
name: bootstrap-primus-safe-cluster
description: >-
  Bootstrap an entire Primus-SaFE cluster end to end from a single config file:
  rewrite the Kubespray inventory, provision Kubernetes, set up storage, optional
  gateway/registry, and drive SaFE's interactive install.sh non-interactively.
  Use when the user wants to install, bring up, provision, or bootstrap a new
  Primus-SaFE cluster, or mentions bootstrap.sh / install.sh / hosts.ini automation.
disable-model-invocation: true
---

# Bootstrap a Primus-SaFE cluster

Provision a new Primus-SaFE cluster end to end, driven by one filled-in config file.
The authoritative human runbook is `docs-site/docs/getting-started/install.md`; this
skill executes it non-interactively. The one fragile part -- SaFE's interactive
`install.sh` -- is handled by piping config-derived answers in exact prompt order; see
[reference.md](reference.md).

## Inputs

- A completed config file based on `cluster-config.example.yaml` (ask the user for its
  path; do not assume). Treat it as the single source of truth.
- Run from the **deploy host**: has `kubectl` + `helm`, and **passwordless root SSH to
  every node**. The repo is cloned at `Primus-SaFE/`.

## Hard rules

- **Never guess a required value.** If a required field is missing (nodes, a required
  password, `cluster_name` when `ingress: higress`), STOP and report BLOCKED naming
  exactly what is absent. A missing prerequisite is BLOCKED, not a failure to paper over.
- **Check before you run; re-run to resume.** Never assume a fresh cluster. Every step's
  verification is also its *precondition*: run the check first, and if it already passes,
  mark the step done and skip it. If it partially passes or a previous run failed midway,
  re-run that step's script -- **every script here is idempotent and safe to re-run**
  (see the detection table and idempotency notes in [reference.md](reference.md)). Do not
  tear things down to "start clean" unless the user asks.
- **Pause and verify after every long step** (Bootstrap ~20-40 min, install.sh). Run the
  step's verification (see [reference.md](reference.md)) and confirm it passes before
  continuing. Do not chain past a failed check.
- **Destructive edits happen** -- the scripts `sed` `hosts.ini`, `harbor/values.yaml`,
  and `storage/ceph/storageclass.yaml` in place. Expected; do not "fix" them.
- This is real infrastructure. Show the user each verification result.

## Workflow

Each step is **detect -> (skip if already satisfied) -> run -> verify**. Run the step's
verification check *first*; only run its script if the check fails or partially passes.
Copy this checklist and track it:

```
- [ ] 0. Preconditions
- [ ] 1. Rewrite Bootstrap/hosts.ini from nodes
- [ ] 2. bootstrap.sh -> Kubernetes + base add-ons        (verify nodes Ready)
- [ ] 3. Storage (local-path | ceph)                      (verify default SC)
- [ ] 4. (optional) Higress gateway
- [ ] 5. (optional) Harbor registry
- [ ] 6. Pre-create primus-safe-opensearch-config secret
- [ ] 7. install.sh via piped answers                     (verify pods Running)
- [ ] 8. Access the console                               (root / root)
```

### Resuming a partial install

This skill is re-runnable end to end. Before starting at step 0, **probe current state and
jump to the first incomplete step** rather than blindly re-running everything. Quick sweep:

```bash
kubectl get nodes 2>/dev/null                              # step 2 done if all Ready
kubectl get storageclass 2>/dev/null                       # step 3 done if a (default) exists
kubectl get pods -n higress-system 2>/dev/null             # step 4 (if gateway.higress)
kubectl get pods -n harbor 2>/dev/null                     # step 5 (if registry.harbor)
kubectl get secret primus-safe-opensearch-config -n primus-safe 2>/dev/null   # step 6
helm list -n primus-safe 2>/dev/null                       # step 7 releases present?
kubectl get pods -n primus-safe 2>/dev/null                # step 7 pods Running?
```

If `kubectl` cannot reach a cluster at all, start at step 1. If the cluster and storage are
up but `primus-safe` pods are missing or unhealthy, resume at step 6/7. When in doubt, re-run
the step's script -- all are idempotent (see [reference.md](reference.md), "Idempotency &
resuming a partial install"). Report which steps were skipped as already-satisfied.

### 0. Preconditions

- Read the config file. Confirm `kubectl` and `helm` exist on the deploy host.
- Confirm SSH reachability to each node (`ssh -i <ssh_key> <user>@<ansible_host> true`).
- If `safe.ingress: higress`, require `safe.cluster_name` and `gateway.higress: true`.
- If `registry.harbor.enabled`, require `registry.harbor.admin_password`.

### 1. Rewrite `Bootstrap/hosts.ini`

The checked-in file is a broken placeholder -- overwrite it entirely. Standard Ansible
inventory; control-plane/etcd nodes (odd count) also listed under `[kube_node]` so they
run workloads:

```ini
[all]
node-01 ansible_host=10.0.0.11 ip=10.0.0.11 ansible_user=root ansible_ssh_private_key_file=~/.ssh/id_ed25519
# ...one line per node from config...

[kube_control_plane]
node-01

[etcd]
node-01

[kube_node]
node-01

[k8s_cluster:children]
kube_control_plane
kube_node
```

If `network.*` overrides are set in the config, edit the matching top-of-file variables
in `Bootstrap/bootstrap.sh` (KUBE_VERSION, KUBESPRAY_VERSION, KUBE_NETWORK_PLUGIN,
KUBE_PODS_SUBNET, KUBE_SERVICE_ADDRESSES, NODE_LOCAL_DNS_IP).

### 2. Provision Kubernetes

```bash
cd Primus-SaFE/Bootstrap
bash bootstrap.sh
```

~20-40 min. Then **pause** and verify: `kubectl get nodes -o wide` (all `Ready`) and
`helm list -A` (cert-manager, amd-gpu-operator, network-operator, scheduler-plugins). If
any node is `NotReady` or the script errored, stop and surface the Kubespray/Ansible
error (usually SSH reachability or an inventory mistake) -- `bootstrap.sh` has no
`set -e`, so trust the checks, not the exit code.

### 3. Storage

`storage.type: local-path`:

```bash
cd Primus-SaFE/Bootstrap/storage/local-path
bash local-path.sh    # answer the directory prompt with storage.local_path_dir
```

`storage.type: ceph` (edit `storage/ceph/cephcluster.yaml` first if not using all
nodes/devices):

```bash
cd Primus-SaFE/Bootstrap/storage/ceph
bash ceph.sh          # the resulting default StorageClass is named `rbd`
```

Verify: `kubectl get storageclass` shows a `(default)` class. Its name must match
`safe.storage_class`.

### 4. Higress gateway (only if `gateway.higress: true`)

```bash
cd Primus-SaFE/Bootstrap/higress
bash higress.sh
```

Verify pods in `higress-system`. Required when `safe.ingress: higress`.

### 5. Harbor registry (only if `registry.harbor.enabled: true`)

First generate `Primus-SaFE/Bootstrap/harbor/hosts.yaml` (gitignored) -- an Ansible
inventory with an `[all]` group listing every node with `ansible_host` +
`ansible_ssh_user` + key, so Harbor can push its CA to each node. Then:

```bash
cd Primus-SaFE/Bootstrap/harbor
# args: <admin_password> <domain> <storage_class> <ssh_key>   (README order is wrong)
bash harbor.sh '<admin_password>' '<domain>' '<storage_class>' '<ssh_key>'
```

Always pass the password as arg 1 so it never prompts. Verify pods in `harbor`.

### 6. Pre-create the OpenSearch secret

On a fresh cluster `install.sh`'s pre-install hook expects this to exist:

```bash
kubectl create namespace primus-safe 2>/dev/null
kubectl create secret generic primus-safe-opensearch-config -n primus-safe \
  --from-literal=username=admin --from-literal=password=admin \
  --from-literal=endpoint=primus-robust-logs.primus-robust.svc.cluster.local:9200
```

### 7. Install the Primus-SaFE application

`install.sh` is the application installer (everything above only prepared the cluster). It
is **not** a single black box -- it deploys a stack in order, each as its own Helm release
in the `primus-safe` namespace (plus a separate observability namespace):

1. Secrets: `primus-safe-image`, `-s3`, `-sso`, `-opensearch-config` (+ higress TLS `default`).
2. `grafana-operator`.
3. `primus-pgo` -- the Postgres operator (the script waits for it to be `Running`).
4. `primus-safe` -- the admin plane: apiserver, job-manager, resource-manager, webhooks, controllers.
5. `primus-safe-cr` -- the custom resources that drive the platform.
6. `node-agent` -- the data plane, **only if** `safe.install_node_agent: true`.
7. `primus-safe-observability` (its own namespace) -- **only if** `safe.install_obs_logs`
   or `safe.install_obs_metrics` is true.

Build the ordered answer stream from the config (default path is 14 lines; conditional
blocks insert extra lines) and pipe it. The exact prompt->field mapping, conditional
insertions, and all-or-nothing behavior are in [reference.md](reference.md) -- read it
before assembling the stream, and recount the lines against the table.

```bash
cd Primus-SaFE/SaFE/bootstrap
printf '%s\n' <ordered answers per reference.md> | bash install.sh
```

**Idempotent / resumable.** `install.sh` runs with `set -euo pipefail` but every action is
safe to repeat: it uses `helm upgrade --install`, auto-cleans any Helm release stuck in a
`failed`/`pending-install` state before reinstalling, `kubectl apply`s all secrets, and
*keeps* an existing OpenSearch or higress TLS secret. So on a partial or failed app install,
the supported fix is simply to **re-run `install.sh` with the same piped answers** -- it
finishes whatever is missing and upgrades the rest in place. Do not uninstall first.

Verify (all should hold):

```bash
helm list -n primus-safe          # grafana-operator, primus-pgo, primus-safe, primus-safe-cr
                                  # (+ node-agent if enabled) all STATUS=deployed
kubectl get pods -n primus-safe   # apiserver, job-manager, resource-manager, webhooks,
                                  # controllers, primus-pgo + its db pod all Running
# only when logs/metrics were enabled:
kubectl get pods -n primus-safe-observability
```

If pods stay `Pending`/`CrashLoopBackOff`, it usually points to storage (no bound PVC ->
recheck step 3) or the OpenSearch secret (step 6); fix that, then re-run `install.sh`.

**`install.sh` vs `upgrade.sh`:** use `install.sh` for a first install *and* to resume/repair
a partial one -- it prompts (piped) and rebuilds every release. `upgrade.sh` is only for a
later config-preserving upgrade: it reuses `SaFE/bootstrap/.env` (written by `install.sh`),
prompts for nothing, and expects `cd_require_approval` to already be in `.env` (install does
not write it). Do not use `upgrade.sh` to resume a broken first install.

### 8. Access the console

- `nginx`: `http://<any-node-ip>:30183`
- `higress`: `https://<cluster_name>.<your-domain>`

Seeded admin is **`root` / `root`** -- tell the user to change it immediately. With
`higress` the TLS cert is self-signed (WebShell fails until trusted); note it.

## Report

End with a short status table: each step -> PASS / FAIL / BLOCKED / SKIPPED, plus the
console URL and the change-the-root-password reminder. On FAIL/BLOCKED, quote the error.
