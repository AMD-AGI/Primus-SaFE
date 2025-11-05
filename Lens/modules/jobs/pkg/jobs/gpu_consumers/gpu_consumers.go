package gpu_consumers

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	consumerPodGpuUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "consumer_pod_gpu_usage",
		Help: "Gpu consumers usage",
	}, []string{"kind", "name", "uid"})
	consumerPodGpuAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "consumer_pod_gpu_allocated",
		Help: "Gpu consumers allocated",
	}, []string{"kind", "name", "uid"})
	consumerActivePods = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "consumer_pod_active",
		Help: "Consumers pod active",
	}, []string{"kind", "name", "uid"})
)

func init() {
	prometheus.MustRegister(consumerPodGpuUsage)
	prometheus.MustRegister(consumerPodGpuAllocated)
	prometheus.MustRegister(consumerActivePods)
}

type GpuConsumersJob struct {
}

func (g *GpuConsumersJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSets *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()
	
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	consumers, err := gpu.GetGpuConsumerInfo(ctx, clientSets, storageClientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		return stats, err
	}
	
	for _, consumer := range consumers {
		consumerPodGpuUsage.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(consumer.Stat.GpuUtilization)
		consumerPodGpuAllocated.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(float64(consumer.Stat.GpuRequest))
		consumerActivePods.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(float64(len(consumer.Pods)))
	}
	
	stats.RecordsProcessed = int64(len(consumers))
	stats.AddCustomMetric("consumers_count", len(consumers))
	stats.AddMessage("GPU consumers info updated successfully")
	
	return stats, nil
}
func (g *GpuConsumersJob) Schedule() string {
	return "@every 30s"
}
