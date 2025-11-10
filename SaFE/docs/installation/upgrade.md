## 🔄 Upgrade Script Explanation

The [upgrade.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/upgrade.sh) script provides a mechanism to upgrade the Primus-SaFE system after the initial installation has been completed using [install.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/install.sh).

### 📋 Prerequisites

Before running the upgrade script, ensure that:

1. [install.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/install.sh) has been executed successfully
2. The environment configuration and code directory remain unchanged
3. Custom images have been built and pushed to your registry (if needed)
4. Chart values have been updated with new image versions

### 🛠️ Preparing for Upgrade

#### Building and Pushing Custom Images

Users need to build and push their custom images to a container registry before upgrading:

```bash
docker buildx build . -t your-image-registry/primussafe/resource-manager:version -f ./SaFE/resource-manager/installer/Dockerfile --push
```


Replace `your-image-registry` with your actual container registry address and the appropriate version tag.

#### Updating Chart Values

Before running the upgrade, manually update the image versions in [values.yaml](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/charts/primus-safe/values.yaml):

1. Update image tags to the new versions
2. Modify registry addresses to point to your custom registry
3. Ensure all component versions are consistent

### 🚀 Upgrade Process

The script performs the following steps:

1. **Load Configuration**: Reads parameters from the existing `.env` file created during installation
2. **Upgrade Admin Plane**:
    - Updates the main `primus-safe` Helm chart
    - Applies CRD (Custom Resource Definition) updates
    - Updates RBAC configurations and webhooks
3. **Upgrade Data Plane**:
    - Updates the `node-agent` component with new configurations
4. **Preserve Settings**: Maintains existing configurations and customizations

### 📝 Key Differences from Installation

Unlike [install.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/install.sh), the upgrade script:
- Does not prompt for configuration parameters
- Reuses existing configurations from the `.env` file
- Focuses on updating components rather than initial setup
- Preserves existing data and configurations
- Only upgrades components that were previously installed

### ⚠️ Important Notes

- The upgrade script only works if install.sh was previously executed
- Ensure the environment configuration and code directory have not changed
- Custom images must be built and pushed before running the upgrade
- Manual updates to [values.yaml](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/charts/primus-safe/values.yaml) are required for image version changes
- Backup your configuration before performing upgrades
- Test upgrades in a non-production environment first

### 🔄 Upgrade Command

To execute the upgrade:

```bash
cd bootstrap
./upgrade.sh
```
