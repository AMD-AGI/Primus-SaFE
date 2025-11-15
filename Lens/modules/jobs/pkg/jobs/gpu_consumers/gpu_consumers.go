package gpu_consumers

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "gpu_consumers_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	span.SetAttributes(
		attribute.String("job.name", "gpu_consumers"),
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	// Get GPU consumer info
	consumerSpan, consumerCtx := trace.StartSpanFromContext(ctx, "getGpuConsumerInfo")
	consumerSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	queryStart := time.Now()
	consumers, err := gpu.GetGpuConsumerInfo(consumerCtx, clientSets, storageClientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		consumerSpan.RecordError(err)
		consumerSpan.SetAttributes(attribute.String("error.message", err.Error()))
		consumerSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(consumerSpan)

		span.SetStatus(codes.Error, "Failed to get GPU consumer info")
		return stats, err
	}

	duration := time.Since(queryStart)
	consumerSpan.SetAttributes(
		attribute.Int("consumers.count", len(consumers)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	consumerSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(consumerSpan)

	// Update Prometheus metrics
	metricsSpan, _ := trace.StartSpanFromContext(ctx, "updatePrometheusMetrics")
	metricsSpan.SetAttributes(attribute.Int("consumers.count", len(consumers)))

	metricsStart := time.Now()
	totalPods := 0
	totalGpuAllocated := 0
	for _, consumer := range consumers {
		consumerPodGpuUsage.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(consumer.Stat.GpuUtilization)
		consumerPodGpuAllocated.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(float64(consumer.Stat.GpuRequest))
		consumerActivePods.WithLabelValues(consumer.Kind, consumer.Name, consumer.Uid).Set(float64(len(consumer.Pods)))

		totalPods += len(consumer.Pods)
		totalGpuAllocated += consumer.Stat.GpuRequest
	}

	duration = time.Since(metricsStart)
	metricsSpan.SetAttributes(
		attribute.Int("metrics.total_pods", totalPods),
		attribute.Int("metrics.total_gpu_allocated", totalGpuAllocated),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	metricsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(metricsSpan)

	stats.RecordsProcessed = int64(len(consumers))
	stats.AddCustomMetric("consumers_count", len(consumers))
	stats.AddMessage("GPU consumers info updated successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Int("consumers.count", len(consumers)),
		attribute.Int("total_pods", totalPods),
		attribute.Int("total_gpu_allocated", totalGpuAllocated),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)
	span.SetStatus(codes.Ok, "")
	return stats, nil
}
func (g *GpuConsumersJob) Schedule() string {
	return "@every 30s"
}
