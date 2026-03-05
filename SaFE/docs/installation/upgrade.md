## 🔄 Upgrade Script Explanation

The [upgrade.sh](../../bootstrap/upgrade.sh) script provides a mechanism to upgrade the Primus-SaFE system after the initial installation has been completed using [install.sh](../../bootstrap/install.sh).

**Prerequisites:** `helm` and `kubectl` must be installed.

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
| `tracing_enable` | Enable OpenTelemetry tracing: `true` or `false` |
| `tracing_mode` | Tracing mode: `all` or `error_only` |
| `tracing_sampling_ratio` | Sampling ratio (e.g. `1.0`) |
| `tracing_otlp_endpoint` | OTLP collector endpoint |
| `proxy_services` | YAML list for proxy services configuration |

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
     - `opensearch.enable` and `grafana.enable` based on `lens_enable`
       - If enabled, injects Grafana password from `primus-lens` secret automatically
     - `s3.enable` and `s3.secret` when S3 is enabled
     - `sso.enable` and `sso.secret` when SSO is enabled
     - `cd.require_approval` from `cd_require_approval`
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
- If `lens_enable=true`, Grafana password is synced from the `primus-lens` secret automatically
- Backup your configuration before performing upgrades
- Test upgrades in a non-production environment first

### 🔄 Upgrade Command

To execute the upgrade:

```bash
cd bootstrap
./upgrade.sh
```
