/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

const (
	PrimusSafePrefix = "primus-safe."

	// node
	NodePrefix = PrimusSafePrefix + "node."
	// The expected GPU count for the node, it should be annotated as a label
	NodeGpuCountLabel = NodePrefix + "gpu.count"
	// The node's last startup time
	NodeStartupTimeLabel = NodePrefix + "startup.time"

	// nodeflavor
	NodeFlavorPrefix = PrimusSafePrefix + "nodeflavor."
	NodeFlavorLabel  = NodeFlavorPrefix + "name"
	// Cluster lables
	ClusterPrefix                 = PrimusSafePrefix + "cluster."
	ClusterManagePrefix           = ClusterPrefix + "manage."
	ClusterManageActionLabel      = ClusterManagePrefix + "action"
	ClusterManageClusterLabel     = ClusterManagePrefix + "cluster"
	ClusterManageNodeLabel        = ClusterManagePrefix + "node"
	ClusterManageNodeClusterLabel = ClusterManagePrefix + "node.cluster"
	ClusterManageScaleDownLabel   = ClusterManagePrefix + "scale.down"
	ClusterServiceName            = ClusterManagePrefix + "service.name"
	// cluster
	ClusterNameLabel = ClusterPrefix + ".name"

	// storage
	StoragePrefix              = PrimusSafePrefix + "storage."
	StorageDefaultClusterLabel = StoragePrefix + "default.cluster"
	StorageTypeLabel           = StoragePrefix + "type"
	StorageClusterNameLabel    = StoragePrefix + "cluster.name"
)

const (
	Pending  Phase = "Pending"
	Creating Phase = "Creating"
	Ready    Phase = "Ready"
	Unknown  Phase = "Unknown"
	Deleted  Phase = "Deleted"
)
