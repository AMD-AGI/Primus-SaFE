# Reference: install.sh prompt mapping, repo gotchas, verification

This is the fragile detail behind the skill. If `SaFE/bootstrap/install.sh` changes
its prompts, **the prompt-order table below is the one thing to re-check.**

## `install.sh` is interactive-only

Every parameter goes through `read` (`get_input_with_default` /
`get_secret_input_with_default`). There is **no** `--env-file`, no flags, and no
"skip if variable already set" logic. `install.sh` only *writes* `.env` at the end;
it never reads one on a fresh install. So the only zero-code-change way to automate it
is to **pipe answers to stdin in exact prompt order**. Empty input = the shown default.

## Prompt order (default path = 14 prompts)

Feed one answer per line, in this order. `<-` shows the config field; `(Enter)` means
send an empty line to accept the default.

| # | Prompt | Answer from config | Default |
|---|--------|--------------------|---------|
| 1 | `Enter ethernet nic(...)` | `safe.ethernet_nic` | `eno0` |
| 2 | `Enter rdma nic(...)` | `safe.rdma_nic` | `rdma0,...,rdma7` (see caveat below) |
| 3 | `Enter cluster scale, choose 'small/medium/large' (...)` | `safe.cluster_scale` | `small` |
| 4 | `Enter storage class(...)` | `safe.storage_class` | `local-path` |
| 5 | `Support S3 ? (y/n)` | `y` if `safe.s3.enabled` else `n` | `n` |
| 6 | `Create image pull secret ? (y/n)` | `y` if `safe.image_pull_secret.enabled` else `n` | `n` |
| 7 | `Enter the ingress name (nginx/higress)` | `safe.ingress` | `nginx` |
| 8 | `Support SSO ? (y/n)` | `y` if `safe.sso.enabled` else `n` | `n` |
| 9 | `Enter OpenSearch username (empty if not required)` | `safe.opensearch.username` | `` (empty) |
| 10 | `Enter OpenSearch password (...)` *(silent)* | `safe.opensearch.password` | `` (empty) |
| 11 | `Enter csi volume handle? (...)` | `safe.csi_volume_handle` | `` (empty) |
| 12 | `install node-agent ? (y/n)` | `y`/`n` from `safe.install_node_agent` | `n` |
| 13 | `install SaFE observability logging stack ... ? (y/n)` | `y`/`n` from `safe.install_obs_logs` | `n` |
| 14 | `install SaFE observability metrics stack ... ? (y/n)` | `y`/`n` from `safe.install_obs_metrics` | `n` |

### Conditional prompts (inserted right after their trigger)

These extra lines must be inserted **in place** when the trigger answer is `y` /
`higress`, shifting everything after them down:

- After #5 `Support S3 ? y` -> 4 lines: `s3.endpoint`, `s3.bucket`, `s3.access_key`, `s3.secret_key`
- After #6 `Create image pull secret ? y` -> 3 lines: `image_pull_secret.registry`, `.username`, `.password`
- After #7 `ingress = higress` -> 1 line: `safe.cluster_name` (prompt: `Enter cluster name(lowercase with hyphen)`, default `amd`)
- After #8 `Support SSO ? y` -> 4 lines: `sso.endpoint`, `sso.client_id`, `sso.client_secret`, `sso.redirect_uri`

Maximum path (S3 + image secret + higress + SSO all on) = **26 prompts**.

### All-or-nothing behavior (don't be surprised)

- **S3**: if the user said `y` but any of endpoint/bucket/access_key/secret_key is
  empty after the prompts, the script silently sets `s3_enable=false`.
- **SSO**: same all-or-nothing disable.
- **Image pull secret**: `y` with any blank field -> an empty placeholder secret.

### `rdma_nic` cannot be blanked from the config (prompt #2)

`get_input_with_default` treats an **empty** line as "accept the default", so piping `""`
for `safe.rdma_nic` does **not** clear it -- it sets `NCCL_IB_HCA` to the `rdma0,...,rdma7`
default. On a cluster with no RDMA NICs this silently bakes in a meaningless HCA list
(the value is a cluster-wide default; single-node jobs are unaffected, but it is
misleading). There is no piped input that yields an empty `NCCL_IB_HCA`. Options:
- Set `safe.rdma_nic` to the node's real RDMA devices (`rdma link` / `ibdev2netdev`).
- Accept the default is cosmetic on a no-RDMA / single-node cluster, or override
  `NCCL_IB_HCA` per workload later.

### Not prompted

- No admin-password prompt. The platform seeds **`root` / `root`** (change immediately).
- `lens_enable` is hardcoded `false`.

## Building the answer stream

Construct the exact ordered list first (with conditional insertions), then pipe it.
`printf` with one `%s\n` per answer is more reliable than a fragile heredoc:

```bash
cd Primus-SaFE/SaFE/bootstrap
printf '%s\n' \
  "$ETHERNET_NIC" \
  "$RDMA_NIC" \
  "$CLUSTER_SCALE" \
  "$STORAGE_CLASS" \
  "n" \
  "n" \
  "nginx" \
  "n" \
  "" \
  "" \
  "$CSI_VOLUME_HANDLE" \
  "$NODE_AGENT_YN" \
  "$OBS_LOGS_YN" \
  "$OBS_METRICS_YN" \
  | bash install.sh
```

Adjust the middle lines (and insert the conditional blocks) to match the config.
Verify each line count against the table before running -- an off-by-one shifts every
later answer.

## `install.sh` internal step map

Once the prompts are answered, `install.sh` runs through numbered internal steps, each
producing a Helm release (or secrets). Use this to locate a mid-run failure to a component
and to know what a re-run will reconcile:

| install.sh step | What it does | Helm release / namespace |
|---|---|---|
| 1 Input Parameters | reads the piped answers; auto-detects Harbor proxy if `sub_domain` set | (none) |
| 2 Secrets | creates/applies `primus-safe-image`, `-s3`, `-sso`, `-opensearch-config`, higress TLS `default` | secrets in `primus-safe` |
| 3 grafana-operator | installs the Grafana operator | `grafana-operator` |
| 4 admin plane | installs `primus-pgo` (Postgres operator, then waits for it Running), then the admin plane (apiserver, job-manager, resource-manager, webhooks, controllers) | `primus-pgo`, `primus-safe` |
| 5 primus-safe cr | applies the platform custom resources | `primus-safe-cr` |
| 6 data plane | installs node-agent -- **only if** `install_node_agent=y` | `node-agent` |
| 6b observability | installs metrics (VictoriaMetrics + exporters + enricher) and/or logs (OpenSearch + FluentBit) -- **only if** logs or metrics enabled; then mirrors the OpenSearch admin secret into `primus-safe` and restarts apiserver + resource-manager | `primus-safe-observability` (own namespace) |
| 7 done | writes `SaFE/bootstrap/.env` | (file) |

A failure at any step leaves earlier releases installed; re-running resumes from the
equivalent point because each release is reconciled with `helm upgrade --install`.

## Idempotency & resuming a partial install

Every script in this skill is safe to re-run. On a partial/failed install, detect what is
already done and re-run only the incomplete steps (or just re-run the step's script -- a
no-op if it is already satisfied).

### Per-step "already done?" detection

| Step | Detect command | Done when | If not done |
|---|---|---|---|
| 2 bootstrap | `kubectl get nodes` | every node `Ready` + base add-ons in `helm list -A` | re-run `bootstrap.sh` (idempotent, but slow -- 20-40 min) |
| 3 storage | 1Mi PVC bind smoke-test (below), not just `kubectl get storageclass` | the smoke-test PVC reaches `Bound` and the class name matches `safe.storage_class` (Ceph: OSD pods Running + `CephBlockPool` Ready) | re-run `local-path.sh` / `ceph.sh`; if a present class won't bind, that's BLOCKED, not a re-run |
| 4 higress | `kubectl get pods -n higress-system` | gateway pods `Running` | re-run `higress.sh` |
| 5 harbor | `kubectl get pods -n harbor` | Harbor pods `Running` | regenerate `harbor/hosts.yaml`, re-run `harbor.sh` (pass `$1`=password) |
| 6 opensearch secret | `kubectl get secret primus-safe-opensearch-config -n primus-safe` | secret exists | re-create it (SKILL.md step 6) |
| 7 grafana-operator | `helm status grafana-operator -n primus-safe` | `deployed` | re-run `install.sh` |
| 7 primus-pgo | `helm status primus-pgo -n primus-safe` | `deployed` + pod Running | re-run `install.sh` |
| 7 primus-safe | `helm status primus-safe -n primus-safe` | `deployed` + pods Running | re-run `install.sh` |
| 7 primus-safe-cr | `helm status primus-safe-cr -n primus-safe` | `deployed` | re-run `install.sh` |
| 7 node-agent | `helm status node-agent -n primus-safe` | `deployed` (only if enabled) | re-run `install.sh` |
| 7 observability | `kubectl get pods -n primus-safe-observability` | pods Running (only if enabled) | re-run `install.sh` |

### Why each script is safe to re-run

- **`install.sh`** -- `helm upgrade --install` for every chart; `install_or_upgrade_helm_chart`
  first `helm uninstall --no-hooks` any release stuck in `failed`/`pending-install`, then
  reinstalls. Secrets go through `kubectl apply` (create-or-update). `ensure_opensearch_secret`
  and `ensure_higress_tls_secret` **preserve** an existing secret instead of overwriting. So
  a repeat run reconciles missing pieces and upgrades the rest in place.
- **`bootstrap.sh`** -- wraps Kubespray (Ansible), which is idempotent: a re-run converges the
  cluster to the desired state. It is slow, so prefer to skip when `kubectl get nodes` already
  shows every node `Ready`. Note it has no `set -e`, so trust the checks, not the exit code.
- **Storage / higress / harbor** -- each applies manifests / Helm charts that tolerate a
  re-run; the storage scripts `delete` a same-named class before recreating it.

## Known repo bugs / gotchas to work around

1. **`Bootstrap/hosts.ini` is a broken placeholder** (duplicate `host1`, incomplete
   IPs). Fully rewrite it from `nodes`.
2. **`Bootstrap/README.md` says `hosts.yaml`** for the Kubespray inventory -- wrong.
   `bootstrap.sh` uses `hosts.ini` (`CONFIG_FILE=hosts.ini`). The `docs-site` install
   page is correct.
3. **Harbor arg order** -- README shows `harbor.sh <pwd> [domain] [ssh_key]`, but the
   script is: `$1`=password, `$2`=domain (`primus-safe.amd.com`), `$3`=**storage class**
   (`rbd`), `$4`=ssh key (`~/.ssh/id_ed25519`). Always pass `$1` to skip its prompt.
4. **`Bootstrap/harbor/hosts.yaml`** is a separate, gitignored Ansible inventory Harbor
   needs to push its CA to every node. Generate it before running `harbor.sh`.
4b. **Harbor domain is derived, not free-form.** After the prompts, `install.sh` builds
   `harbor_host="harbor.${sub_domain}.primus-safe.amd.com"` and, if it finds a Harbor
   endpoint there, sets `helm_registry` / `proxy_image_registry` to `${harbor_host}/proxy`
   (pull-through cache). So for auto-detection to work, `registry.harbor.domain` must be
   `harbor.<safe.cluster_name>.primus-safe.amd.com`. A different Harbor domain still
   installs Harbor, but install.sh won't wire it up as the proxy cache.
5. **Ceph SC name mismatch** -- `ceph.sh` does `kubectl delete sc storage-rbd`, but the
   StorageClass it creates is named `rbd` (the secret is `storage-rbd`). The delete is a
   no-op; harmless, but the class to reference downstream is `rbd`.
6. **`bootstrap.sh` has no `set -e`** -- partial failures can continue silently. Check
   each verification step below rather than trusting a zero exit.
7. **Fresh-cluster pre-install hook** -- `install.sh` expects the
   `primus-safe-opensearch-config` secret to already exist. Pre-create it (see SKILL.md
   step 6).

## Per-stage verification commands

Verify **READY**, not just existence: most install failures are pods that come up but never
become Ready (webhooks crashloop, OpenSearch `0/1`, a wrong image `ImagePullBackOff`) or
volumes that never bind. A `Running` phase with `0/1` ready, or a `deployed` helm release
with a broken pod, is still a failure. The `grep -Ev 'Running|Completed'` lines below print
nothing when healthy; anything they print needs investigation.

```bash
# After bootstrap.sh
kubectl get nodes -o wide        # every node Ready
helm list -A                     # cert-manager, amd-gpu-operator, network-operator, scheduler-plugins
kubectl get pods -A | grep -Ev 'Running|Completed'   # nothing = all add-on pods healthy

# After storage -- prove the class BINDS, don't just check it exists (a dead class,
# e.g. Ceph rbd with no OSDs, satisfies a presence check but stalls install.sh later)
kubectl get storageclass         # a (default) class is listed, name == safe.storage_class
# Ceph only: backing cluster must be healthy
kubectl get pods -n rook-ceph | grep 'rook-ceph-osd-'   # >=1 osd pod Running
kubectl get cephblockpool -A                            # PHASE Ready (not Failure)
# Functional bind smoke-test (all storage types)
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: safe-smoke-test, namespace: default}
spec:
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 1Mi}}
  storageClassName: <safe.storage_class>   # omit to use the (default) class
EOF
kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc/safe-smoke-test --timeout=60s
kubectl delete pvc safe-smoke-test
# Pending instead of Bound -> BLOCKED; read `kubectl describe pvc safe-smoke-test`

# After higress (if used)
kubectl get pods -n higress-system | grep -Ev 'Running|Completed'   # nothing = healthy

# After harbor (if used)
kubectl get pods -n harbor | grep -Ev 'Running|Completed'           # nothing = healthy

# After install.sh -- every pod READY n/n, all PVCs Bound
kubectl get pods -n primus-safe   # apiserver, controllers, webhooks, db operator -- READY n/n
kubectl get pods -n primus-safe | grep -Ev 'Running|Completed'      # nothing = healthy
kubectl get pvc -A | grep -v Bound                                 # nothing = all Bound

# Console
#   nginx:   http://<any-node-ip>:30183
#   higress: https://<cluster_name>.<your-domain>
# Seeded login: root / root  (change immediately)
```

## `install.sh` vs `upgrade.sh` (which to run when)

Both are in `SaFE/bootstrap/`. They are not interchangeable:

| | `install.sh` | `upgrade.sh` |
|---|---|---|
| Purpose | first install **and** resume/repair a partial one | later config-preserving upgrade |
| Input | interactive prompts (pipe answers) | reads `SaFE/bootstrap/.env`; prompts for nothing |
| Fresh cluster | yes | no -- fails if `.env` is missing |
| Reconciles | all releases + secrets | admin plane, cr, node-agent, observability |
| Use to resume a broken first install | **yes** | no |

`install.sh` writes `SaFE/bootstrap/.env` at the end (secrets are NOT stored there --
they live in K8s secrets `primus-safe-image`, `primus-safe-s3`, `primus-safe-sso`,
`primus-safe-opensearch-config`). `upgrade.sh` re-reads `.env` and prompts for nothing.
If you plan to run `upgrade.sh`, add `cd_require_approval=true` (or `false`) to `.env`
first -- install does not write it and upgrade expects it. (`upgrade.sh` also honors
`CALLED_BY_CD=true` to skip the node-agent step when driven by `cd-deploy.sh`.)

To resume a partial *application* install, re-run `install.sh` -- not `upgrade.sh`.
