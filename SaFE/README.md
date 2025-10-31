# Primus-SaFE

Primus-SaFE(Stability and Fault Endurance) is a cloud-native framework for large-scale foundation model training and inference. Built on Kubernetes and related technologies, it provides elastic scheduling, fast failure recovery, and automatic health checks.

It minimizes downtime caused by node failures, network fluctuations, and checkpoint overhead, thereby improving effective compute utilization (goodput). It offers multi-tenant isolation and observability so model teams can focus on algorithmic progress while SaFE takes care of stability and efficiency.

In short: a highly available, high‑performance platform for large‑model training that lets users focus on modeling instead of underlying stability.

## 📦 Installation

Quick installation with one command:

```bash
cd bootstrap
./install.sh
```

Notes and full parameter explanations are available in the installation docs:

- Network, storage, optional components (S3, monitoring), image pull secrets, and ingress: see `docs/installation/install.md`
- The script saves your selections to `.env` for future upgrades

Requirements:

- Run on a Kubernetes control-plane node
- Kubernetes 1.21+, kubectl, Helm 3+

If you need to bootstrap a Kubernetes cluster first, see: [Primus-SaFE-Bootstrap](https://github.com/AMD-AGI/Primus-SaFE/tree/main/Bootstrap)

### 🔄 Upgrade

```bash
cd bootstrap
./upgrade.sh
```

Summary (see `docs/installation/upgrade.md` for details):

- Reuses `.env` from install; does not prompt for parameters
- Update image tags in `charts/primus-safe/values.yaml` before upgrading
- Build and push custom images as needed
- Upgrades admin plane (charts/CRDs/RBAC/webhooks) and node-agent, preserving settings

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

## 📚 API Documentation

All API references live under `docs/apis`. Start here:
- [Overview and index](./docs/apis/index.md)


## 🛠️ Key Features

- **Multi-tenancy support**: Resource isolation mechanism based on workspaces
- **Elastic scaling**: Dynamic resource allocation based on load
- **High availability design**: Fault tolerance and recovery capabilities for critical components
- **Plugin extensibility**: Support for custom plugins to extend node functionality
- **Visual interface**: Integrated Grafana for rich monitoring views. Additionally, a web-based console interface is available to help users better use the system

## 📄 License

This project is licensed under the Apache License 2.0. See the [LICENSE](https://github.com/AMD-AGI/Primus-SaFE/blob/main/LICENSE) file for details.
Third-party software and their licenses are listed in the LICENSE file. Users must comply with the respective licenses of these third-party projects when using or distributing Primus-SaFE.