/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

const (
	Pending  Phase = "Pending"
	Creating Phase = "Creating"
	Ready    Phase = "Ready"
	Unknown  Phase = "Unknown"
	Deleted  Phase = "Deleted"
)

const (
	PrimusSafePrefix = "primus-safe."

	// general
	DisplayNameLabel = PrimusSafePrefix + "display.name"
	// Chip product name, mainly referring to the GPU chip here. e.g. AMD MI300X
	GpuProductNameAnnotation = PrimusSafePrefix + "gpu.product.name"
	// Corresponding resource names in Kubernetes ResourceList, such as amd.com/gpu or nvidia.com/gpu
	GpuResourceNameAnnotation = PrimusSafePrefix + "gpu.resource.name"

	// node
	NodePrefix    = PrimusSafePrefix + "node."
	NodeFinalizer = NodePrefix + "finalizer"
	// The expected GPU count for the node, it should be annotated as a label
	NodeGpuCountLabel = NodePrefix + "gpu.count"
	// The node's last startup time
	NodeStartupTimeLabel  = NodePrefix + "startup.time"
	NodesLabelAction      = NodePrefix + "label.action"
	NodesAnnotationAction = NodePrefix + "annotation.action"
	NodeActionAdd         = "add"
	NodeActionRemove      = "remove"

	// Cluster lables
	ClusterPrefix                 = PrimusSafePrefix + "cluster."
	ClusterFinalizer              = ClusterPrefix + "finalizer"
	ClusterManagePrefix           = ClusterPrefix + "manage."
	ClusterManageActionLabel      = ClusterManagePrefix + "action"
	ClusterManageClusterLabel     = ClusterManagePrefix + "cluster"
	ClusterManageNodeLabel        = ClusterManagePrefix + "node"
	ClusterManageNodeClusterLabel = ClusterManagePrefix + "node.cluster"
	ClusterManageScaleDownLabel   = ClusterManagePrefix + "scale.down"
	ClusterServiceName            = ClusterManagePrefix + "service.name"
	ClusterNameLabel              = ClusterPrefix + "name"

	// storage
	StoragePrefix              = PrimusSafePrefix + "storage."
	StorageDefaultClusterLabel = StoragePrefix + "default.cluster"
	StorageTypeLabel           = StoragePrefix + "type"
	StorageClusterNameLabel    = StoragePrefix + "cluster.name"

	// nodeflavor
	NodeFlavorPrefix  = PrimusSafePrefix + "nodeflavor."
	NodeFlavorIdLabel = NodeFlavorPrefix + "id"

	// workspace
	WorkspacePrefix    = PrimusSafePrefix + "workspace."
	WorkspaceFinalizer = WorkspacePrefix + "finalizer"
	WorkspaceIdLabel   = WorkspacePrefix + "id"

	// fault
	FaultPrefix       = PrimusSafePrefix + "fault."
	FaultFinalizer    = FaultPrefix + "finalizer"
	FaultIDLabel      = FaultPrefix + "id"
	PrimusTaintPrefix = "primus.amd.com/"
)
