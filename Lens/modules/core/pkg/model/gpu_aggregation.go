package model

import "time"

// ClusterGpuHourlyStats 集群级别小时聚合统计
type ClusterGpuHourlyStats struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName string    `gorm:"type:varchar(100);not null;index:idx_cluster_hour" json:"clusterName"`
	StatHour    time.Time `gorm:"type:timestamp;not null;index:idx_cluster_hour;index:idx_hour" json:"statHour"`

	// GPU分配统计
	TotalGpuCapacity  int     `gorm:"not null;default:0" json:"totalGpuCapacity"`
	AllocatedGpuCount float64 `gorm:"type:double precision;not null;default:0" json:"allocatedGpuCount"`
	AllocationRate    float64 `gorm:"type:double precision;not null;default:0" json:"allocationRate"`

	// GPU使用率统计
	AvgUtilization float64 `gorm:"type:double precision;not null;default:0" json:"avgUtilization"`
	MaxUtilization float64 `gorm:"type:double precision;not null;default:0" json:"maxUtilization"`
	MinUtilization float64 `gorm:"type:double precision;not null;default:0" json:"minUtilization"`
	P50Utilization float64 `gorm:"type:double precision;not null;default:0" json:"p50Utilization"`
	P95Utilization float64 `gorm:"type:double precision;not null;default:0" json:"p95Utilization"`

	// 采样统计
	SampleCount int `gorm:"not null;default:0" json:"sampleCount"`

	// 时间戳
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName 指定表名
func (ClusterGpuHourlyStats) TableName() string {
	return "cluster_gpu_hourly_stats"
}

// NamespaceGpuHourlyStats Namespace级别小时聚合统计
type NamespaceGpuHourlyStats struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName string    `gorm:"type:varchar(100);not null;index:idx_cluster_ns_hour" json:"clusterName"`
	Namespace   string    `gorm:"type:varchar(253);not null;index:idx_cluster_ns_hour;index:idx_namespace" json:"namespace"`
	StatHour    time.Time `gorm:"type:timestamp;not null;index:idx_cluster_ns_hour;index:idx_namespace" json:"statHour"`

	// GPU容量统计
	TotalGpuCapacity int `gorm:"not null;default:0" json:"totalGpuCapacity"`

	// GPU分配统计
	AllocatedGpuCount float64 `gorm:"type:double precision;not null;default:0" json:"allocatedGpuCount"`

	// GPU使用率统计
	AvgUtilization float64 `gorm:"type:double precision;not null;default:0" json:"avgUtilization"`
	MaxUtilization float64 `gorm:"type:double precision;not null;default:0" json:"maxUtilization"`
	MinUtilization float64 `gorm:"type:double precision;not null;default:0" json:"minUtilization"`

	// Workload统计
	ActiveWorkloadCount int `gorm:"not null;default:0" json:"activeWorkloadCount"`

	// 时间戳
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName 指定表名
func (NamespaceGpuHourlyStats) TableName() string {
	return "namespace_gpu_hourly_stats"
}

// LabelGpuHourlyStats Label/Annotation聚合小时统计
type LabelGpuHourlyStats struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName    string    `gorm:"type:varchar(100);not null;index:idx_cluster_type_key_value_hour" json:"clusterName"`
	DimensionType  string    `gorm:"type:varchar(20);not null;index:idx_cluster_type_key_value_hour;index:idx_type_key" json:"dimensionType"` // 'label' 或 'annotation'
	DimensionKey   string    `gorm:"type:varchar(255);not null;index:idx_cluster_type_key_value_hour;index:idx_type_key" json:"dimensionKey"` // label/annotation的key
	DimensionValue string    `gorm:"type:text;not null;index:idx_cluster_type_key_value_hour" json:"dimensionValue"`                          // label/annotation的value
	StatHour       time.Time `gorm:"type:timestamp;not null;index:idx_cluster_type_key_value_hour;index:idx_type_key" json:"statHour"`

	// GPU分配统计
	AllocatedGpuCount float64 `gorm:"type:double precision;not null;default:0" json:"allocatedGpuCount"`

	// GPU使用率统计
	AvgUtilization float64 `gorm:"type:double precision;not null;default:0" json:"avgUtilization"`
	MaxUtilization float64 `gorm:"type:double precision;not null;default:0" json:"maxUtilization"`
	MinUtilization float64 `gorm:"type:double precision;not null;default:0" json:"minUtilization"`

	// Workload统计
	ActiveWorkloadCount int `gorm:"not null;default:0" json:"activeWorkloadCount"`

	// 时间戳
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName 指定表名
func (LabelGpuHourlyStats) TableName() string {
	return "label_gpu_hourly_stats"
}

// GpuAllocationSnapshot GPU分配快照 (支持多维度)
type GpuAllocationSnapshot struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName  string    `gorm:"type:varchar(100);not null;index:idx_cluster_time;index:idx_dimension" json:"clusterName"`
	SnapshotTime time.Time `gorm:"type:timestamp;not null;index:idx_cluster_time;index:idx_time;index:idx_dimension" json:"snapshotTime"`

	// 维度信息
	DimensionType  string  `gorm:"type:varchar(20);not null;index:idx_dimension" json:"dimensionType"` // 'cluster', 'namespace', 'label', 'annotation'
	DimensionKey   *string `gorm:"type:varchar(255);index:idx_dimension" json:"dimensionKey"`          // label/annotation的key (可为NULL)
	DimensionValue *string `gorm:"type:text;index:idx_dimension" json:"dimensionValue"`                // namespace名称或label/annotation的value (可为NULL)

	// GPU容量和分配
	TotalGpuCapacity  int `gorm:"not null;default:0" json:"totalGpuCapacity"`
	AllocatedGpuCount int `gorm:"not null;default:0" json:"allocatedGpuCount"`

	AllocationDetails string    `gorm:"type:jsonb;not null;default:'{}'" json:"allocationDetails"` // JSON格式存储详细信息
	CreatedAt         time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

// TableName 指定表名
func (GpuAllocationSnapshot) TableName() string {
	return "gpu_allocation_snapshots"
}

// AllocationDetails 快照详细信息的JSON结构
type AllocationDetails struct {
	Namespaces  map[string]NamespaceAllocation  `json:"namespaces"`
	Annotations map[string]AnnotationAllocation `json:"annotations"`
}

// NamespaceAllocation Namespace分配信息
type NamespaceAllocation struct {
	AllocatedGPU  int                `json:"allocatedGpu"`
	Utilization   float64            `json:"utilization"`
	WorkloadCount int                `json:"workloadCount"`
	Workloads     []WorkloadSnapshot `json:"workloads"`
}

// AnnotationAllocation Annotation分配信息
type AnnotationAllocation struct {
	AllocatedGPU  int     `json:"allocatedGpu"`
	Utilization   float64 `json:"utilization"`
	WorkloadCount int     `json:"workloadCount"`
}

// WorkloadSnapshot Workload快照信息
type WorkloadSnapshot struct {
	UID          string  `json:"uid"`
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	Kind         string  `json:"kind"`
	AllocatedGPU int     `json:"allocatedGpu"`
	Utilization  float64 `json:"utilization"`
}

// ReportQuery 报表查询参数
type ReportQuery struct {
	ClusterName   string    `json:"clusterName"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	Granularity   string    `json:"granularity"` // hour, day, week, month
	GroupBy       string    `json:"groupBy"`     // cluster, namespace, annotation
	AnnotationKey string    `json:"annotationKey,omitempty"`
	Namespace     string    `json:"namespace,omitempty"`
}

// ReportData 报表数据
type ReportData struct {
	TimeRange   TimeRange        `json:"timeRange"`
	Granularity string           `json:"granularity"`
	GroupBy     string           `json:"groupBy"`
	Data        []DimensionStats `json:"data"`
	Summary     ReportSummary    `json:"summary"`
}

// TimeRange 时间范围
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// DimensionStats 某个维度的统计数据
type DimensionStats struct {
	Dimension string      `json:"dimension"` // namespace名称、annotation值等
	Stats     []TimeStats `json:"stats"`
}

// TimeStats 某个时间点的统计数据
type TimeStats struct {
	Timestamp           time.Time `json:"timestamp"`
	AvgAllocationRate   float64   `json:"avgAllocationRate,omitempty"` // 仅集群级别有
	AvgUtilization      float64   `json:"avgUtilization"`
	MaxUtilization      float64   `json:"maxUtilization"`
	MinUtilization      float64   `json:"minUtilization,omitempty"`
	AllocatedGpuCount   float64   `json:"allocatedGpuCount"`
	ActiveWorkloadCount int       `json:"activeWorkloadCount,omitempty"` // namespace/annotation级别
}

// ReportSummary 报表汇总
type ReportSummary struct {
	TotalAllocatedGpuHours float64 `json:"totalAllocatedGpuHours"` // GPU-小时
	AvgAllocationRate      float64 `json:"avgAllocationRate"`
	AvgUtilization         float64 `json:"avgUtilization"`
	MaxUtilization         float64 `json:"maxUtilization"`
	TotalWorkloads         int     `json:"totalWorkloads,omitempty"`
}

// GpuAggregationConfig GPU聚合配置
type GpuAggregationConfig struct {
	Enabled bool `json:"enabled"`

	Sampling struct {
		Interval string `json:"interval"` // 如 "5m"
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
			LabelKeys      []string `json:"label_keys"`      // 要聚合的label keys
			AnnotationKeys []string `json:"annotation_keys"` // 要聚合的annotation keys
			DefaultValue   string   `json:"default_value"`
		} `json:"label"`
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
