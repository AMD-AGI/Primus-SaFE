# SaFE-Bootstrap

SaFE-Bootstrap is an automation toolkit for deploying, configuring, and benchmarking a Kubernetes-based cloud-native environment. It provides scripts for cluster bootstrapping, Ceph storage integration, Harbor registry setup, and performance benchmarking.

## Project Structure

- **bootstrap.sh**  
  Main script to bootstrap a Kubernetes cluster using Kubespray, install essential Helm charts (GPU operators, Prometheus, Cert-Manager, Higress, NVIDIA Network Operator, and Primus-SaFE), and configure local access.

- **higress/**  
  Scripts and configuration for deploying Higress cloud-native gateway with Kubernetes Gateway API support.
  - `higress.sh`: Automates Higress gateway deployment, including Helm repository setup, gateway installation, Gateway API CRDs deployment, and configuration verification.
  - `gateway.yaml`: Kubernetes Gateway API resources (GatewayClass and Gateway) for SSH traffic routing on port 2222.
  - `values.yaml`: Helm chart values for Higress core configuration, including HTTP/HTTPS/SSH port settings and host network mode.

- **ceph/**  
  Scripts and manifests for deploying a Rook-Ceph storage cluster and integrating Ceph CSI drivers.
  - `ceph.sh`: Automates Ceph installation and CSI configuration.
  - `cephcluster.yaml`: Ceph cluster definition for Rook.
  - `storageclass.yaml`: StorageClass and Secret for Ceph RBD.
  - `csi.template`: Template for CSI config.
  - `ceph-rgw.yaml`: S3 object store user setup.

- **harbor/**  
  Scripts and configs for deploying a Harbor container registry with custom CA and integration with Higress.
  - `harbor.sh`: Automates Harbor deployment, CA setup, and DNS integration.
  - `values.yaml`: Harbor Helm values.
  - `harbor_ca_task.yaml`, `hosts_template`: Ansible and DNS templates.

- **benchmark/**  
  Tools and scripts for cluster performance benchmarking using kube-burner.
  - `benchmark.sh`: Downloads kube-burner, runs API-intensive benchmarks, and parses results.
  - `api-intensive.yml`: Benchmark scenario definition.
  - `parse_pod_latency.sh`, `parse_node_latency.sh`, `parse_pod_quantiles.sh`, `parse_node_quantiles.sh`: Scripts to process latency and quantile results.
  - `etcd_perf.sh`: Script for etcd performance check.
  - `hollow-node.yaml`: StatefulSet for kubemark hollow nodes.
  - `kubemark.Dockerfile`: Dockerfile for building kubemark image.
  - `templates/`: YAML/JSON templates for benchmarking workloads.

- **hosts.yaml**  
  Ansible inventory file for Kubespray, listing all nodes and their roles.

## Editing hosts.yaml

Before bootstrapping the cluster, you must edit the `hosts.yaml` file to define your cluster nodes and their roles. This file follows the Ansible inventory format and is used by Kubespray to determine which machines will serve as control plane nodes, etcd nodes, and worker nodes.

Example structure:

```
[all]
  node1 ansible_host=192.168.1.10 ip=192.168.1.10
  node2 ansible_host=192.168.1.11 ip=192.168.1.11
  node3 ansible_host=192.168.1.12 ip=192.168.1.12

[kube_control_plane]
  node1
  node2

[etcd]
  node1
  node2
  node3

[kube_node]
  node1
  node2
  node3

[k8s-cluster:children]
  kube_node
  kube_control_plane
```

- Replace `node1`, `node2`, etc. with your actual hostnames or aliases.
- Set the correct `ansible_host` and `ip` for each node.
- Assign nodes to the appropriate groups based on their intended roles.

## Quick Start

### 1. Bootstrap the Kubernetes Cluster
```bash
bash bootstrap.sh
```
- Installs dependencies, sets up Kubespray, deploys the cluster, and installs core Helm charts.

### 2. Deploy Higress Gateway
```bash
cd higress
bash higress.sh
```
- Installs Higress cloud-native gateway, deploys Gateway API CRDs, and configures SSH gateway on port 2222.

### 3. Deploy Ceph Storage
```bash
cd ceph
bash ceph.sh
```
- Installs Rook-Ceph, configures CSI, and sets up storage classes and S3 object store.

### 4. Deploy Harbor Registry
```bash
cd harbor
bash harbor.sh <HARBOR_ADMIN_PASSWORD> [HARBOR_DOMAIN] [SSH_KEY]
```
- Installs Harbor, sets up CA, integrates with Higress, and configures DNS.

### 5. Run Benchmarks
```bash
cd benchmark
bash benchmark.sh
```
- Downloads kube-burner, runs API-intensive tests, and parses results.

## Requirements

- Ubuntu-based system with `bash`, `python3`, `pip`, `virtualenv`, `ansible`, `helm`, and `kubectl`.
- Sudo privileges for system setup.
- Internet access for downloading dependencies and Helm charts.

## Notes

- The scripts assume a fresh environment and may overwrite existing configurations.
- Review and adjust YAML and script parameters as needed for your infrastructure.
- For production, consider pinning Helm chart versions and securing credentials.