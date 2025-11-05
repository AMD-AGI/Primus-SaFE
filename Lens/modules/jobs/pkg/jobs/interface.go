package jobs

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/cluster_overview"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/device_info"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_aggregation"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_allocation"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_consumers"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_history_cache_1h"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_history_cache_24h"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_history_cache_6h"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_pod"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_realtime_cache"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/gpu_workload"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/node_info"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs/storage_scan"
)

type Job interface {
	Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error
	Schedule() string
}

var jobs = []Job{}

func InitJobs() {
	jobs = []Job{
		&gpu_allocation.GpuAllocationJob{},
		&gpu_consumers.GpuConsumersJob{},
		&node_info.NodeInfoJob{},
		&device_info.DeviceInfoJob{},
		&gpu_workload.GpuWorkloadJob{},
		&gpu_pod.GpuPodJob{},
		&storage_scan.StorageScanJob{},
		&cluster_overview.ClusterOverviewJob{},
		// GPU cache jobs - split into separate jobs for better performance
		&gpu_realtime_cache.GpuRealtimeCacheJob{},      // Every 30s - realtime metrics
		&gpu_history_cache_1h.GpuHistoryCache1hJob{},   // Every 1m - 1 hour history
		&gpu_history_cache_6h.GpuHistoryCache6hJob{},   // Every 5m - 6 hour history
		&gpu_history_cache_24h.GpuHistoryCache24hJob{}, // Every 10m - 24 hour history
		gpu_aggregation.NewGpuAggregationJob(),
	}
}
