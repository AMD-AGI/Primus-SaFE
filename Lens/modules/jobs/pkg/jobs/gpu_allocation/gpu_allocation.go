package gpu_allocation

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
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

func (g *GpuAllocationJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error {
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	allocationRate, err := gpu.GetClusterGpuAllocationRate(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		return err
	}
	allocationRateGauge.Set(allocationRate)
	return nil
}

func (g *GpuAllocationJob) Schedule() string {
	return "@every 30s"
}
