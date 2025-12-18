package gpu_aggregation

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

const (
	// ConfigKeyGpuAggregation is the configuration key for GPU aggregation task
	ConfigKeyGpuAggregation = "job.gpu_aggregation.config"
)

// InitDefaultConfig initializes default configuration (if config doesn't exist)
func InitDefaultConfig(ctx context.Context, clusterName string) error {
	configManager := config.GetConfigManagerForCluster(clusterName)

	// Check if configuration already exists
	exists, err := configManager.Exists(ctx, ConfigKeyGpuAggregation)
	if err != nil {
		return fmt.Errorf("failed to check config existence: %w", err)
	}

	if exists {
		log.Infof("GPU aggregation config already exists for cluster: %s", clusterName)
		return nil
	}

	// Create default configuration
	defaultConfig := &model.GpuAggregationConfig{
		Enabled: true,
	}

	// Sampling configuration
	defaultConfig.Sampling.Enabled = true
	defaultConfig.Sampling.Interval = "5m"
	defaultConfig.Sampling.Timeout = "2m"

	// Aggregation configuration
	defaultConfig.Aggregation.Enabled = true
	defaultConfig.Aggregation.TriggerOffsetMinutes = 5
	defaultConfig.Aggregation.Timeout = "5m"

	// Dimension configuration
	defaultConfig.Dimensions.Cluster.Enabled = true

	defaultConfig.Dimensions.Namespace.Enabled = true
	defaultConfig.Dimensions.Namespace.IncludeSystemNamespaces = false
	defaultConfig.Dimensions.Namespace.ExcludeNamespaces = []string{}

	defaultConfig.Dimensions.Label.Enabled = true
	defaultConfig.Dimensions.Label.LabelKeys = []string{"app", "team", "env"}
	defaultConfig.Dimensions.Label.AnnotationKeys = []string{"project", "cost-center"}
	defaultConfig.Dimensions.Label.DefaultValue = "unknown"

	defaultConfig.Dimensions.Workload.Enabled = true

	// Prometheus configuration
	defaultConfig.Prometheus.WorkloadUtilizationQuery = `avg(workload_gpu_utilization{workload_uid="%s"})`
	defaultConfig.Prometheus.GpuMemoryUsedQuery = `avg(workload_gpu_used_vram{workload_uid="%s"})`
	defaultConfig.Prometheus.GpuMemoryTotalQuery = `avg(workload_gpu_total_vram{workload_uid="%s"})`
	defaultConfig.Prometheus.QueryStep = 30 // 30 seconds
	defaultConfig.Prometheus.QueryTimeout = "30s"

	// Save default configuration
	err = configManager.Set(ctx, ConfigKeyGpuAggregation, defaultConfig,
		config.WithDescription("GPU utilization aggregation job configuration"),
		config.WithCategory("job"),
		config.WithCreatedBy("system"),
		config.WithRecordHistory(true),
	)
	if err != nil {
		return fmt.Errorf("failed to save default config: %w", err)
	}

	log.Infof("Default GPU aggregation config initialized for cluster: %s", clusterName)
	return nil
}

// GetConfig loads configuration from config manager for the specified cluster
func GetConfig(ctx context.Context, clusterName string) (*model.GpuAggregationConfig, error) {
	configManager := config.GetConfigManagerForCluster(clusterName)
	var cfg model.GpuAggregationConfig

	err := configManager.Get(ctx, ConfigKeyGpuAggregation, &cfg)
	if err != nil {
		return nil, fmt.Errorf("config not found: %w", err)
	}

	return &cfg, nil
}

// UpdateConfig updates configuration to config manager
func UpdateConfig(ctx context.Context, clusterName string, cfg *model.GpuAggregationConfig, updatedBy string) error {
	configManager := config.GetConfigManagerForCluster(clusterName)

	err := configManager.Set(ctx, ConfigKeyGpuAggregation, cfg,
		config.WithDescription("GPU utilization aggregation job configuration"),
		config.WithCategory("job"),
		config.WithUpdatedBy(updatedBy),
		config.WithRecordHistory(true),
		config.WithChangeReason("Update GPU aggregation job config"),
	)
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	log.Infof("GPU aggregation config updated successfully by %s", updatedBy)
	return nil
}
