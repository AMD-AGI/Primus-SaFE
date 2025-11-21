package jobs

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/cluster_overview"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/device_info"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_aggregation"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_allocation"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_consumers"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_history_cache_1h"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_history_cache_24h"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_history_cache_6h"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_pod"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_realtime_cache"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/storage_scan"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/workload_statistic"
)

type Job interface {
	Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error)
	Schedule() string
}

var jobs = []Job{}

func InitJobs() {
	jobs = []Job{
		&gpu_allocation.GpuAllocationJob{},
		&gpu_consumers.GpuConsumersJob{},
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
		workload_statistic.NewWorkloadStatisticJob(), // Every 5m - workload GPU utilization statistics
	}
}
