package gpu_allocation

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	allocationRateGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gpu_allocation_rate",
		Help: "Allocation Rate Of GPU",
	})
)

func init() {
	prometheus.MustRegister(allocationRateGauge)
}

type GpuAllocationJob struct {
}

func (g *GpuAllocationJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()

	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	allocationRate, err := gpu.GetClusterGpuAllocationRate(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		return stats, err
	}
	allocationRateGauge.Set(allocationRate)

	stats.RecordsProcessed = 1
	stats.AddCustomMetric("allocation_rate", allocationRate)
	stats.AddMessage("GPU allocation rate updated successfully")

	return stats, nil
}

func (g *GpuAllocationJob) Schedule() string {
	return "@every 30s"
}
