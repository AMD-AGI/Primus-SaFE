## 🔄 Upgrade Script Explanation

The [upgrade.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/upgrade.sh) script provides a mechanism to upgrade the Primus-SaFE system after the initial installation has been completed using [install.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/install.sh).

### 📋 Prerequisites

Before running the upgrade script, ensure that:

1. [install.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/install.sh) has been executed successfully and generated a `.env` file
2. The environment configuration and code directory remain unchanged (the script reads `.env`)
3. Custom images have been built and pushed to your registry (if needed)
4. If upgrading images, chart values have been updated with new image versions/registries

### 🛠️ Preparing for Upgrade

#### Building and Pushing Custom Images

Users need to build and push their custom images to a container registry before upgrading:

```bash
docker buildx build . -t your-image-registry/primussafe/resource-manager:version -f ./SaFE/resource-manager/installer/Dockerfile --push
```


Replace `your-image-registry` with your actual container registry address and the appropriate version tag.

#### Updating Chart Values

Before running the upgrade, if you need to change images, manually update the image versions/registries in the corresponding chart `values.yaml`:

1. Update image tags to the new versions
2. Modify registry addresses to point to your custom registry
3. Ensure all component versions are consistent

### 🚀 Upgrade Process

The script performs the following steps:

1. **Load Configuration**: Reads parameters from `.env` (ethernet/rdma NICs, cluster scale, storage class, lens/s3/sso flags, ingress, sub-domain and Higress node port, whether to upgrade node‑agent).
2. **Derive Resources by Cluster Scale**:
   - small: replicas 1, cpu 2000m, memory 4Gi
   - medium: replicas 2, cpu 8000m, memory 8Gi
   - large: replicas 2, cpu 32000m, memory 16Gi
3. **Upgrade Admin Plane**:
   - Generates a temporary override from the chart values and applies:
     - NCCL network configs (`nccl_socket_ifname`, `nccl_ib_hca`)
     - `replicas`, `cpu`, `memory`, `storage_class`
     - Image pull secret name (`primus-safe-image`)
     - Ingress selection; when `higress`, sets `sub_domain`
     - `opensearch.enable` and `grafana.enable` based on `lens_enable`
       - If enabled, injects Grafana password from `primus-lens` secret automatically
     - `s3.enable` and `s3.secret` when S3 is enabled
     - `sso.enable` and `sso.secret` when SSO is enabled
   - If `primus-safe` is already installed:
     - Replaces CRDs
     - Renders and replaces RBAC role and webhooks manifests
   - Runs `helm upgrade -i primus-safe` and `helm upgrade -i primus-safe-cr`
4. **Optional: Upgrade Data Plane**:
   - When configured, upgrades `node-agent` with NIC overrides and the image pull secret
5. **Preserve Settings**: All inputs come from `.env`; custom settings persist

### 📝 Key Differences from Installation

Unlike [install.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/install.sh), the upgrade script:
- Does not prompt for configuration parameters
- Reuses existing configurations from the `.env` file
- Focuses on updating components rather than initial setup (does not create secrets)
- Preserves existing data and configurations
- Only upgrades components that were previously installed
 - Applies CRD/RBAC/Webhook updates for `primus-safe` when already installed

### ⚠️ Important Notes

- The upgrade script only works if install.sh was previously executed and `.env` exists
- Ensure the environment configuration and code directory have not changed
- Custom images must be built and pushed before running the upgrade
- Manual updates to chart `values.yaml` are required for image version changes
- If `ingress=higress`, both `sub_domain` from `.env` are applied
- If `lens_enable=true`, Grafana password is synced from the `primus-lens` secret automatically
- Backup your configuration before performing upgrades
- Test upgrades in a non-production environment first

### 🔄 Upgrade Command

To execute the upgrade:

```bash
cd bootstrap
./upgrade.sh
```
