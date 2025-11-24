package gpu_usage_weekly_report

import (
	"encoding/json"
	"time"

	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// ReportPeriod represents the time range for a report
type ReportPeriod struct {
	StartTime time.Time
	EndTime   time.Time
}

// ReportData contains all the data for a weekly report
type ReportData struct {
	ClusterName    string
	Period         ReportPeriod
	MarkdownReport string
	ChartData      *ChartData
	Summary        *ReportSummary
}

// ChartData contains chart data from Conductor API
type ChartData struct {
	GpuUtilization []TimeSeriesPoint `json:"gpu_utilization"`
	GpuAllocation  []TimeSeriesPoint `json:"gpu_allocation"`
	NamespaceUsage []NamespaceData   `json:"namespace_usage"`
	LowUtilUsers   []UserData        `json:"low_util_users"`
}

// TimeSeriesPoint represents a single point in a time series chart
type TimeSeriesPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// NamespaceData represents GPU usage data for a namespace
type NamespaceData struct {
	Name        string  `json:"name"`
	GpuHours    float64 `json:"gpu_hours"`
	Utilization float64 `json:"utilization"`
}

// UserData represents low utilization user data
type UserData struct {
	Username       string  `json:"username"`
	Namespace      string  `json:"namespace"`
	AvgUtilization float64 `json:"avg_utilization"`
	GpuCount       int     `json:"gpu_count"`
	WastedGpuHours float64 `json:"wasted_gpu_hours"`
}

// ReportSummary contains summary statistics for the report
type ReportSummary struct {
	TotalGPUs      int     `json:"total_gpus"`
	AvgUtilization float64 `json:"avg_utilization"`
	AvgAllocation  float64 `json:"avg_allocation"`
	TotalGpuHours  float64 `json:"total_gpu_hours"`
	LowUtilCount   int     `json:"low_util_count"`
	WastedGpuDays  float64 `json:"wasted_gpu_days"`
}

// ConductorReportRequest represents the request to Conductor API
type ConductorReportRequest struct {
	Cluster              string `json:"cluster"`
	TimeRangeDays        int    `json:"time_range_days"`
	StartTime            string `json:"start_time,omitempty"`
	EndTime              string `json:"end_time,omitempty"`
	UtilizationThreshold int    `json:"utilization_threshold"`
	MinGpuCount          int    `json:"min_gpu_count"`
	TopN                 int    `json:"top_n"`
}

// ConductorReportResponse represents the response from Conductor API
type ConductorReportResponse struct {
	MarkdownReport string                 `json:"markdown_report"`
	ChartData      map[string]interface{} `json:"chart_data"`
	Summary        map[string]interface{} `json:"summary"`
}

// ToExtType converts ReportData to ExtType for database storage
func (r *ReportData) ToExtType() dbmodel.ExtType {
	data := map[string]interface{}{
		"cluster_name":    r.ClusterName,
		"markdown_report": r.MarkdownReport,
		"chart_data":      r.ChartData,
		"summary":         r.Summary,
	}

	// Convert to JSON and back to ExtType
	jsonBytes, _ := json.Marshal(data)
	var extType dbmodel.ExtType
	json.Unmarshal(jsonBytes, &extType)
	return extType
}

// GenerateMetadata creates metadata for the report
func (r *ReportData) GenerateMetadata() dbmodel.ExtType {
	metadata := map[string]interface{}{
		"cluster_name": r.ClusterName,
	}

	if r.Summary != nil {
		metadata["avg_utilization"] = r.Summary.AvgUtilization
		metadata["avg_allocation"] = r.Summary.AvgAllocation
		metadata["total_gpus"] = r.Summary.TotalGPUs
		metadata["low_util_count"] = r.Summary.LowUtilCount
		metadata["wasted_gpu_days"] = r.Summary.WastedGpuDays
	}

	jsonBytes, _ := json.Marshal(metadata)
	var extType dbmodel.ExtType
	json.Unmarshal(jsonBytes, &extType)
	return extType
}
