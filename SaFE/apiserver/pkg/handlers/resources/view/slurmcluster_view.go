/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

// NodePool describes one group of Slurm worker nodes. Each pool becomes a Slinky
// `slurm` chart NodeSet (slurmd StatefulSet) plus a matching Slurm partition of
// the same name.
type NodePool struct {
	// Name is the pool/partition name (Slurm partition + NodeSet key).
	Name string `json:"name"`
	// Nodes is the number of slurmd replicas in the pool.
	Nodes int `json:"nodes"`
	// GPU is the per-node GPU count (amd.com/gpu). 0 means no GPU request.
	GPU int `json:"gpu,omitempty"`
	// CPU is the per-node CPU limit (e.g. "8" or "8000m"). Empty = unset.
	CPU string `json:"cpu,omitempty"`
	// Memory is the per-node memory limit (e.g. "32Gi"). Empty = unset.
	Memory string `json:"memory,omitempty"`
}

// CreateSlurmClusterRequest is the request body for creating a Slurm cluster.
// A Slurm cluster is a per-workspace Helm release of the Slinky `slurm` chart,
// deployed into the workspace's namespace via the Addon mechanism.
type CreateSlurmClusterRequest struct {
	// WorkspaceId selects the workspace (and namespace) the cluster is deployed into.
	WorkspaceId string `json:"workspaceId"`
	// Name is the Slurm cluster name (Helm release is "slurm-<name>").
	Name string `json:"name"`
	// AccountingEnabled toggles the slurmdbd accounting subsystem.
	AccountingEnabled bool `json:"accountingEnabled,omitempty"`
	// Pools defines >=1 node pools (NodeSet + partition) deployed up front.
	Pools []NodePool `json:"pools"`
	// ImageTag optionally overrides the shared Slurm component image tag
	// (slurmctld/slurmrestd/slurmd/login/slurmdbd). Advanced; empty = chart default.
	ImageTag string `json:"imageTag,omitempty"`
	// Description is an optional human-friendly description.
	Description string `json:"description,omitempty"`
}

// PatchSlurmClusterRequest updates an existing Slurm cluster (helm upgrade).
type PatchSlurmClusterRequest struct {
	Pools             []NodePool `json:"pools,omitempty"`
	AccountingEnabled *bool      `json:"accountingEnabled,omitempty"`
	ImageTag          *string    `json:"imageTag,omitempty"`
	Description       *string    `json:"description,omitempty"`
}

// SlurmPod describes a single live pod belonging to a Slurm cluster's helm
// release, surfaced on the detail view.
type SlurmPod struct {
	// Name is the Kubernetes pod name.
	Name string `json:"name"`
	// Role is a coarse component role (controller/login/restapi/accounting/worker).
	Role string `json:"role"`
	// Node is the Kubernetes node the pod is scheduled on ("" if unscheduled).
	Node string `json:"node,omitempty"`
	// Phase is the pod phase (Pending/Running/Succeeded/Failed).
	Phase string `json:"phase"`
	// PodIP is the pod's cluster IP ("" if not yet assigned).
	PodIP string `json:"podIP,omitempty"`
	// HostIP is the node IP hosting the pod.
	HostIP string `json:"hostIP,omitempty"`
}

// SlurmClusterResponseItem is the response shape for a single Slurm cluster.
type SlurmClusterResponseItem struct {
	Name              string     `json:"name"`
	Workspace         string     `json:"workspace"`
	Namespace         string     `json:"namespace"`
	Cluster           string     `json:"cluster"`
	Phase             string     `json:"phase"`
	AccountingEnabled bool       `json:"accountingEnabled"`
	Pools             []NodePool `json:"pools,omitempty"`
	Partitions        []string   `json:"partitions,omitempty"`
	NodesReady        int        `json:"nodesReady"`
	NodesDesired      int        `json:"nodesDesired"`
	// Stopped reports whether the cluster is currently stopped (scaled to zero).
	Stopped bool `json:"stopped"`
	// ImageTag is the overridden component image tag, if any.
	ImageTag string `json:"imageTag,omitempty"`
	// Description is the optional human-friendly description.
	Description string `json:"description,omitempty"`
	// Pods is the live pod list, populated only on the detail (get) response.
	Pods         []SlurmPod `json:"pods,omitempty"`
	CreationTime string     `json:"creationTime"`
}

// ListSlurmClusterResponse is the response for listing Slurm clusters.
type ListSlurmClusterResponse struct {
	Items      []SlurmClusterResponseItem `json:"items"`
	TotalCount int                        `json:"totalCount"`
}

// SlurmLoginResponse describes how to SSH into a Slurm cluster's login node. The
// SSH command routes through the apiserver's SSH gateway (identical mechanism to
// workload SSH); the encoded username selects the login pod/container/namespace.
type SlurmLoginResponse struct {
	// Enabled reports whether SSH is enabled on this deployment at all.
	Enabled bool `json:"enabled"`
	// Ready reports whether a login pod is currently Running and reachable.
	Ready bool `json:"ready"`
	// SSHCommand is the ready-to-copy `ssh ...` command (empty when not ready).
	SSHCommand string `json:"sshCommand,omitempty"`
	// PodName is the login pod the command targets.
	PodName string `json:"podName,omitempty"`
	// Container is the login pod container the session execs into.
	Container string `json:"container,omitempty"`
	// Message is a human-friendly explanation when Ready is false.
	Message string `json:"message,omitempty"`
}
