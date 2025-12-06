package gpu_aggregation_backfill

import (
	"context"
	"fmt"
	"sort"
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
)

const (
	// DefaultLabelBackfillDays is the default number of days to backfill for label stats
	DefaultLabelBackfillDays = 7

	// DefaultLabelBatchSize is the default batch size for processing hours
	DefaultLabelBatchSize = 24
)

// LabelGpuAggregationBackfillConfig is the configuration for label backfill job
type LabelGpuAggregationBackfillConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// BackfillDays is the number of days to scan for missing data
	BackfillDays int `json:"backfill_days"`

	// BatchSize is the number of hours to process in each batch
	BatchSize int `json:"batch_size"`

	// LabelKeys is the list of label keys to aggregate
	LabelKeys []string `json:"label_keys"`

	// AnnotationKeys is the list of annotation keys to aggregate
	AnnotationKeys []string `json:"annotation_keys"`

	// DefaultValue is the default value when label/annotation is not found
	DefaultValue string `json:"default_value"`
}

// LabelGpuAggregationBackfillJob is the job for backfilling missing label GPU aggregation data
type LabelGpuAggregationBackfillJob struct {
	config      *LabelGpuAggregationBackfillConfig
	clusterName string
}

// NewLabelGpuAggregationBackfillJob creates a new label backfill job with default config
func NewLabelGpuAggregationBackfillJob() *LabelGpuAggregationBackfillJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &LabelGpuAggregationBackfillJob{
		config: &LabelGpuAggregationBackfillConfig{
			Enabled:        true,
			BackfillDays:   DefaultLabelBackfillDays,
			BatchSize:      DefaultLabelBatchSize,
			LabelKeys:      []string{},
			AnnotationKeys: []string{},
			DefaultValue:   "unknown",
		},
		clusterName: clusterName,
	}
}

// NewLabelGpuAggregationBackfillJobWithConfig creates a new label backfill job with custom config
func NewLabelGpuAggregationBackfillJobWithConfig(cfg *LabelGpuAggregationBackfillConfig) *LabelGpuAggregationBackfillJob {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	return &LabelGpuAggregationBackfillJob{
		config:      cfg,
		clusterName: clusterName,
	}
}

// Run executes the label backfill job
func (j *LabelGpuAggregationBackfillJob) Run(ctx context.Context,
	k8sClientSet *clientsets.K8SClientSet,
	storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {

	span, ctx := trace.StartSpanFromContext(ctx, "label_gpu_aggregation_backfill_job.Run")
	defer trace.FinishSpan(span)

	stats := common.NewExecutionStats()
	jobStartTime := time.Now()

	clusterName := j.clusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	span.SetAttributes(
		attribute.String("job.name", "label_gpu_aggregation_backfill"),
		attribute.String("cluster.name", clusterName),
		attribute.Int("config.backfill_days", j.config.BackfillDays),
	)

	if !j.config.Enabled {
		log.Debugf("Label GPU aggregation backfill job is disabled")
		stats.AddMessage("Label GPU aggregation backfill job is disabled")
		return stats, nil
	}

	// Check if there are any keys to aggregate
	if len(j.config.LabelKeys) == 0 && len(j.config.AnnotationKeys) == 0 {
		log.Debugf("No label or annotation keys configured for backfill")
		stats.AddMessage("No label or annotation keys configured")
		return stats, nil
	}

	// Calculate time range
	// Exclude current hour to avoid conflict with ongoing aggregation
	endTime := time.Now().Truncate(time.Hour).Add(-time.Hour)
	startTime := endTime.Add(-time.Duration(j.config.BackfillDays) * 24 * time.Hour)

	log.Infof("Starting label GPU aggregation backfill job for cluster: %s, time range: %v to %v (excluding current hour)",
		clusterName, startTime, endTime)

	// 1. Generate all hours in the time range
	allHours := generateAllHours(startTime, endTime)
	log.Infof("Generated %d hours to check for label backfill", len(allHours))

	if len(allHours) == 0 {
		log.Infof("No hours to process")
		stats.AddMessage("No hours to process")
		return stats, nil
	}

	// 2. Find missing label stats for all hours
	missingSpan, missingCtx := trace.StartSpanFromContext(ctx, "findMissingLabelStats")
	missingLabelHours, err := j.findMissingLabelStats(missingCtx, clusterName, allHours)
	if err != nil {
		missingSpan.RecordError(err)
		missingSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(missingSpan)
		return stats, fmt.Errorf("failed to find missing label stats: %w", err)
	}
	missingSpan.SetAttributes(attribute.Int("missing.label_hours", len(missingLabelHours)))
	missingSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(missingSpan)

	log.Infof("Found %d missing label hours", len(missingLabelHours))
	stats.AddCustomMetric("missing_label_hours", len(missingLabelHours))

	// 3. Backfill label stats
	if len(missingLabelHours) > 0 {
		backfillSpan, backfillCtx := trace.StartSpanFromContext(ctx, "backfillLabelStats")
		backfillSpan.SetAttributes(attribute.Int("hours.count", len(missingLabelHours)))

		count, backfillErr := j.backfillLabelStats(backfillCtx, clusterName, missingLabelHours)
		if backfillErr != nil {
			backfillSpan.RecordError(backfillErr)
			backfillSpan.SetStatus(codes.Error, backfillErr.Error())
			trace.FinishSpan(backfillSpan)
			stats.ErrorCount++
			log.Errorf("Failed to backfill label stats: %v", backfillErr)
		} else {
			backfillSpan.SetAttributes(attribute.Int64("backfilled.count", count))
			backfillSpan.SetStatus(codes.Ok, "")
			trace.FinishSpan(backfillSpan)
			stats.ItemsCreated = count
			log.Infof("Backfilled %d label hourly stats", count)
		}
	}

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")

	stats.ProcessDuration = totalDuration.Seconds()
	stats.AddMessage(fmt.Sprintf("Label backfill completed: %d label stats created", stats.ItemsCreated))

	log.Infof("Label GPU aggregation backfill job completed in %v", totalDuration)
	return stats, nil
}

// findMissingLabelStats finds hours that are missing label stats
// Returns a slice of hours that need backfill
func (j *LabelGpuAggregationBackfillJob) findMissingLabelStats(
	ctx context.Context,
	clusterName string,
	allHours []time.Time) ([]time.Time, error) {

	if len(allHours) == 0 {
		return nil, nil
	}

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()

	startTime := allHours[0]
	endTime := allHours[len(allHours)-1].Add(time.Hour)

	// Get existing label stats for all configured keys
	existingStats := make(map[time.Time]map[string]struct{})

	// Query existing stats for each label key
	for _, labelKey := range j.config.LabelKeys {
		stats, err := facade.ListLabelHourlyStatsByKey(ctx, statistics.DimensionTypeLabel, labelKey, startTime, endTime)
		if err != nil {
			log.Warnf("Failed to query existing label stats for key %s: %v", labelKey, err)
			continue
		}

		for _, stat := range stats {
			hour := stat.StatHour.Truncate(time.Hour)
			if existingStats[hour] == nil {
				existingStats[hour] = make(map[string]struct{})
			}
			key := statistics.BuildDimensionKey(stat.DimensionType, stat.DimensionKey, stat.DimensionValue)
			existingStats[hour][key] = struct{}{}
		}
	}

	// Query existing stats for each annotation key
	for _, annotationKey := range j.config.AnnotationKeys {
		stats, err := facade.ListLabelHourlyStatsByKey(ctx, statistics.DimensionTypeAnnotation, annotationKey, startTime, endTime)
		if err != nil {
			log.Warnf("Failed to query existing annotation stats for key %s: %v", annotationKey, err)
			continue
		}

		for _, stat := range stats {
			hour := stat.StatHour.Truncate(time.Hour)
			if existingStats[hour] == nil {
				existingStats[hour] = make(map[string]struct{})
			}
			key := statistics.BuildDimensionKey(stat.DimensionType, stat.DimensionKey, stat.DimensionValue)
			existingStats[hour][key] = struct{}{}
		}
	}

	// Find missing hours - hours without any label stats for configured keys
	var missingHours []time.Time

	for _, hour := range allHours {
		existingForHour := existingStats[hour]
		if existingForHour == nil {
			// No stats exist for this hour at all - needs backfill
			missingHours = append(missingHours, hour)
			continue
		}

		// Check if all configured keys have stats for this hour
		hasAllKeys := j.checkAllKeysExist(existingForHour)
		if !hasAllKeys {
			missingHours = append(missingHours, hour)
		}
	}

	return missingHours, nil
}

// checkAllKeysExist checks if all configured keys have stats for an hour
func (j *LabelGpuAggregationBackfillJob) checkAllKeysExist(existingForHour map[string]struct{}) bool {
	// Check label keys
	for _, labelKey := range j.config.LabelKeys {
		found := false
		prefix := fmt.Sprintf("%s:%s:", statistics.DimensionTypeLabel, labelKey)
		for key := range existingForHour {
			if len(key) > len(prefix) && key[:len(prefix)] == prefix {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check annotation keys
	for _, annotationKey := range j.config.AnnotationKeys {
		found := false
		prefix := fmt.Sprintf("%s:%s:", statistics.DimensionTypeAnnotation, annotationKey)
		for key := range existingForHour {
			if len(key) > len(prefix) && key[:len(prefix)] == prefix {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// backfillLabelStats backfills missing label hourly stats
func (j *LabelGpuAggregationBackfillJob) backfillLabelStats(
	ctx context.Context,
	clusterName string,
	missingHours []time.Time) (int64, error) {

	if len(missingHours) == 0 {
		return 0, nil
	}

	facade := database.GetFacadeForCluster(clusterName).GetGpuAggregation()
	var createdCount int64

	// Sort hours for consistent processing
	sort.Slice(missingHours, func(i, j int) bool {
		return missingHours[i].Before(missingHours[j])
	})

	// Create calculator with configuration
	calculator := statistics.NewLabelAggregationCalculator(clusterName, &statistics.LabelAggregationConfig{
		LabelKeys:      j.config.LabelKeys,
		AnnotationKeys: j.config.AnnotationKeys,
		DefaultValue:   j.config.DefaultValue,
	})

	for _, hour := range missingHours {
		// Calculate aggregation for this hour
		summary, err := calculator.CalculateHourlyLabelAggregation(ctx, hour)
		if err != nil {
			log.Warnf("Failed to calculate label aggregation for hour %v: %v", hour, err)
			continue
		}

		if summary.TotalWorkloads == 0 {
			// No workloads for this hour - create zero-value stats for all configured keys
			count := j.createZeroValueStats(ctx, facade, clusterName, hour)
			createdCount += count
			continue
		}

		// Save aggregations to database
		for _, agg := range summary.Results {
			stats := &dbmodel.LabelGpuHourlyStats{
				ClusterName:         clusterName,
				DimensionType:       agg.DimensionType,
				DimensionKey:        agg.DimensionKey,
				DimensionValue:      agg.DimensionValue,
				StatHour:            hour,
				AllocatedGpuCount:   agg.TotalAllocatedGpu,
				ActiveWorkloadCount: int32(agg.ActiveWorkloadCount),
				// Note: Utilization is not available for backfill (no Prometheus data)
				AvgUtilization: 0,
				MaxUtilization: 0,
				MinUtilization: 0,
			}

			if err := facade.SaveLabelHourlyStats(ctx, stats); err != nil {
				log.Errorf("Failed to save label stats for %s:%s=%s at %v: %v",
					agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour, err)
				continue
			}

			createdCount++
			log.Debugf("Backfilled label stats for %s:%s=%s at %v: allocated=%.2f, workloads=%d",
				agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour,
				stats.AllocatedGpuCount, agg.ActiveWorkloadCount)
		}
	}

	return createdCount, nil
}

// createZeroValueStats creates zero-value stats for all configured keys
func (j *LabelGpuAggregationBackfillJob) createZeroValueStats(
	ctx context.Context,
	facade database.GpuAggregationFacadeInterface,
	clusterName string,
	hour time.Time) int64 {

	var count int64

	// Create zero-value stats for label keys with default value
	for _, labelKey := range j.config.LabelKeys {
		stats := &dbmodel.LabelGpuHourlyStats{
			ClusterName:         clusterName,
			DimensionType:       statistics.DimensionTypeLabel,
			DimensionKey:        labelKey,
			DimensionValue:      j.config.DefaultValue,
			StatHour:            hour,
			AllocatedGpuCount:   0,
			ActiveWorkloadCount: 0,
			AvgUtilization:      0,
			MaxUtilization:      0,
			MinUtilization:      0,
		}

		if err := facade.SaveLabelHourlyStats(ctx, stats); err != nil {
			log.Warnf("Failed to save zero-value label stats for %s at %v: %v", labelKey, hour, err)
			continue
		}
		count++
	}

	// Create zero-value stats for annotation keys with default value
	for _, annotationKey := range j.config.AnnotationKeys {
		stats := &dbmodel.LabelGpuHourlyStats{
			ClusterName:         clusterName,
			DimensionType:       statistics.DimensionTypeAnnotation,
			DimensionKey:        annotationKey,
			DimensionValue:      j.config.DefaultValue,
			StatHour:            hour,
			AllocatedGpuCount:   0,
			ActiveWorkloadCount: 0,
			AvgUtilization:      0,
			MaxUtilization:      0,
			MinUtilization:      0,
		}

		if err := facade.SaveLabelHourlyStats(ctx, stats); err != nil {
			log.Warnf("Failed to save zero-value annotation stats for %s at %v: %v", annotationKey, hour, err)
			continue
		}
		count++
	}

	return count
}

// Schedule returns the job's scheduling expression
func (j *LabelGpuAggregationBackfillJob) Schedule() string {
	return "@every 5m"
}

// SetConfig sets the job configuration
func (j *LabelGpuAggregationBackfillJob) SetConfig(cfg *LabelGpuAggregationBackfillConfig) {
	j.config = cfg
}

// GetConfig returns the current configuration
func (j *LabelGpuAggregationBackfillJob) GetConfig() *LabelGpuAggregationBackfillConfig {
	return j.config
}
