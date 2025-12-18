package gpu_aggregation_backfill

import (
	"context"
	"encoding/json"
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

	// SystemConfigKeyGpuAggregationBackfill is the system_config key for GPU aggregation configuration
	SystemConfigKeyGpuAggregationBackfill = "job.gpu_aggregation.config"
)

// LabelBackfillFacadeGetter is the function signature for getting database facade
type LabelBackfillFacadeGetter func(clusterName string) database.FacadeInterface

// LabelBackfillClusterNameGetter is the function signature for getting cluster name
type LabelBackfillClusterNameGetter func() string

// LabelBackfillAggregationCalculatorFactory creates a label aggregation calculator
type LabelBackfillAggregationCalculatorFactory func(clusterName string, config *statistics.LabelAggregationConfig) LabelBackfillAggregationCalculatorInterface

// LabelBackfillUtilizationCalcFunc calculates weighted utilization for workloads
type LabelBackfillUtilizationCalcFunc func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, workloadGpuCounts map[string]int32, startTime, endTime time.Time, step int) statistics.UtilizationStats

// LabelBackfillAggregationCalculatorInterface defines the interface for label aggregation calculation
type LabelBackfillAggregationCalculatorInterface interface {
	CalculateHourlyLabelAggregation(ctx context.Context, hour time.Time) (*statistics.LabelAggregationSummary, error)
}

// LabelGpuAggregationSystemConfig represents the full system_config structure for label GPU aggregation
type LabelGpuAggregationSystemConfig struct {
	Dimensions struct {
		Label struct {
			Enabled        bool     `json:"enabled"`
			LabelKeys      []string `json:"label_keys"`
			AnnotationKeys []string `json:"annotation_keys"`
			DefaultValue   string   `json:"default_value"`
		} `json:"label"`
	} `json:"dimensions"`
}

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
	config                       *LabelGpuAggregationBackfillConfig
	clusterName                  string
	facadeGetter                 LabelBackfillFacadeGetter
	clusterNameGetter            LabelBackfillClusterNameGetter
	aggregationCalculatorFactory LabelBackfillAggregationCalculatorFactory
	utilizationCalcFunc          LabelBackfillUtilizationCalcFunc
}

// LabelBackfillJobOption is a function that configures a LabelGpuAggregationBackfillJob
type LabelBackfillJobOption func(*LabelGpuAggregationBackfillJob)

// WithLabelBackfillFacadeGetter sets the facade getter function
func WithLabelBackfillFacadeGetter(getter LabelBackfillFacadeGetter) LabelBackfillJobOption {
	return func(j *LabelGpuAggregationBackfillJob) {
		j.facadeGetter = getter
	}
}

// WithLabelBackfillClusterNameGetter sets the cluster name getter function
func WithLabelBackfillClusterNameGetter(getter LabelBackfillClusterNameGetter) LabelBackfillJobOption {
	return func(j *LabelGpuAggregationBackfillJob) {
		j.clusterNameGetter = getter
	}
}

// WithLabelBackfillClusterName sets the cluster name directly
func WithLabelBackfillClusterName(name string) LabelBackfillJobOption {
	return func(j *LabelGpuAggregationBackfillJob) {
		j.clusterName = name
	}
}

// WithLabelBackfillAggregationCalculatorFactory sets the aggregation calculator factory
func WithLabelBackfillAggregationCalculatorFactory(factory LabelBackfillAggregationCalculatorFactory) LabelBackfillJobOption {
	return func(j *LabelGpuAggregationBackfillJob) {
		j.aggregationCalculatorFactory = factory
	}
}

// WithLabelBackfillUtilizationCalcFunc sets the utilization calculation function
func WithLabelBackfillUtilizationCalcFunc(fn LabelBackfillUtilizationCalcFunc) LabelBackfillJobOption {
	return func(j *LabelGpuAggregationBackfillJob) {
		j.utilizationCalcFunc = fn
	}
}

// defaultLabelBackfillFacadeGetter is the default implementation
func defaultLabelBackfillFacadeGetter(clusterName string) database.FacadeInterface {
	return database.GetFacadeForCluster(clusterName)
}

// defaultLabelBackfillClusterNameGetter is the default implementation
func defaultLabelBackfillClusterNameGetter() string {
	return clientsets.GetClusterManager().GetCurrentClusterName()
}

// defaultLabelBackfillAggregationCalculatorFactory is the default implementation
func defaultLabelBackfillAggregationCalculatorFactory(clusterName string, config *statistics.LabelAggregationConfig) LabelBackfillAggregationCalculatorInterface {
	return statistics.NewLabelAggregationCalculator(clusterName, config)
}

// defaultLabelBackfillUtilizationCalcFunc is the default implementation
func defaultLabelBackfillUtilizationCalcFunc(ctx context.Context, storageClientSet *clientsets.StorageClientSet, workloadGpuCounts map[string]int32, startTime, endTime time.Time, step int) statistics.UtilizationStats {
	return statistics.CalculateWorkloadsUtilizationWeighted(ctx, storageClientSet, workloadGpuCounts, startTime, endTime, step)
}

// NewLabelGpuAggregationBackfillJob creates a new label backfill job with default config
func NewLabelGpuAggregationBackfillJob(opts ...LabelBackfillJobOption) *LabelGpuAggregationBackfillJob {
	j := &LabelGpuAggregationBackfillJob{
		config: &LabelGpuAggregationBackfillConfig{
			Enabled:        true,
			BackfillDays:   DefaultLabelBackfillDays,
			BatchSize:      DefaultLabelBatchSize,
			LabelKeys:      []string{},
			AnnotationKeys: []string{},
			DefaultValue:   "unknown",
		},
		facadeGetter:                 defaultLabelBackfillFacadeGetter,
		clusterNameGetter:            defaultLabelBackfillClusterNameGetter,
		aggregationCalculatorFactory: defaultLabelBackfillAggregationCalculatorFactory,
		utilizationCalcFunc:          defaultLabelBackfillUtilizationCalcFunc,
	}

	for _, opt := range opts {
		opt(j)
	}

	if j.clusterName == "" {
		j.clusterName = j.clusterNameGetter()
	}

	return j
}

// NewLabelGpuAggregationBackfillJobWithConfig creates a new label backfill job with custom config
func NewLabelGpuAggregationBackfillJobWithConfig(cfg *LabelGpuAggregationBackfillConfig, opts ...LabelBackfillJobOption) *LabelGpuAggregationBackfillJob {
	j := &LabelGpuAggregationBackfillJob{
		config:                       cfg,
		facadeGetter:                 defaultLabelBackfillFacadeGetter,
		clusterNameGetter:            defaultLabelBackfillClusterNameGetter,
		aggregationCalculatorFactory: defaultLabelBackfillAggregationCalculatorFactory,
		utilizationCalcFunc:          defaultLabelBackfillUtilizationCalcFunc,
	}

	for _, opt := range opts {
		opt(j)
	}

	if j.clusterName == "" {
		j.clusterName = j.clusterNameGetter()
	}

	return j
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
		clusterName = j.clusterNameGetter()
	}

	span.SetAttributes(
		attribute.String("job.name", "label_gpu_aggregation_backfill"),
		attribute.String("cluster.name", clusterName),
		attribute.Int("config.backfill_days", j.config.BackfillDays),
	)

	// Load configuration from system_config
	if err := j.loadConfigFromSystemConfig(ctx, clusterName); err != nil {
		log.Warnf("Failed to load label aggregation config from system_config, using defaults: %v", err)
	}

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
	log.Infof("Generated %d hours to process for label backfill", len(allHours))

	if len(allHours) == 0 {
		log.Infof("No hours to process")
		stats.AddMessage("No hours to process")
		return stats, nil
	}

	// 2. Process each hour and backfill missing stats
	backfillSpan, backfillCtx := trace.StartSpanFromContext(ctx, "backfillLabelStats")
	backfillSpan.SetAttributes(attribute.Int("hours.count", len(allHours)))

	count, skipped, backfillErr := j.backfillLabelStats(backfillCtx, clusterName, allHours, storageClientSet)
	if backfillErr != nil {
		backfillSpan.RecordError(backfillErr)
		backfillSpan.SetStatus(codes.Error, backfillErr.Error())
		trace.FinishSpan(backfillSpan)
		stats.ErrorCount++
		log.Errorf("Failed to backfill label stats: %v", backfillErr)
	} else {
		backfillSpan.SetAttributes(
			attribute.Int64("backfilled.count", count),
			attribute.Int64("skipped.count", skipped),
		)
		backfillSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(backfillSpan)
		stats.ItemsCreated = count
		stats.AddCustomMetric("skipped_existing", int(skipped))
		log.Infof("Backfilled %d label hourly stats, skipped %d existing", count, skipped)
	}

	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")

	stats.ProcessDuration = totalDuration.Seconds()
	stats.AddMessage(fmt.Sprintf("Label backfill completed: %d label stats created", stats.ItemsCreated))

	log.Infof("Label GPU aggregation backfill job completed in %v", totalDuration)
	return stats, nil
}

// backfillLabelStats processes each hour, calculates aggregation, and saves only missing stats
// Returns: (created count, skipped count, error)
func (j *LabelGpuAggregationBackfillJob) backfillLabelStats(
	ctx context.Context,
	clusterName string,
	allHours []time.Time,
	storageClientSet *clientsets.StorageClientSet) (int64, int64, error) {

	if len(allHours) == 0 {
		return 0, 0, nil
	}

	facade := j.facadeGetter(clusterName).GetGpuAggregation()
	var createdCount, skippedCount int64

	// Sort hours for consistent processing
	sort.Slice(allHours, func(i, k int) bool {
		return allHours[i].Before(allHours[k])
	})

	// Create calculator with configuration
	calculator := j.aggregationCalculatorFactory(clusterName, &statistics.LabelAggregationConfig{
		LabelKeys:      j.config.LabelKeys,
		AnnotationKeys: j.config.AnnotationKeys,
		DefaultValue:   j.config.DefaultValue,
	})

	for _, hour := range allHours {
		hourStart := hour
		hourEnd := hour.Add(time.Hour)

		// Calculate aggregation for this hour
		summary, err := calculator.CalculateHourlyLabelAggregation(ctx, hour)
		if err != nil {
			log.Warnf("Failed to calculate label aggregation for hour %v: %v", hour, err)
			continue
		}

		if summary.TotalWorkloads == 0 {
			// No workloads for this hour - skip (no need to create zero-value stats)
			log.Debugf("No workloads found for hour %v, skipping", hour)
			continue
		}

		// Process each aggregation result
		for _, agg := range summary.Results {
			// Check if this specific key-value-hour combination already exists
			exists, err := facade.LabelHourlyStatsExists(ctx, clusterName, agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour)
			if err != nil {
				log.Warnf("Failed to check existence for %s:%s=%s at %v: %v",
					agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour, err)
				continue
			}

			if exists {
				// Already exists, skip
				skippedCount++
				log.Debugf("Skipped existing label stats for %s:%s=%s at %v",
					agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour)
				continue
			}

			// Query utilization for all workloads in this aggregation (weighted by GPU count)
			utilizationStats := j.utilizationCalcFunc(ctx, storageClientSet, agg.WorkloadGpuCounts, hourStart, hourEnd, 0)

			// Create new stats record
			stats := BuildLabelStatsFromAggregation(clusterName, hour, agg, &utilizationStats)

			if err := facade.SaveLabelHourlyStats(ctx, stats); err != nil {
				log.Errorf("Failed to save label stats for %s:%s=%s at %v: %v",
					agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour, err)
				continue
			}

			createdCount++
			log.Debugf("Backfilled label stats for %s:%s=%s at %v: allocated=%.2f, workloads=%d, avgUtil=%.2f%%",
				agg.DimensionType, agg.DimensionKey, agg.DimensionValue, hour,
				stats.AllocatedGpuCount, agg.ActiveWorkloadCount, stats.AvgUtilization)
		}
	}

	return createdCount, skippedCount, nil
}

// BuildLabelStatsFromAggregation builds LabelGpuHourlyStats from aggregation result
// This is exported for testing purposes
func BuildLabelStatsFromAggregation(
	clusterName string,
	hour time.Time,
	agg *statistics.LabelAggregationResult,
	utilizationStats *statistics.UtilizationStats) *dbmodel.LabelGpuHourlyStats {

	stats := &dbmodel.LabelGpuHourlyStats{
		ClusterName:         clusterName,
		DimensionType:       agg.DimensionType,
		DimensionKey:        agg.DimensionKey,
		DimensionValue:      agg.DimensionValue,
		StatHour:            hour,
		AllocatedGpuCount:   agg.TotalAllocatedGpu,
		ActiveWorkloadCount: int32(agg.ActiveWorkloadCount),
	}

	if utilizationStats != nil {
		stats.AvgUtilization = utilizationStats.AvgUtilization
		stats.MaxUtilization = utilizationStats.MaxUtilization
		stats.MinUtilization = utilizationStats.MinUtilization
	}

	return stats
}

// loadConfigFromSystemConfig loads configuration from system_config table
func (j *LabelGpuAggregationBackfillJob) loadConfigFromSystemConfig(ctx context.Context, clusterName string) error {
	configFacade := j.facadeGetter(clusterName).GetSystemConfig()
	sysConfig, err := configFacade.GetByKey(ctx, SystemConfigKeyGpuAggregationBackfill)
	if err != nil {
		return fmt.Errorf("failed to get system config: %w", err)
	}
	if sysConfig == nil {
		return fmt.Errorf("system config key %s not found", SystemConfigKeyGpuAggregationBackfill)
	}

	// Parse the JSON value
	configBytes, err := json.Marshal(sysConfig.Value)
	if err != nil {
		return fmt.Errorf("failed to marshal config value: %w", err)
	}

	var gpuAggConfig LabelGpuAggregationSystemConfig
	if err := json.Unmarshal(configBytes, &gpuAggConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Update job config from system config
	labelConfig := gpuAggConfig.Dimensions.Label
	j.config.Enabled = labelConfig.Enabled
	j.config.LabelKeys = labelConfig.LabelKeys
	j.config.AnnotationKeys = labelConfig.AnnotationKeys
	j.config.DefaultValue = labelConfig.DefaultValue

	log.Debugf("Loaded label backfill config: enabled=%v, labelKeys=%v, annotationKeys=%v, defaultValue=%s",
		j.config.Enabled, j.config.LabelKeys, j.config.AnnotationKeys, j.config.DefaultValue)

	return nil
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
