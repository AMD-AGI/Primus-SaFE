package gpu_usage_weekly_report

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/go-resty/resty/v2"
)

// ReportGenerator handles report data generation by calling Conductor API
type ReportGenerator struct {
	config *config.WeeklyReportConfig
	client *resty.Client
}

// NewReportGenerator creates a new ReportGenerator instance
func NewReportGenerator(cfg *config.WeeklyReportConfig) *ReportGenerator {
	client := resty.New()

	// Set default timeout if not configured
	timeout := 300 * time.Second
	if cfg != nil && cfg.Conductor.Timeout > 0 {
		timeout = cfg.Conductor.Timeout
	}
	client.SetTimeout(timeout)

	// Set retry configuration
	retryCount := 3
	if cfg != nil && cfg.Conductor.Retry > 0 {
		retryCount = cfg.Conductor.Retry
	}
	client.SetRetryCount(retryCount)

	return &ReportGenerator{
		config: cfg,
		client: client,
	}
}

// Generate generates report data by calling Conductor API
func (g *ReportGenerator) Generate(ctx context.Context, clusterName string, period ReportPeriod) (*ReportData, error) {
	log.Infof("ReportGenerator: generating report for cluster %s", clusterName)

	// Build API request
	req := g.buildRequest(clusterName, period)

	// Call Conductor API
	conductorURL := g.getConductorURL()
	apiEndpoint := fmt.Sprintf("%s/api/v1/cluster-report", conductorURL)

	log.Debugf("ReportGenerator: calling Conductor API at %s", apiEndpoint)

	resp, err := g.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&ConductorReportResponse{}).
		Post(apiEndpoint)

	if err != nil {
		log.Errorf("ReportGenerator: failed to call Conductor API: %v", err)
		return nil, fmt.Errorf("failed to call Conductor API: %w", err)
	}

	if !resp.IsSuccess() {
		log.Errorf("ReportGenerator: Conductor API returned error status: %d, body: %s",
			resp.StatusCode(), string(resp.Body()))
		return nil, fmt.Errorf("Conductor API returned error status: %d", resp.StatusCode())
	}

	// Parse response
	conductorResp := resp.Result().(*ConductorReportResponse)
	reportData, err := g.parseResponse(conductorResp, clusterName, period)
	if err != nil {
		log.Errorf("ReportGenerator: failed to parse Conductor API response: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	log.Infof("ReportGenerator: successfully generated report data")
	return reportData, nil
}

// buildRequest builds the request payload for Conductor API
func (g *ReportGenerator) buildRequest(clusterName string, period ReportPeriod) *ConductorReportRequest {
	req := &ConductorReportRequest{
		Cluster:              clusterName,
		TimeRangeDays:        7,
		UtilizationThreshold: 30,
		MinGpuCount:          1,
		TopN:                 20,
	}

	// Apply configuration if available
	if g.config != nil {
		if g.config.TimeRangeDays > 0 {
			req.TimeRangeDays = g.config.TimeRangeDays
		}
		if g.config.UtilizationThreshold > 0 {
			req.UtilizationThreshold = g.config.UtilizationThreshold
		}
		if g.config.MinGpuCount > 0 {
			req.MinGpuCount = g.config.MinGpuCount
		}
		if g.config.TopN > 0 {
			req.TopN = g.config.TopN
		}
	}

	// Set explicit time range
	req.StartTime = period.StartTime.Format(time.RFC3339)
	req.EndTime = period.EndTime.Format(time.RFC3339)

	return req
}

// parseResponse parses the Conductor API response into ReportData
func (g *ReportGenerator) parseResponse(resp *ConductorReportResponse, clusterName string, period ReportPeriod) (*ReportData, error) {
	// Use Report field first, fallback to MarkdownReport for backward compatibility
	markdownReport := resp.Report
	if markdownReport == "" {
		markdownReport = resp.MarkdownReport
	}

	reportData := &ReportData{
		ClusterName:    clusterName,
		Period:         period,
		MarkdownReport: markdownReport,
		Metadata:       resp.Metadata,
	}

	// Parse chart data
	if resp.ChartData != nil {
		chartData, err := g.parseChartData(resp.ChartData)
		if err != nil {
			log.Warnf("ReportGenerator: failed to parse chart data: %v", err)
			reportData.ChartData = &ChartData{}
		} else {
			reportData.ChartData = chartData
		}
	} else {
		reportData.ChartData = &ChartData{}
	}

	// Parse summary
	if resp.Summary != nil {
		summary, err := g.parseSummary(resp.Summary)
		if err != nil {
			log.Warnf("ReportGenerator: failed to parse summary: %v", err)
			reportData.Summary = &ReportSummary{}
		} else {
			reportData.Summary = summary
		}
	} else {
		reportData.Summary = &ReportSummary{}
	}

	return reportData, nil
}

// parseChartData parses chart data from the API response
func (g *ReportGenerator) parseChartData(data map[string]interface{}) (*ChartData, error) {
	chartData := &ChartData{}

	// Convert to JSON and back for easy parsing
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(jsonBytes, chartData)
	if err != nil {
		return nil, err
	}

	return chartData, nil
}

// parseSummary parses summary data from the API response
func (g *ReportGenerator) parseSummary(data map[string]interface{}) (*ReportSummary, error) {
	summary := &ReportSummary{}

	// Convert to JSON and back for easy parsing
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(jsonBytes, summary)
	if err != nil {
		return nil, err
	}

	// Handle total_gpu_count field (prioritize it over total_gpus)
	if totalGpuCount, ok := data["total_gpu_count"]; ok {
		switch v := totalGpuCount.(type) {
		case float64:
			summary.TotalGPUs = int(v)
		case int:
			summary.TotalGPUs = v
		case int64:
			summary.TotalGPUs = int(v)
		}
		log.Debugf("ReportGenerator: using total_gpu_count from response: %d", summary.TotalGPUs)
	}

	return summary, nil
}

// getConductorURL gets the Conductor API base URL from configuration
func (g *ReportGenerator) getConductorURL() string {
	if g.config != nil && g.config.Conductor.BaseURL != "" {
		return g.config.Conductor.BaseURL
	}
	// Default URL
	return "http://primus-conductor-api:8000"
}
