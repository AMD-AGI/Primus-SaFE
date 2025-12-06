package gpu_aggregation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/statistics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
)

const (
	// CacheKeyNamespaceGpuAggregationLastHour is the cache key for storing the last processed hour
	CacheKeyNamespaceGpuAggregationLastHour = "job.namespace_gpu_aggregation.last_processed_hour"
)

// NamespaceGpuAggregationConfig is the configuration for namespace GPU aggregation job
type NamespaceGpuAggregationConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// ExcludeNamespaces is the list of namespaces to exclude
	ExcludeNamespaces []string `json:"exclude_namespaces"`

	// IncludeSystemNamespaces controls whether to include system namespaces
	IncludeSystemNamespaces bool `json:"include_system_namespaces"`
}

// NamespaceGpuAggregationJob aggregates namespace-level GPU statistics using time-weighted calculation
type NamespaceGpuAggregationJob struct {
	config      *NamespaceGpuAggregationConfig
	clusterName string
}

// NewNamespaceGpuAggregationJob creates a new namespace GPU aggregation job
func NewNamespaceGpuAggregationJob() *NamespaceGpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &NamespaceGpuAggregationJob{
		config: &NamespaceGpuAggregationConfig{
			Enabled:                 true,
			ExcludeNamespaces:       []string{},
			IncludeSystemNamespaces: false,
		},
		clusterName: clusterName,
	}
}

// NewNamespaceGpuAggregationJobWithConfig creates a new namespace GPU aggregation job with custom config
func NewNamespaceGpuAggregationJobWithConfig(cfg *NamespaceGpuAggregationConfig) *NamespaceGpuAggregationJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &NamespaceGpuAggregationJob{
		config:      cfg,
		clusterName: clusterName,
	}
}

// Run executes the namespace GPU aggregation job
func (j *NamespaceGpuAggregationJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "namespace_gpu_aggregation_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	span.SetAttributes(
		attribute.String("job.name", "namespace_gpu_aggregation"),
		attribute.String("cluster.name", clusterName),
	)

	if !j.config.Enabled {
		log.Debugf("Namespace GPU aggregation job is disabled")
		stats.AddMessage("Namespace GPU aggregation job is disabled")
		return stats, nil
	}

	// Get the last processed hour from cache
	lastProcessedHour, err := j.getLastProcessedHour(ctx, clusterName)
	if err != nil {
		// If no cache entry exists, use current hour minus 1
		lastProcessedHour = time.Now().Truncate(time.Hour).Add(-time.Hour)
		log.Infof("No last processed hour found in cache, using default: %v", lastProcessedHour)
	}

	// Check if hour has changed - aggregate data for the previous hour
	now := time.Now()
	currentHour := now.Truncate(time.Hour)
	previousHour := currentHour.Add(-time.Hour)

	// If the last processed hour is before the previous hour, we need to aggregate
	if lastProcessedHour.Before(previousHour) || lastProcessedHour.Equal(previousHour) {
		hourToProcess := previousHour
		if lastProcessedHour.Before(previousHour) {
			// Process the hour after the last processed hour
			hourToProcess = lastProcessedHour.Add(time.Hour)
		}

		log.Infof("Processing namespace aggregation for hour: %v (last processed: %v)", hourToProcess, lastProcessedHour)

		aggSpan, aggCtx := trace.StartSpanFromContext(ctx, "aggregateNamespaceStats")
		aggSpan.SetAttributes(
			attribute.String("cluster.name", clusterName),
			attribute.String("stat.hour", hourToProcess.Format(time.RFC3339)),
		)

		count, err := j.aggregateNamespaceStats(aggCtx, clusterName, hourToProcess)
		if err != nil {
			aggSpan.RecordError(err)
			aggSpan.SetStatus(codes.Error, err.Error())
			trace.FinishSpan(aggSpan)
			stats.ErrorCount++
			log.Errorf("Failed to aggregate namespace stats: %v", err)
		} else {
			aggSpan.SetAttributes(attribute.Int64("namespaces.count", count))
			aggSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(aggSpan)
			stats.ItemsCreated = count
			stats.AddMessage(fmt.Sprintf("Aggregated %d namespace stats for %v", count, hourToProcess))

			// Update the last processed hour in cache
			if err := j.setLastProcessedHour(ctx, clusterName, hourToProcess); err != nil {
				log.Warnf("Failed to update last processed hour in cache: %v", err)
			}
		}
	}

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")

	stats.ProcessDuration = totalDuration.Seconds()
	return stats, nil
}

// getLastProcessedHour retrieves the last processed hour from cache
func (j *NamespaceGpuAggregationJob) getLastProcessedHour(ctx context.Context, clusterName string) (time.Time, error) {
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	var lastHourStr string
	err := cacheFacade.Get(ctx, CacheKeyNamespaceGpuAggregationLastHour, &lastHourStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return time.Time{}, fmt.Errorf("no cache entry found")
		}
		return time.Time{}, err
	}

	lastHour, err := time.Parse(time.RFC3339, lastHourStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse last processed hour: %w", err)
	}

	return lastHour, nil
}

// setLastProcessedHour stores the last processed hour in cache
func (j *NamespaceGpuAggregationJob) setLastProcessedHour(ctx context.Context, clusterName string, hour time.Time) error {
	cacheFacade := database.GetFacadeForCluster(clusterName).GetGenericCache()

	hourStr := hour.Format(time.RFC3339)
	return cacheFacade.Set(ctx, CacheKeyNamespaceGpuAggregationLastHour, hourStr, nil)
}

// aggregateNamespaceStats aggregates namespace-level statistics using time-weighted calculation
func (j *NamespaceGpuAggregationJob) aggregateNamespaceStats(
	ctx context.Context,
	clusterName string,
	hour time.Time) (int64, error) {

	// Get all namespaces from namespace_info table
	namespaceInfoList, err := database.GetFacadeForCluster(clusterName).GetNamespaceInfo().List(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list namespace info: %w", err)
	}

	// Build namespace list with quotas
	namespaceQuotas := make(map[string]int32)
	namespaces := make([]string, 0, len(namespaceInfoList))
	for _, nsInfo := range namespaceInfoList {
		if !j.shouldExcludeNamespace(nsInfo.Name) {
			namespaces = append(namespaces, nsInfo.Name)
			namespaceQuotas[nsInfo.Name] = nsInfo.GpuResource
		}
	}

	log.Infof("Aggregating stats for %d namespaces at hour %v", len(namespaces), hour)

	calculator := statistics.NewGpuAllocationCalculator(clusterName)
	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	var createdCount int64

	for _, namespace := range namespaces {
		// Use time-weighted calculation for this namespace
		result, err := calculator.CalculateHourlyNamespaceGpuAllocation(ctx, namespace, hour)
		if err != nil {
			log.Warnf("Failed to calculate GPU allocation for namespace %s at hour %v: %v",
				namespace, hour, err)
			result = &statistics.GpuAllocationResult{}
		}

		// Build namespace stats
		nsStats := &dbmodel.NamespaceGpuHourlyStats{
			ClusterName:         clusterName,
			Namespace:           namespace,
			StatHour:            hour,
			AllocatedGpuCount:   result.TotalAllocatedGpu,
			ActiveWorkloadCount: int32(result.WorkloadCount),
		}

		// Set GPU quota if available and calculate allocation rate
		if quota, exists := namespaceQuotas[namespace]; exists && quota > 0 {
			nsStats.TotalGpuCapacity = quota
			if nsStats.AllocatedGpuCount > 0 {
				nsStats.AllocationRate = (nsStats.AllocatedGpuCount / float64(quota)) * 100
			}
		}

		// Note: Utilization data will be aggregated from workload stats later
		// For now, we set utilization to 0

		// Save namespace stats
		if err := facade.SaveNamespaceHourlyStats(ctx, nsStats); err != nil {
			log.Errorf("Failed to save namespace stats for %s at %v: %v", namespace, hour, err)
			continue
		}

		createdCount++
		log.Debugf("Namespace stats saved for %s at %v: allocated=%.2f, workloads=%d",
			namespace, hour, nsStats.AllocatedGpuCount, result.WorkloadCount)
	}

	return createdCount, nil
}

// shouldExcludeNamespace checks if a namespace should be excluded
func (j *NamespaceGpuAggregationJob) shouldExcludeNamespace(namespace string) bool {
	// Check if in exclusion list
	for _, excluded := range j.config.ExcludeNamespaces {
		if namespace == excluded {
			return true
		}
	}

	// Check if it's a system namespace
	if !j.config.IncludeSystemNamespaces {
		systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}
		for _, sysNs := range systemNamespaces {
			if namespace == sysNs {
				return true
			}
		}
	}

	return false
}

// Schedule returns the job's scheduling expression
func (j *NamespaceGpuAggregationJob) Schedule() string {
	return "@every 5m"
}

// SetConfig sets the job configuration
func (j *NamespaceGpuAggregationJob) SetConfig(cfg *NamespaceGpuAggregationConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *NamespaceGpuAggregationJob) GetConfig() *NamespaceGpuAggregationConfig {
	return j.config
}
