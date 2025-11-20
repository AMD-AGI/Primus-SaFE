package model

import "time"

// ClusterGpuHourlyStats is cluster-level hourly aggregated statistics
type ClusterGpuHourlyStats struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName string    `gorm:"type:varchar(100);not null;index:idx_cluster_hour" json:"clusterName"`
	StatHour    time.Time `gorm:"type:timestamp;not null;index:idx_cluster_hour;index:idx_hour" json:"statHour"`

	// GPU allocation statistics
	TotalGpuCapacity  int     `gorm:"not null;default:0" json:"totalGpuCapacity"`
	AllocatedGpuCount float64 `gorm:"type:double precision;not null;default:0" json:"allocatedGpuCount"`
	AllocationRate    float64 `gorm:"type:double precision;not null;default:0" json:"allocationRate"`

	// GPU utilization statistics
	AvgUtilization float64 `gorm:"type:double precision;not null;default:0" json:"avgUtilization"`
	MaxUtilization float64 `gorm:"type:double precision;not null;default:0" json:"maxUtilization"`
	MinUtilization float64 `gorm:"type:double precision;not null;default:0" json:"minUtilization"`
	P50Utilization float64 `gorm:"type:double precision;not null;default:0" json:"p50Utilization"`
	P95Utilization float64 `gorm:"type:double precision;not null;default:0" json:"p95Utilization"`

	// Sampling statistics
	SampleCount int `gorm:"not null;default:0" json:"sampleCount"`

	// Timestamps
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName specifies the table name
func (ClusterGpuHourlyStats) TableName() string {
	return "cluster_gpu_hourly_stats"
}

// NamespaceGpuHourlyStats is namespace-level hourly aggregated statistics
type NamespaceGpuHourlyStats struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName string    `gorm:"type:varchar(100);not null;index:idx_cluster_ns_hour" json:"clusterName"`
	Namespace   string    `gorm:"type:varchar(253);not null;index:idx_cluster_ns_hour;index:idx_namespace" json:"namespace"`
	StatHour    time.Time `gorm:"type:timestamp;not null;index:idx_cluster_ns_hour;index:idx_namespace" json:"statHour"`

	// GPU capacity statistics
	TotalGpuCapacity int `gorm:"not null;default:0" json:"totalGpuCapacity"`

	// GPU allocation statistics
	AllocatedGpuCount float64 `gorm:"type:double precision;not null;default:0" json:"allocatedGpuCount"`
	AllocationRate    float64 `gorm:"type:double precision;not null;default:0;comment:GPU allocation rate (allocated_gpu_count / total_gpu_capacity) during this hour" json:"allocationRate"`

	// GPU utilization statistics
	AvgUtilization float64 `gorm:"type:double precision;not null;default:0" json:"avgUtilization"`
	MaxUtilization float64 `gorm:"type:double precision;not null;default:0" json:"maxUtilization"`
	MinUtilization float64 `gorm:"type:double precision;not null;default:0" json:"minUtilization"`

	// Workload statistics
	ActiveWorkloadCount int `gorm:"not null;default:0" json:"activeWorkloadCount"`

	// Timestamps
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName specifies the table name
func (NamespaceGpuHourlyStats) TableName() string {
	return "namespace_gpu_hourly_stats"
}

// LabelGpuHourlyStats is label/annotation aggregated hourly statistics
type LabelGpuHourlyStats struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName    string    `gorm:"type:varchar(100);not null;index:idx_cluster_type_key_value_hour" json:"clusterName"`
	DimensionType  string    `gorm:"type:varchar(20);not null;index:idx_cluster_type_key_value_hour;index:idx_type_key" json:"dimensionType"` // 'label' or 'annotation'
	DimensionKey   string    `gorm:"type:varchar(255);not null;index:idx_cluster_type_key_value_hour;index:idx_type_key" json:"dimensionKey"` // label/annotation key
	DimensionValue string    `gorm:"type:text;not null;index:idx_cluster_type_key_value_hour" json:"dimensionValue"`                          // label/annotation value
	StatHour       time.Time `gorm:"type:timestamp;not null;index:idx_cluster_type_key_value_hour;index:idx_type_key" json:"statHour"`

	// GPU allocation statistics
	AllocatedGpuCount float64 `gorm:"type:double precision;not null;default:0" json:"allocatedGpuCount"`

	// GPU utilization statistics
	AvgUtilization float64 `gorm:"type:double precision;not null;default:0" json:"avgUtilization"`
	MaxUtilization float64 `gorm:"type:double precision;not null;default:0" json:"maxUtilization"`
	MinUtilization float64 `gorm:"type:double precision;not null;default:0" json:"minUtilization"`

	// Workload statistics
	ActiveWorkloadCount int `gorm:"not null;default:0" json:"activeWorkloadCount"`

	// Timestamps
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName specifies the table name
func (LabelGpuHourlyStats) TableName() string {
	return "label_gpu_hourly_stats"
}

// GpuAllocationSnapshot is GPU allocation snapshot (supports multiple dimensions)
type GpuAllocationSnapshot struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName  string    `gorm:"type:varchar(100);not null;index:idx_cluster_time;index:idx_dimension" json:"clusterName"`
	SnapshotTime time.Time `gorm:"type:timestamp;not null;index:idx_cluster_time;index:idx_time;index:idx_dimension" json:"snapshotTime"`

	// Dimension information
	DimensionType  string  `gorm:"type:varchar(20);not null;index:idx_dimension" json:"dimensionType"` // 'cluster', 'namespace', 'label', 'annotation'
	DimensionKey   *string `gorm:"type:varchar(255);index:idx_dimension" json:"dimensionKey"`          // label/annotation key (nullable)
	DimensionValue *string `gorm:"type:text;index:idx_dimension" json:"dimensionValue"`                // namespace name or label/annotation value (nullable)

	// GPU capacity and allocation
	TotalGpuCapacity  int `gorm:"not null;default:0" json:"totalGpuCapacity"`
	AllocatedGpuCount int `gorm:"not null;default:0" json:"allocatedGpuCount"`

	AllocationDetails string    `gorm:"type:jsonb;not null;default:'{}'" json:"allocationDetails"` // Store details in JSON format
	CreatedAt         time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

// TableName specifies the table name
func (GpuAllocationSnapshot) TableName() string {
	return "gpu_allocation_snapshots"
}

// AllocationDetails is the JSON structure for snapshot details
type AllocationDetails struct {
	Namespaces  map[string]NamespaceAllocation  `json:"namespaces"`
	Annotations map[string]AnnotationAllocation `json:"annotations"`
	Workloads   map[string]WorkloadSnapshot     `json:"workloads"`
}

// NamespaceAllocation is namespace allocation information
type NamespaceAllocation struct {
	AllocatedGPU  int                `json:"allocatedGpu"`
	Utilization   float64            `json:"utilization"`
	WorkloadCount int                `json:"workloadCount"`
	Workloads     []WorkloadSnapshot `json:"workloads"`
}

// AnnotationAllocation is annotation allocation information
type AnnotationAllocation struct {
	AllocatedGPU  int     `json:"allocatedGpu"`
	Utilization   float64 `json:"utilization"`
	WorkloadCount int     `json:"workloadCount"`
}

// WorkloadSnapshot is workload snapshot information
type WorkloadSnapshot struct {
	UID          string  `json:"uid"`
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	Kind         string  `json:"kind"`
	AllocatedGPU int     `json:"allocatedGpu"`
	Utilization  float64 `json:"utilization"`
	ReplicaCount int     `json:"replicaCount"`
}

// ReportQuery is report query parameters
type ReportQuery struct {
	ClusterName   string    `json:"clusterName"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	Granularity   string    `json:"granularity"` // hour, day, week, month
	GroupBy       string    `json:"groupBy"`     // cluster, namespace, annotation
	AnnotationKey string    `json:"annotationKey,omitempty"`
	Namespace     string    `json:"namespace,omitempty"`
}

// ReportData is report data
type ReportData struct {
	TimeRange   TimeRange        `json:"timeRange"`
	Granularity string           `json:"granularity"`
	GroupBy     string           `json:"groupBy"`
	Data        []DimensionStats `json:"data"`
	Summary     ReportSummary    `json:"summary"`
}

// TimeRange is time range
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// DimensionStats is statistics data for a specific dimension
type DimensionStats struct {
	Dimension string      `json:"dimension"` // namespace name, annotation value, etc.
	Stats     []TimeStats `json:"stats"`
}

// TimeStats is statistics data at a specific time point
type TimeStats struct {
	Timestamp           time.Time `json:"timestamp"`
	AvgAllocationRate   float64   `json:"avgAllocationRate,omitempty"` // Only available at cluster level
	AvgUtilization      float64   `json:"avgUtilization"`
	MaxUtilization      float64   `json:"maxUtilization"`
	MinUtilization      float64   `json:"minUtilization,omitempty"`
	AllocatedGpuCount   float64   `json:"allocatedGpuCount"`
	ActiveWorkloadCount int       `json:"activeWorkloadCount,omitempty"` // namespace/annotation level
}

// ReportSummary is report summary
type ReportSummary struct {
	TotalAllocatedGpuHours float64 `json:"totalAllocatedGpuHours"` // GPU-hours
	AvgAllocationRate      float64 `json:"avgAllocationRate"`
	AvgUtilization         float64 `json:"avgUtilization"`
	MaxUtilization         float64 `json:"maxUtilization"`
	TotalWorkloads         int     `json:"totalWorkloads,omitempty"`
}

// GpuAggregationConfig is GPU aggregation configuration
type GpuAggregationConfig struct {
	Enabled bool `json:"enabled"`

	Sampling struct {
		Interval string `json:"interval"` // e.g. "5m"
		Timeout  string `json:"timeout"`
		Enabled  bool   `json:"enabled"`
	} `json:"sampling"`

	Aggregation struct {
		TriggerOffsetMinutes int    `json:"trigger_offset_minutes"`
		Timeout              string `json:"timeout"`
		Enabled              bool   `json:"enabled"`
	} `json:"aggregation"`

	Dimensions struct {
		Cluster struct {
			Enabled bool `json:"enabled"`
		} `json:"cluster"`

		Namespace struct {
			Enabled                 bool     `json:"enabled"`
			IncludeSystemNamespaces bool     `json:"include_system_namespaces"`
			ExcludeNamespaces       []string `json:"exclude_namespaces"`
		} `json:"namespace"`

		Label struct {
			Enabled        bool     `json:"enabled"`
			LabelKeys      []string `json:"label_keys"`      // Label keys to aggregate
			AnnotationKeys []string `json:"annotation_keys"` // Annotation keys to aggregate
			DefaultValue   string   `json:"default_value"`
		} `json:"label"`

		Workload struct {
			Enabled bool `json:"enabled"`
		} `json:"workload"`
	} `json:"dimensions"`

	Retention struct {
		HourlyDays      int    `json:"hourly_days"`
		DailyDays       int    `json:"daily_days"`
		MonthlyDays     int    `json:"monthly_days"`
		AutoCleanup     bool   `json:"auto_cleanup"`
		CleanupSchedule string `json:"cleanup_schedule"`
	} `json:"retention"`

	Prometheus struct {
		UtilizationQuery         string `json:"utilization_query"`
		WorkloadUtilizationQuery string `json:"workload_utilization_query"`
		QueryStep                int    `json:"query_step"`
		QueryTimeout             string `json:"query_timeout"`
	} `json:"prometheus"`

	Performance struct {
		ConcurrentQueries int    `json:"concurrent_queries"`
		BatchInsertSize   int    `json:"batch_insert_size"`
		EnableCache       bool   `json:"enable_cache"`
		CacheTTL          string `json:"cache_ttl"`
	} `json:"performance"`

	Monitoring struct {
		ExportMetrics  bool   `json:"export_metrics"`
		MetricsPrefix  string `json:"metrics_prefix"`
		VerboseLogging bool   `json:"verbose_logging"`
	} `json:"monitoring"`
}
