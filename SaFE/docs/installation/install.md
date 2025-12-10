## 📦 Installation Script Explanation

The [install.sh](https://github.com/AMD-AGI/Primus-SaFE/blob/main/SaFE/bootstrap/install.sh) script provides a one-click installation for the Primus-SaFE system and all its dependent components. During the installation process, users need to provide a series of configuration parameters.

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
        - small: 1 replica, 2 CPU cores, 4Gi memory
        - medium: 2 replicas, 8 CPU cores, 8Gi memory
        - large: 2 replicas, 32 CPU cores, 16Gi memory

- **storage_class (Storage Class)**
    - Kubernetes StorageClass name for persistent storage
    - Default value: `local-path`
    - Requires the StorageClass to be pre-created by the administrator

#### Optional Feature Components

- **Primus-lens (Monitoring and Log Collection)**
    - Another open-source component from our team for system monitoring and log collection
    - Optional feature that can be enabled based on requirements

- **S3 Storage Support**
    - Used for log downloading and other S3 storage functions
    - Optional feature that can be enabled during installation
    - When enabled, users need to provide:
      - S3 endpoint: The S3 service endpoint URL
      - S3 bucket: The bucket name for storage
      - S3 access-key: The access key for authentication
      - S3 secret-key: The secret key for authentication
    - If any of these values are left empty, S3 support will be disabled automatically

- **Single Sign-On (SSO) with OIDC**
    - Enables logging in with an external Identity Provider (IdP) over OpenID Connect
    - Optional feature that can be enabled during installation
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
    - Authentication information for component image downloads
    - Requires users to provide image registry address, username, and password
    - Must be configured to ensure components can pull images properly

- **Ingress Gateway**
    - External service gateway
    - Supports `nginx` (default) and `higress` types
    - If `higress` is selected:
      - You must enter a cluster name (used as the subdomain)
      - You will also enter the Higress gateway NodePort (default: `32608`)
      - After installation, you can access the web service at: `http://<cluster>.primus-safe.amd.com`
    - If `nginx` is selected, after installation you can access the web service via any Kubernetes node HostIP on port `30183` (NodePort), e.g., `http://10.0.0.31:30183`
    - You must configure external access for the chosen address (e.g., a public DNS/domain)

#### Image Pull Secret

- Authentication information for component image downloads
- Requires users to provide image registry address, username, and password
- If you choose not to provide registry credentials, the installer still creates an empty image pull secret with the correct type so all components reference a consistent secret name

### 🚀 Installation Process

The script performs the following steps after configuration:

1. Collects user input configuration parameters
2. Creates required Kubernetes secrets:
   - Image pull secret (real or empty placeholder, same name used by all components)
   - Optional S3 secret (when S3 enabled)
   - Optional SSO secret (when SSO enabled)
3. Installs grafana-operator monitoring component
4. Deploys Primus‑SaFE admin plane components:
   - Installs `primus-pgo` (Postgres Operator)
   - Installs/Upgrades `primus-safe` (apiserver, webhooks, controllers)
   - Installs `primus-safe-cr` (custom resources)
5. Optionally deploys Primus‑SaFE data plane components (`node-agent`)
6. Saves configuration parameters to `.env` file for future upgrades

### 📄 Artifacts Created

- Kubernetes Secrets (namespace `primus-safe`):
  - `primus-safe-image`: image pull secret (type `kubernetes.io/dockerconfigjson`)
  - `primus-safe-s3`: only when S3 is enabled
  - `primus-safe-sso`: only when SSO is enabled, keys: `id`, `secret`, `endpoint`, `redirect_uri`
- `.env` file (in `bootstrap/`) with the following keys for future upgrades:
  - `ethernet_nic`, `rdma_nic`, `cluster_scale`, `storage_class`
  - `lens_enable`, `s3_enable`, `sso_enable`
  - `ingress`, `sub_domain`
  - `install_node_agent`

### 🔄 Install Command

To execute the install:

```bash
cd bootstrap
./install.sh
```
