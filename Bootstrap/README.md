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

- **hosts.ini**  
  Ansible inventory file that `bootstrap.sh` passes to Kubespray, listing all nodes and their roles.

## Editing hosts.ini

Before bootstrapping the cluster, you must edit the `hosts.ini` file to define your cluster nodes and their roles. This file follows the Ansible inventory format and is used by Kubespray to determine which machines will serve as control plane nodes, etcd nodes, and worker nodes.

Example structure:

```ini
[all]
node-01 ansible_host=10.0.0.11 ip=10.0.0.11
node-02 ansible_host=10.0.0.12 ip=10.0.0.12
node-03 ansible_host=10.0.0.13 ip=10.0.0.13

[kube_control_plane]
node-01
node-02
node-03

[etcd]
node-01
node-02
node-03

[kube_node]
node-01
node-02
node-03

[k8s_cluster:children]
kube_control_plane
kube_node
```

- Replace `node-01`, `node-02`, etc. with your actual hostnames. Kubespray renames each host to its inventory name.
- Set the correct `ansible_host` (the address Ansible connects to) and `ip` (the address Kubernetes binds to) for each node.
- Assign nodes to the appropriate groups based on their intended roles. Kubespray stops if the number of `etcd` nodes is even.
- Node IPs must not overlap the pod range (`KUBE_PODS_SUBNET`, default `172.16.0.0/12`) or the service range (`KUBE_SERVICE_ADDRESSES`, default `192.168.0.0/16`). If they do, change those variables at the top of `bootstrap.sh`.

### Logging in as a non-root user

By default, Ansible logs in to the nodes as the user that runs `bootstrap.sh`. To log in as a specific non-root user instead, add the login settings to `hosts.ini`:

```ini
[all:vars]
ansible_user=<user>
ansible_ssh_private_key_file=~/.ssh/id_ed25519
```

- The user needs passwordless sudo on every node, because `bootstrap.sh` does not prompt for a sudo password.
- Authorize the key for that user on every node, including the deploy host if it is one of the nodes.
- Do not set `ansible_become` in the inventory. `bootstrap.sh` already runs every playbook with `--become --become-user=root`, and inventory variables override Kubespray's per-task `become: false`, which it uses mainly for tasks that run on the deploy host.
- Put these settings in `hosts.ini`, not in an `ansible.cfg` in `Bootstrap/`. Ansible uses only the first config file it finds, and `./ansible.cfg` in the current directory comes before `~/.ansible.cfg`. `bootstrap.sh` runs Ansible from `Bootstrap/kubespray`, which has its own `ansible.cfg`, so a file in `Bootstrap/` is never read.

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