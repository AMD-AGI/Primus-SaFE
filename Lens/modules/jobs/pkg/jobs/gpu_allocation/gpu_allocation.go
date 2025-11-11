package gpu_allocation

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "gpu_allocation_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	span.SetAttributes(
		attribute.String("job.name", "gpu_allocation"),
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	// Get cluster GPU allocation rate
	allocationSpan, allocationCtx := trace.StartSpanFromContext(ctx, "getClusterGpuAllocationRate")
	allocationSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	queryStart := time.Now()
	allocationRate, err := gpu.GetClusterGpuAllocationRate(allocationCtx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		allocationSpan.RecordError(err)
		allocationSpan.SetAttributes(attribute.String("error.message", err.Error()))
		allocationSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(allocationSpan)

		span.SetStatus(codes.Error, "Failed to get GPU allocation rate")
		return stats, err
	}

	duration := time.Since(queryStart)
	allocationSpan.SetAttributes(
		attribute.Float64("gpu.allocation_rate", allocationRate),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	allocationSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(allocationSpan)

	// Set Prometheus metric
	allocationRateGauge.Set(allocationRate)

	stats.RecordsProcessed = 1
	stats.AddCustomMetric("allocation_rate", allocationRate)
	stats.AddMessage("GPU allocation rate updated successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(
		attribute.Float64("gpu.allocation_rate", allocationRate),
		attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())),
	)
	span.SetStatus(codes.Ok, "")
	return stats, nil
}

func (g *GpuAllocationJob) Schedule() string {
	return "@every 30s"
}
