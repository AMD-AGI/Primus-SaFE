# Cluster API Documentation

## Overview
The Cluster API provides comprehensive management capabilities for computing clusters. A cluster serves as the top-level resource container in the system, used to organize and manage node resources, network configurations, and the runtime environment for workloads. Each cluster is an independent Kubernetes environment that provides physical resource isolation and supports multi-tenancy.

### Core Concepts

A cluster is the root-level container for all computing resources, with the following key characteristics:

* Resource Isolation: Provides physical-level resource isolation to ensure clusters operate independently without interference.
* Node Management: Manages control plane nodes and worker nodes.
* Network Configuration: Defines Pod subnets, service addresses, and network plugin settings.
* Kubernetes Environment: Encapsulates a complete Kubernetes runtime environment, including version and API server configuration.

## API Endpoints

### Create Cluster

#### Request
```http
POST /api/v1/clusters
Content-Type: application/json
```


**Request Body (CreateClusterRequest):**
```json
{
  "name": "string", 	         // The cluster name specified by the user
  "description": "string",       // The cluster description
  "sshSecretId": "string",       // The SSH secret id specified by the user, which must exist, used for node SSH login.
  "labels": {                    // The labels for cluster
    "key": "string"
  },
  "isProtected": true,        // Whether the cluster is under protection. When set to true, direct deletion is not allowed unless the label is removed
  "nodes": ["string"],           // The nodes of control plane
  "kubeSprayImage": "string",    // KubeSpray image name used for installation, such as kubespray:20200530
  "kubePodsSubnet": "string",    // Pod subnet configuration, such as 172.16.0.0/12
  "kubeServiceAddress": "string",// Service Address configuration, such as 192.168.0.0/16
  "kubeNetworkPlugin": "string", // Network plugin, default is flannel
  "kubernetesVersion": "string", // Kubernetes version, such as 1.32.5
  "kubeApiServerArgs": {         // Some settings for Kubernetes
    "max-mutating-requests-inflight": "5000",
    "max-requests-inflight": "10000"
  }
}
```


#### Response (CreateClusterResponse)
```json
{
  "clusterId": "string"  // The cluster id
}
```


### List Clusters

#### Request
```http
GET /api/v1/clusters
```


#### Response (ListClusterResponse)
```json
{
  "totalCount": 0,              // The total number of clusters, not limited by pagination
  "items": [
    {
      "clusterId": "string",    // The cluster id
      "userId": "string",       // User id who created the cluster.
      "phase": "string",        // The cluster status
      "isProtected": true,  	// Whether the cluster is under protection
      "creationTime": "string"  // The cluster creation time
    }
  ]
}
```


### Get Cluster Details

#### Request
```http
GET /api/v1/clusters/{clusterId}
```


#### Response (GetClusterResponse)
```json
{
  "clusterId": "string",           // The cluster id
  "userId": "string",              // User who created the cluster.
  "phase": "string",	           // The cluster status, such as Ready,Creating,Failed,Deleting
  "isProtected": true,          // Whether the cluster is under protection
  "creationTime": "string", 	   // The cluster creation time, such as  "2025-07-08T10:31:46"
  "description": "string",         // The cluster description
  "endpoint": "string",            // The endpoint of cluster control plane. such as "10.0.0.1:443"
  "sshSecretId": "string",         // The secret id for node ssh specified by the user
  "imageSecretId": "string",       // The secret id for pulling image specified by the user
  "nodes": ["string"],             // The nodes of control plane
  "kubeSprayImage": "string",      // KubeSpray image name used for installation. such as "docker.io/your-group/kubespray:20200530"
  "kubePodsSubnet": "string",      // Subnet configuration, such as "10.0.0.0/16"
  "kubeServiceAddress": "string",  // Service Address configuration, such as "10.254.0.0/16"
  "kubeNetworkPlugin": "string",   // Network plugin, default is flannel
  "kubernetesVersion": "string",   // Kubernetes version, such as "1.32.5"
  "kubeApiServerArgs": {	       // Some settings for Kubernetes
    "max-mutating-requests-inflight": "5000",
    "max-requests-inflight": "10000"
  }
}
```


### Update Cluster

#### Request
```http
PATCH /api/v1/clusters/{clusterId}
Content-Type: application/json
```


**Request Body (PatchClusterRequest):**
```json
{
  "isProtected": true    // Whether the cluster is under protection
}
```


#### Response
Returns 200 OK status code with no response body on success.

### Delete Cluster

#### Request
```http
DELETE /api/v1/clusters/{clusterId}
```


#### Response
Returns 200 OK status code with no response body on success.

### Manage Cluster Nodes

#### Request
```http
POST /api/v1/clusters/{clusterId}/nodes
Content-Type: application/json
```


**Request Body (ProcessNodesRequest):**
```json
{
  "nodeIds": ["string"],   // List of node ids to operate on.
  "action": "string" 	   // The action taken on the node of cluster, such as add or remove
}
```


#### Response (ProcessNodesResponse)
```json
{
  "totalCount": 0,        // Total number of nodes to be processed
  "successCount": 0       // Number of nodes processed successfully
}
```


### Get Cluster Logs

#### Request
```http
GET /api/v1/clusters/{clusterId}/logs
```


#### Response (GetClusterPodLogResponse)
```json
{
  "clusterId": "string",  // The cluster id
  "podId": "string",      // Pod id used to create the cluster.
  "logs": ["string"]      // An array of log lines, returned in the same order as they appear in the original logs
}
```
