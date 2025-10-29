package gpu_consumers

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
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

func (g *GpuConsumersJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSets *clientsets.StorageClientSet) error {
	consumers, err := gpu.GetGpuConsumerInfo(ctx, clientSets, storageClientSets, metadata.GpuVendorAMD)
	if err != nil {
		return err
	}
	for _, consumer := range consumers {
		consumerPodGpuUsage.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(consumer.Stat.GpuUtilization)
		consumerPodGpuAllocated.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(float64(consumer.Stat.GpuRequest))
		consumerActivePods.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(float64(len(consumer.Pods)))

	}
	return nil
}
func (g *GpuConsumersJob) Schedule() string {
	return "@every 30s"
}
