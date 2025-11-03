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
          - nginx (NodePort): `http://<node-ip>:30183` (or `http://<node-ip>:30183/<your-callback-path>` if required by IdP)
          - higress (domain): `https://<cluster>.safe-primus.ai` (or `https://<cluster>.safe-primus.ai/<your-callback-path>`)
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
    - If `higress` is selected, you must enter a cluster name to be used as the subdomain
    - If `nginx` is selected, after installation you can access the web service via any Kubernetes node HostIP on port 30183 (NodePort), e.g., http://10.0.0.31:30183
    - If `higress` is selected, after installation you can open the web service at: http://<cluster>.safe-primus.ai
    - You must configure external access for the chosen address (e.g., a public DNS/domain)

### 🚀 Installation Process

The script performs the following steps after configuration:

1. Collects user input configuration parameters
2. Creates image pull secret for authentication
3. Installs grafana-operator monitoring component
4. Deploys Primus-SaFE admin plane components
5. Deploys Primus-SaFE data plane components (`node-agent`)
6. Saves configuration parameters to `.env` file for future upgrades

### 🔄 Install Command

To execute the install:

```bash
cd bootstrap
./install.sh
```
