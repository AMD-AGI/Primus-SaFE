## 📦 Installation Script Explanation

The [install.sh](../../bootstrap/install.sh) script provides a one-click installation for the Primus-SaFE system and all its dependent components. During the installation process, users need to provide a series of configuration parameters.

**Prerequisites:** `helm` and `kubectl` must be installed.

### 🛠️ Configuration Parameters Explanation

#### Network Configuration Parameters

- **ethernet_nic (Ethernet Interface)**
    - Ethernet interface name used for TCP communication
    - Default value: `eno0`
    - Examples: `eno0`, `eth0`, `enp3s0f0`, etc.

- **rdma_nic (RDMA Network Interface)**
    - List of network interfaces used for RDMA high-performance network communication
    - Default value: `rdma0,rdma1,rdma2,rdma3,rdma4,rdma5,rdma6,rdma7`
    - Supports multiple interfaces separated by commas

#### Cluster Resource Configuration

- **cluster_scale (Cluster Scale)**
    - Automatically adjusts resource allocation for management plane components based on cluster scale
    - Options: `small` (default), `medium`, `large`
    - Affects replica count, CPU, and memory resources for management components:
        - small: 1 replica, 2000m CPU, 4Gi memory
        - medium: 2 replicas, 8000m CPU, 8Gi memory
        - large: 2 replicas, 32000m CPU, 16Gi memory

- **storage_class (Storage Class)**
    - Kubernetes StorageClass name for persistent storage
    - Default value: `local-path`
    - Requires the StorageClass to be pre-created by the administrator

#### Optional Feature Components

- **Primus-lens (Monitoring and Log Collection)**
    - Another open-source component from our team for system monitoring and log collection
    - Optional feature that can be enabled based on requirements (y/n, default: n)

- **S3 Storage Support**
    - Used for log downloading and other S3 storage functions
    - Optional feature that can be enabled during installation (y/n, default: n)
    - When enabled, users need to provide:
      - S3 endpoint: The S3 service endpoint URL
      - S3 bucket: The bucket name for storage
      - S3 access-key: The access key for authentication
      - S3 secret-key: The secret key for authentication
    - If any of these values are left empty, S3 support will be disabled automatically

- **Single Sign-On (SSO) with OIDC**
    - Enables logging in with an external Identity Provider (IdP) over OpenID Connect
    - Optional feature that can be enabled during installation (y/n, default: n)
    - When enabled, users need to provide:
      - SSO endpoint: The OIDC issuer URL of your IdP (e.g., `https://accounts.example.com`)
      - SSO client id: The client/application ID registered in your IdP
      - SSO client secret: The client/application secret issued by your IdP
      - SSO redirect uri: The redirect URL registered for your client in the IdP
        - This should point to your Primus‑SaFE Web Console base URL
        - Examples based on ingress choice:
          - nginx (NodePort): `http://<node-ip>:30183`
          - higress (domain): `https://<cluster>.primus-safe.amd.com`
    - What the installer does:
      - Creates a Kubernetes secret `primus-safe-sso` in namespace `primus-safe` with keys: `id`, `secret`, `endpoint`, `redirect_uri`
      - Enables the `sso.enable` flag and wires the secret into the Primus‑SaFE Helm values

#### Image and Network Configuration

- **Image Pull Secret**
    - Authentication information for component image downloads (y/n, default: n)
    - If enabled, requires image registry address, username, and password
    - If you choose not to provide registry credentials, the installer creates an empty image pull secret with the correct type so all components reference a consistent secret name

- **Ingress Gateway**
    - External service gateway
    - Supports `nginx` (default) and `higress` types
    - If `higress` is selected:
      - You must enter a cluster name (lowercase with hyphen, used as the subdomain)
      - After installation, you can access the web service at: `https://<cluster>.primus-safe.amd.com`
    - If `nginx` is selected, after installation you can access the web service via any Kubernetes node HostIP on port `30183` (NodePort), e.g., `http://10.0.0.31:30183`
    - You must configure external access for the chosen address (e.g., a public DNS/domain)

#### Storage and Data Plane

- **csi_volume_handle**
    - CSI volume handle for persistent filesystem (PFS) storage in workspaces
    - Leave empty to disable PFS for workspace creation
    - When set, the installer creates a PV template ConfigMap for workspace provisioning

- **install_node_agent**
    - Whether to install the Primus‑SaFE data plane (node-agent) component (y/n, default: n)
    - When enabled, deploys the node-agent DaemonSet on the cluster

### 🚀 Installation Process

The script performs the following steps after configuration:

1. **Step 1:** Collects user input configuration parameters
2. **Step 2:** Creates required Kubernetes secrets:
   - Image pull secret (real or empty placeholder, same name used by all components)
   - Optional S3 secret (when S3 enabled)
   - Optional SSO secret (when SSO enabled)
3. **Step 3:** Installs grafana-operator monitoring component
4. **Step 4:** Deploys Primus‑SaFE admin plane components:
   - Installs `primus-pgo` (Postgres Operator)
   - Waits for Postgres Operator to be running
   - Installs/Upgrades `primus-safe` (apiserver, webhooks, controllers)
5. **Step 5:** Upgrades `primus-safe-cr` (custom resources)
6. **Step 6:** Optionally deploys Primus‑SaFE data plane components (`node-agent`) when `install_node_agent` is enabled
7. **Step 7:** Saves configuration parameters to `.env` file for future upgrades

### 📄 Artifacts Created

- Kubernetes Secrets (namespace `primus-safe`):
  - `primus-safe-image`: image pull secret (type `kubernetes.io/dockerconfigjson`)
  - `primus-safe-s3`: only when S3 is enabled
  - `primus-safe-sso`: only when SSO is enabled, keys: `id`, `secret`, `endpoint`, `redirect_uri`
- `.env` file (in `bootstrap/`) with the following keys for future upgrades:
  - `ethernet_nic`, `rdma_nic`, `cluster_scale`, `storage_class`
  - `lens_enable`, `s3_enable`, `sso_enable`
  - `ingress`, `sub_domain`
  - `install_node_agent`, `csi_volume_handle`

### 🔄 Install Command

To execute the install:

```bash
cd bootstrap
./install.sh
```
