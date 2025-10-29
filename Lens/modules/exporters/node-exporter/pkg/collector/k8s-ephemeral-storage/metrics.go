package k8s_ephemeral_storage

import (
	"github.com/AMD-AGI/primus-lens/core/pkg/metrics"
)

var (
	PodEphemeralStorageUsageBytes      = metrics.NewGaugeVec("pod_ephemeral_storage_usage_bytes", "Pod emphermeral storage usage in bytes", []string{"pod_namespace", "pod_name", "node"}, metrics.WithNamespace(""), metrics.WithoutSuffix())
	NodeEphemeralStorageUsageBytes     = metrics.NewGaugeVec("node_ephemeral_storage_usage_bytes", "Node emphermal storage usage in bytes", []string{"node"}, metrics.WithNamespace(""), metrics.WithoutSuffix())
	NodeEphemeralStorageCapacityBytes  = metrics.NewGaugeVec("node_ephemeral_storage_capacity_bytes", "Node emphermal storage capacity in bytes", []string{"node"}, metrics.WithNamespace(""), metrics.WithoutSuffix())
	NodeEphemeralStorageUsagePercent   = metrics.NewGaugeVec("node_ephemeral_storage_usage_percent", "Node emphermal storage usage in percent", []string{"node"}, metrics.WithNamespace(""), metrics.WithoutSuffix())
	NodeEphemeralStorageAvailableBytes = metrics.NewGaugeVec("node_ephemeral_storage_available_bytes", "Node emphermal storage available in bytes", []string{"node"}, metrics.WithNamespace(""), metrics.WithoutSuffix())
)
