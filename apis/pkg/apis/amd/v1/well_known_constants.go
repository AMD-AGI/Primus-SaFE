/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

const (
	PrimusSafePrefix = "primus-safe."

	// general
	DisplayNameLabel = PrimusSafePrefix + "display.name"
	// Chip product name, mainly referring to the GPU chip here. e.g. AMD MI300X
	GpuProductNameAnnotation = PrimusSafePrefix + "gpu.product.name"
	// Corresponding resource names in Kubernetes ResourceList, such as amd.com/gpu or nvidia.com/gpu
	GpuResourceNameAnnotation = PrimusSafePrefix + "gpu.resource.name"
	// the label for Control-plane node
	KubernetesControlPlane = "node-role.kubernetes.io/control-plane"
	// total number of failures (used for internal retries)
	FailedCountAnnotation = PrimusSafePrefix + "failed.count"

	// node
	NodePrefix    = PrimusSafePrefix + "node."
	NodeFinalizer = NodePrefix + "finalizer"
	// The expected GPU count for the node, it should be annotated as a label
	NodeGpuCountLabel = NodePrefix + "gpu.count"
	// The node's last startup time
	NodeStartupTimeLabel = NodePrefix + "startup.time"
	NodeLabelAction      = NodePrefix + "label.action"
	NodeAnnotationAction = NodePrefix + "annotation.action"
	NodeIdLabel          = NodePrefix + "id"

	NodeActionAdd    = "add"
	NodeActionRemove = "remove"

	// Cluster lables
	ClusterPrefix                 = PrimusSafePrefix + "cluster."
	ClusterFinalizer              = ClusterPrefix + "finalizer"
	ClusterManagePrefix           = ClusterPrefix + "manage."
	ClusterManageActionLabel      = ClusterManagePrefix + "action"
	ClusterManageClusterLabel     = ClusterManagePrefix + "cluster"
	ClusterManageNodeLabel        = ClusterManagePrefix + "node"
	ClusterManageNodeClusterLabel = ClusterManagePrefix + "node.cluster"
	ClusterManageScaleDownLabel   = ClusterManagePrefix + "scale.down"
	ClusterIdLabel                = ClusterPrefix + "id"

	// storage
	StoragePrefix              = PrimusSafePrefix + "storage."
	StorageFinalizer           = StoragePrefix + "finalizer"
	StorageDefaultClusterLabel = StoragePrefix + "default.cluster"
	StorageTypeLabel           = StoragePrefix + "type"
	StorageClusterNameLabel    = StoragePrefix + "cluster.name"

	// nodeflavor
	NodeFlavorPrefix  = PrimusSafePrefix + "nodeflavor."
	NodeFlavorIdLabel = NodeFlavorPrefix + "id"

	// workspace
	WorkspacePrefix      = PrimusSafePrefix + "workspace."
	WorkspaceFinalizer   = WorkspacePrefix + "finalizer"
	WorkspaceIdLabel     = WorkspacePrefix + "id"
	WorkspaceNodesAction = WorkspacePrefix + "nodes.action"

	// fault
	FaultPrefix    = PrimusSafePrefix + "fault."
	FaultFinalizer = FaultPrefix + "finalizer"

	// workload
	WorkloadPrefix                   = PrimusSafePrefix + "workload."
	WorkloadFinalizer                = WorkloadPrefix + "finalizer"
	WorkloadDispatchedAnnotation     = WorkloadPrefix + "dispatched"
	WorkloadScheduledAnnotation      = WorkloadPrefix + "scheduled"
	WorkloadMainContainer            = WorkloadPrefix + "main.container"
	EnableHostNetworkAnnotation      = WorkloadPrefix + "enable.host.network"
	WorkloadForcedFailoverAnnotation = WorkloadPrefix + "forced.failover"
)

const (
	Pending  Phase = "Pending"
	Creating Phase = "Creating"
	Ready    Phase = "Ready"
	Unknown  Phase = "Unknown"
	Deleted  Phase = "Deleted"
)
