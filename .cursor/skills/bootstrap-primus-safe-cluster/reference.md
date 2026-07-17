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
| 2 | `Enter rdma nic(...)` | `safe.rdma_nic` | `rdma0,...,rdma7` |
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
5. **Ceph SC name mismatch** -- `ceph.sh` does `kubectl delete sc storage-rbd`, but the
   StorageClass it creates is named `rbd` (the secret is `storage-rbd`). The delete is a
   no-op; harmless, but the class to reference downstream is `rbd`.
6. **`bootstrap.sh` has no `set -e`** -- partial failures can continue silently. Check
   each verification step below rather than trusting a zero exit.
7. **Fresh-cluster pre-install hook** -- `install.sh` expects the
   `primus-safe-opensearch-config` secret to already exist. Pre-create it (see SKILL.md
   step 6).

## Per-stage verification commands

```bash
# After bootstrap.sh
kubectl get nodes -o wide        # every node Ready
helm list -A                     # cert-manager, amd-gpu-operator, network-operator, scheduler-plugins

# After storage
kubectl get storageclass         # a (default) class is listed

# After higress (if used)
kubectl get pods -n higress-system

# After harbor (if used)
kubectl get pods -n harbor

# After install.sh
kubectl get pods -n primus-safe  # apiserver, controllers, webhooks, db operator Running

# Console
#   nginx:   http://<any-node-ip>:30183
#   higress: https://<cluster_name>.<your-domain>
# Seeded login: root / root  (change immediately)
```

## `.env` (written by install.sh) and upgrades

`install.sh` writes `SaFE/bootstrap/.env` at the end (secrets are NOT stored there --
they live in K8s secrets `primus-safe-image`, `primus-safe-s3`, `primus-safe-sso`,
`primus-safe-opensearch-config`). `upgrade.sh` re-reads `.env` and prompts for nothing.
If you plan to run `upgrade.sh`, add `cd_require_approval=true` (or `false`) to `.env`
first -- install does not write it and upgrade expects it.
