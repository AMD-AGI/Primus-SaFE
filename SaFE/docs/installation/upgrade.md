## 🔄 Upgrade Script Explanation

The [upgrade.sh](../../bootstrap/upgrade.sh) script provides a mechanism to upgrade the Primus-SaFE system after the initial installation has been completed using [install.sh](../../bootstrap/install.sh).

**Prerequisites:** `helm` and `kubectl` must be installed (and `jq`, only for the `auth_*` keys below and for a release that is already external).

### 📋 Prerequisites

Before running the upgrade script, ensure that:

1. [install.sh](../../bootstrap/install.sh) has been executed successfully and generated a `.env` file in `bootstrap/`
2. The environment configuration and code directory remain unchanged (the script reads `.env`)
3. Custom images have been built and pushed to your registry (if needed)
4. For custom image registry, add `proxy_image_registry` to `.env` before upgrading

### 🛠️ Preparing for Upgrade

#### Building and Pushing Custom Images

Users need to build and push their custom images to a container registry before upgrading:

```bash
docker buildx build . -t your-image-registry/primussafe/resource-manager:version -f ./SaFE/resource-manager/installer/Dockerfile --push
```

Replace `your-image-registry` with your actual container registry address and the appropriate version tag.

#### Extending .env for Upgrade

The upgrade script reads from `.env`. [install.sh](../../bootstrap/install.sh) produces: `ethernet_nic`, `rdma_nic`, `cluster_scale`, `storage_class`, `lens_enable`, `s3_enable`, `sso_enable`, `ingress`, `sub_domain`, `install_node_agent`, `csi_volume_handle`.

The upgrade script also expects `proxy_image_registry`, `helm_registry`, and `cd_require_approval`. If these are missing from `.env`, add them (e.g. `proxy_image_registry=`, `helm_registry=registry-1.docker.io`, `cd_require_approval=true`) to avoid errors.

For additional upgrade-specific behavior, you may add these optional keys to `.env`:

| Key | Description |
|-----|-------------|
| `proxy_image_registry` | Image registry for management components (overrides chart default) |
| `helm_registry` | Helm chart registry for addons (e.g. `registry-1.docker.io`) |
| `cd_require_approval` | CD deployment approval: `true` or `false` |
| `optimize_max_concurrent` | Per-workspace cap on concurrently running optimization tasks (integer, e.g. `1024`). Defaults to chart value `1024` when unset. Synced into both `values.yaml` and the live apiserver ConfigMap. |
| `sandbox_extra_ca_secret` | Name of a Secret in `primus-safe` holding extra CA certificates that sandboxes must trust (annotate it with `primus-safe.workspace.selector: primus-safe.workspace.scope.sandbox=true` and `primus-safe.secret.broadcast: "true"` (**never on a credential**: anyone who can edit a workspace's scopes can make that workspace match the selector, so this is the only protection) to reach every workspace that runs sandboxes, existing ones included, since the mirror derives the label from each workspace's scopes (`kubectl -l` may disagree for a workspace not updated since the label existed); only that label may be selected on; a `primus-safe.workspace.ids` list still works alongside; an invalid selector freezes mirroring instead of removing copies and is reported in `primus-safe.workspace.selector.error`; removing the broadcast annotation freezes the selector the same way: copies already made are kept and their content stays in sync, nothing new is mirrored, nothing is removed, so to stop mirroring remove the selector annotation; the list of the selector's copies is kept by resource-manager in the `primus-safe.workspace.selector.applied` annotation and has a size limit, so with thousands of workspaces this must be reworked first; if that record is missing or damaged, for example after `kubectl replace`, every existing copy is kept as it is, ids removals included, and only the ids list is updated: do not edit or delete it by hand, fix the selector so resource-manager rewrites it, or remove the selector annotation to stop mirroring). Rolling resource-manager back to a release without selector support is not safe: it clears copies reached only by the selector, so add the workspaces to `primus-safe.workspace.ids` first. Sets `sandbox.extra_ca_secret`; unset renders no extra CA. |
| `sandbox_env_secret` | `sandbox-env` (the only accepted value) gives every sandbox the keys of the Secret `sandbox-env` in its workspace namespace as environment variables (`envFrom`, optional); empty or unset injects nothing. The apiserver reserves the name for admins whether or not this is set (see [Secret API](../apis/secret.md#sandbox-environment-secret), which also says what to check before setting it); on a site running an older version, set it in an upgrade after the one that installs this version. A fresh install can pass it to install.sh instead (`sandbox_env_secret=sandbox-env ./install.sh`), which writes it to `.env`. Sets `sandbox.env_secret`. |
| `web_root_redirect` | Where the console's nginx sends a browser that opens the bare `/` of the console's host: a path on that host, e.g. `/portal/`. Only a `GET` or `HEAD` of `/` without a query string is redirected; `/` with one, where sign-ins come back, still loads the console. Unset or empty keeps the console at `/`. Sets `web.root_redirect`. |
| `sandbox_admission_actor` | Client id of the sandbox control plane (`act.sub` of the workload assertion). When set, sandbox admission is turned on (`sandbox.admission.enabled` and `sandbox.admission.actor`). When omitted, admission stays off. The key set URL and the issuer are `https://<sub_domain>.<domain>/.well-known/jwks.json` and `https://<sub_domain>.<domain>`; they are not set here. A fresh install can pass it to install.sh (`sandbox_admission_actor=svc-... ./install.sh`), which writes it to `.env`. |
| `external_enable` | Enable external execution (`true`/`false`). Synced into helm values and the live job-manager ConfigMap. |
| `external_user_dir_mounts` | `true`/`false`: sets `external_execution.user_dir_mounts` (external Pods mount only their own user directory from a hostpath volume with `enableUserDir`). Unset keeps the running release's value, so an upgrade never turns it off; a fresh install gets the chart default `false`. An upgrade whose release values cannot be read stops before changing anything. |
| `tracing_enable` | Enable OpenTelemetry tracing: `true` or `false` |
| `tracing_mode` | Tracing mode: `all` or `error_only` |
| `tracing_sampling_ratio` | Sampling ratio (e.g. `1.0`) |
| `tracing_otlp_endpoint` | OTLP collector endpoint |
| `proxy_services` | YAML list for proxy services configuration |
| `auth_mode` | `builtin` or `external`. Sets `auth.mode`. |
| `auth_login_url` | Where the console sends users to sign in in external mode. Sets `auth.login_url`. |
| `auth_client_secret` | Name of the Secret that holds the client secret of an introspection authenticator. Sets `auth.client_secret`. |
| `auth_authenticators` | JSON array, the authenticator chain. Single-quote it in `.env`. Sets `auth.authenticators`. |
| `auth_group_mappings` | JSON array, identity groups to roles. Sets `auth.group_mappings`. |
| `auth_key_actors` | JSON array of identity client ids. Sets `auth.key_actors`. |
| `auth_api_keys_url` | In external mode, the path on the console's origin where something other than the chart routes API key creation to the identity provider, for example `/api/v1/apikeys`. Sets `auth.api_keys_url`. |
| `auth_api_keys_upstream` | In external mode, the identity provider's service as `host[:port]` (the full in-cluster name): the console's nginx then routes `/api/v1/apikeys` to it. Sets `auth.api_keys_upstream`. |
| `auth_scim_enabled` | `true` or `false`. Sets `auth.scim.enabled`. The SCIM endpoint is served only when this is `true` and `auth.mode` is `external`. |
| `auth_scim_identity_url` | Identity service root. Sets `auth.scim.identity_url`. Empty derives it from the introspection authenticator. |
| `auth_scim_public_url` | Base URL the identity service calls for SCIM. Sets `auth.scim.public_url`. |
| `auth_scim_token_secret` | Name of the Secret that holds the SCIM bearer token. Sets `auth.scim.token_secret`. |

The `auth_*` keys persist [external authentication](../apis/external-authentication.md#keeping-the-settings-across-upgrades) across upgrades and CD runs; they need `jq`, except `auth_mode=builtin` on its own. A release that is not external upgrades without `jq` as before. Omitting `auth_mode` renders the chart default `builtin`, including on a release that is already external. An upgrade whose `.env` sets `auth_mode=external` but drops a key the release has exits non-zero before changing anything. Omitting `sso_enable` renders `true`. Omitting `oidc_login_enable` renders `false`. Omitting `sso_bridge_allowlist` renders `[]`.

With Higress, `auth.mode: external` renders Ingress `primus-identity-login` in `auth.identity_client_secret_namespace` (default `primus-identity`). It sends `POST /api/login` and `POST /api/logout` on the console host to the identity service. A builtin upgrade removes that Ingress. One of the same name that was created outside the release is deleted on a builtin upgrade, and labeled as this release's on an external upgrade so Helm adopts it. The same external upgrade renders Ingress `<release>-scim` in the release namespace when SCIM is enabled. It sends the console host's `/scim/v2` to the apiserver. A builtin upgrade, or `auth_scim_enabled=false`, removes it. An Ingress of that name created outside the release is adopted or deleted the same way. An Ingress of either name that another Helm release owns is not changed: an upgrade that renders it stops with an error, and one that does not render it leaves it in place.

> resource-manager adds a finalizer to every Secret that has workspace mirror targets, including ones made by hand with `kubectl`, and removes it again once the Secret has no targets and its copies are gone (for example after the selector annotation is deleted). Before `helm uninstall` and deleting the `primus-safe` namespace, delete such Secrets or remove the annotations and wait for the finalizer to go, or the namespace stays in `Terminating`.

### 🚀 Upgrade Process

The script performs the following steps:

1. **Step 1: Load Parameters from .env**
   - Reads configuration from `.env` (ethernet/rdma NICs, cluster scale, storage class, lens/s3/sso flags, ingress, sub_domain, install_node_agent, csi_volume_handle, and optional upgrade keys)
   - Derives resources by cluster scale:
     - small: replicas 1, cpu 2000m, memory 4Gi
     - medium: replicas 2, cpu 8000m, memory 8Gi
     - large: replicas 2, cpu 32000m, memory 16Gi

2. **Step 2: Upgrade Admin Plane (primus-safe)**
   - Generates a temporary override from `primus-safe/values.yaml` and applies:
     - `global.image_registry` from `proxy_image_registry` (when set)
     - `csi_volume_handle`, NCCL configs (`nccl_socket_ifname`, `nccl_ib_hca`)
     - `replicas`, `cpu`, `memory`, `storage_class`
     - Image pull secret name (`primus-safe-image`)
     - Ingress selection; when `higress`, sets `sub_domain`
     - `opensearch.enable` and `grafana.enable` are always set to `false` (data-plane observability is managed via Robust AddonTemplate)
     - `s3.enable` and `s3.secret` when S3 is enabled
     - `sso.enable` and `sso.secret` when SSO is enabled
     - `cd.require_approval` from `cd_require_approval`
     - `model_optimization.max_concurrent` from `optimize_max_concurrent` (when set)
     - `sandbox.extra_ca_secret` from `sandbox_extra_ca_secret` (when set)
     - `sandbox.env_secret` from `sandbox_env_secret` (when present, including empty)
     - `web.root_redirect` from `web_root_redirect` (when set)
     - `sandbox.admission.enabled` and `sandbox.admission.actor` from `sandbox_admission_actor` (when set)
     - Tracing config when `tracing_enable=true`
     - `proxy.services` when `proxy_services` is set
   - If `primus-safe` is already installed: replaces CRDs, RBAC role, and webhooks manifests
   - Runs `helm upgrade -i primus-safe`

3. **Step 3: Upgrade primus-safe-cr**
   - Applies `helm_registry` when set
   - Runs `helm upgrade -i primus-safe-cr`

4. **Step 4: Upgrade Data Plane (node-agent)**
   - When `install_node_agent` is not `n` (default: upgrade node-agent), upgrades `node-agent` with:
     - NIC overrides (`nccl_socket_ifname`, `nccl_ib_hca`)
     - Image pull secret and `image_registry` from `proxy_image_registry`

5. **Step 5: All completed**

### 📝 Key Differences from Installation

Unlike [install.sh](../../bootstrap/install.sh), the upgrade script:

- Does not prompt for configuration parameters
- Reuses existing configurations from the `.env` file
- Does not create secrets, install grafana-operator, or install primus-pgo
- Only upgrades `primus-safe`, `primus-safe-cr`, and optionally `node-agent`
- Applies CRD/RBAC/Webhook updates for `primus-safe` when already installed
- Defaults `install_node_agent` to `y` (upgrade node-agent) when not set in `.env`

### ⚠️ Important Notes

- The upgrade script only works if install.sh was previously executed and `.env` exists
- Ensure the environment configuration and code directory have not changed
- Custom images must be built and pushed before running the upgrade
- Add `proxy_image_registry` to `.env` to use a custom image registry
- If `ingress=higress`, `sub_domain` from `.env` is applied
- Data-plane observability (OpenSearch, Grafana) is now managed via Robust AddonTemplate, not direct primus-lens integration
- Backup your configuration before performing upgrades
- **Deployments with `auth.mode: external`:** set `auth_mode=external` and keep the other `auth_*` keys in `.env` (see above). Omitting `auth_mode` renders `builtin`. While the release stays external, the platform roles (`system-admin`, `system-admin-readonly`) come only from `auth.group_mappings`; the ones stored on user objects no longer count, so a local break-glass administrator (a `builtin` entry kept last in `auth.authenticators`) becomes an ordinary user. Grant every administrator the role through a group mapping before upgrading, and have them sign in through the identity provider (a local account no longer opens the administrator pages; `POST /auth/verify` runs the same authenticator chain, so services behind it see those administrators); see [External Authentication](../apis/external-authentication.md#upgrading-from-a-release-that-kept-local-administrators)
- Test upgrades in a non-production environment first

### 🔄 Upgrade Command

To execute the upgrade:

```bash
cd bootstrap
./upgrade.sh
```
