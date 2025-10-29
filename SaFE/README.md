# Primus-SaFE

Primus-SaFE is a cloud-native Stability-as-First Enhancement framework for large-scale foundation-model training and inference. Built on Kubernetes, container networking, elastic storage, and service meshes, it delivers elastic scheduling, fast failure recovery, and automatic health checks—minimising downtime from node loss, network jitter, or checkpoint overhead and boosting overall goodput (the share of compute that truly advances the model). SaFE weaves Megatron, ROCm-optimised components, and proprietary fault-tolerance logic directly into the training stack, transparently handling parameter re-sharding, micro-batch replay, and asynchronous gradient sync to keep goodput above 90 % even on thousand-GPU, multi-day runs. It supports pre-training, post-training (SFT/LoRA), and reinforcement-learning workflows, offers multi-tenant isolation and observability, and lets model teams focus on algorithmic progress while SaFE owns stability and efficiency.

## 📦 Installation Instructions

Use the following script to install Primus-SaFE with one command:

```bash
cd bootstrap
./install.sh
```


> ⚠️ **Note**: The installation script must be executed on a Kubernetes master node.

During installation, you will be prompted to input various parameters including network configuration, storage class, and feature module enablement. The installer will automatically deploy all required components.

If you need to set up a Kubernetes cluster first, you can use our team's another open-source component:
[Primus-SaFE-Bootstrap](https://github.com/AMD-AGI/Primus-SaFE-Bootstrap)

### Upgrading Primus-SaFE

If you have previously executed `install.sh` and want to upgrade your Primus-SaFE deployment, you can use the `upgrade.sh` script:

```bash
cd bootstrap
./upgrade.sh
```


The `upgrade.sh` script will:
- Load parameters from the existing `.env` file created during installation
- Upgrade all Helm charts with the preserved configuration
- Maintain your existing settings and customization

> ⚠️ **Note**: The upgrade script only applies if `install.sh` has been previously executed and the environment configuration and code directory have not changed.

### System Requirements

- Helm 3+
- kubectl
- Kubernetes cluster (1.21+)

## 🏗️ System Architecture

Primus-SaFE consists of five core modules:

### 1. apiserver

Provides unified API interface services for external interactions:

- Resource management APIs: CRUD operations for nodes, clusters, and workspaces
- Workload management APIs: Creation, querying, updating, and deletion of workloads
- User and permission management: User authentication, authorization, and RBAC permission control
- SSH login support: Direct SSH access to compute nodes

### 2. job-manager

Manages the full lifecycle of Workloads:

- Multi-workload type support: PyTorchJob, Job, Deployment, and other common types
- Intelligent scheduling: Task queuing, resource allocation, and priority preemption
- Fault recovery: Automatic retry and migration (failover) after workload failures
- Status monitoring: Real-time tracking of workload execution status
- Log Index: Query and download workload logs

### 3. resource-manager

Centralized resource management center:

- Cluster management: Creation, configuration, and maintenance of AI clusters
- Node management: Registration and monitoring of compute nodes and their plugins
- Workspace management: Isolated working environments for users
- Storage management: Allocation and recycling of persistent storage volumes
- Operations support: Log collection, node health checks, and other OPS functions

### 4. webhooks

Implements Kubernetes admission controller functionality:

- Request validation: Legitimacy checks for resource creation/modification requests
- Object modification: Automatic adjustment of resource configurations based on business rules

### 5. node-agent

Node-level agent service:

- Status probing: Continuous monitoring of node operational status and hardware health
- Fault detection: Timely identification of node failures and configuration anomalies
- Automatic reporting: Reporting node conditions to the central management system
- Self-healing capabilities: Attempting to fix common node issues

## 🛠️ Key Features

- **Multi-tenancy support**: Resource isolation mechanism based on workspaces
- **Elastic scaling**: Dynamic resource allocation based on load
- **High availability design**: Fault tolerance and recovery capabilities for critical components
- **Plugin extensibility**: Support for custom plugins to extend node functionality
- **Visual interface**: Integrated Grafana for rich monitoring views. Additionally, a web-based console interface is available to help users better use the system

## 📄 License

This project is licensed under the Apache License 2.0. See the [LICENSE](https://github.com/AMD-AGI/Primus-SaFE/blob/main/LICENSE) file for details.
Third-party software and their licenses are listed in the LICENSE file. Users must comply with the respective licenses of these third-party projects when using or distributing Primus-SaFE.