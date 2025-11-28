package client

import (
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/go-resty/resty/v2"
)

// Client is the HTTP client for AI Advisor service
type Client struct {
	client  *resty.Client
	baseURL string
}

// Config represents client configuration
type Config struct {
	BaseURL       string
	Timeout       time.Duration
	RetryCount    int
	RetryWaitTime time.Duration
	Debug         bool
}

// DefaultConfig returns default client configuration
func DefaultConfig(baseURL string) *Config {
	return &Config{
		BaseURL:       baseURL,
		Timeout:       30 * time.Second,
		RetryCount:    3,
		RetryWaitTime: 1 * time.Second,
		Debug:         false,
	}
}

// NewClient creates a new AI Advisor HTTP client
func NewClient(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig("http://ai-advisor:8080")
	}

	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetTimeout(cfg.Timeout).
		SetRetryCount(cfg.RetryCount).
		SetRetryWaitTime(cfg.RetryWaitTime).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")

	if cfg.Debug {
		client.SetDebug(true)
	}

	return &Client{
		client:  client,
		baseURL: cfg.BaseURL,
	}
}

// NewClientWithDefaults creates a client with default configuration
func NewClientWithDefaults(baseURL string) *Client {
	return NewClient(DefaultConfig(baseURL))
}

// SetTimeout sets the request timeout
func (c *Client) SetTimeout(timeout time.Duration) *Client {
	c.client.SetTimeout(timeout)
	return c
}

// SetRetry sets retry configuration
func (c *Client) SetRetry(count int, waitTime time.Duration) *Client {
	c.client.SetRetryCount(count)
	c.client.SetRetryWaitTime(waitTime)
	return c
}

// SetDebug enables/disables debug mode
func (c *Client) SetDebug(debug bool) *Client {
	c.client.SetDebug(debug)
	return c
}

// SetAuthToken sets the authentication token
func (c *Client) SetAuthToken(token string) *Client {
	c.client.SetAuthToken(token)
	return c
}

// SetHeader sets a custom header
func (c *Client) SetHeader(key, value string) *Client {
	c.client.SetHeader(key, value)
	return c
}

// ============ Detection APIs ============

// ReportDetection reports a framework detection from any source
func (c *Client) ReportDetection(req *common.DetectionRequest) (*common.Detection, error) {
	var result common.Detection
	resp, err := c.client.R().
		SetBody(req).
		SetResult(&result).
		Post("/api/v1/detection")

	if err != nil {
		return nil, fmt.Errorf("failed to report detection: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("detection API returned error: %s", resp.String())
	}

	return &result, nil
}

// GetDetection retrieves framework detection result for a workload
func (c *Client) GetDetection(workloadUID string) (*common.Detection, error) {
	var result common.Detection
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Get("/api/v1/detection/workloads/{uid}")

	if err != nil {
		return nil, fmt.Errorf("failed to get detection: %w", err)
	}

	if resp.StatusCode() == 404 {
		return nil, nil // Not found
	}

	if resp.IsError() {
		return nil, fmt.Errorf("detection API returned error: %s", resp.String())
	}

	return &result, nil
}

// BatchGetDetection retrieves detection results for multiple workloads
func (c *Client) BatchGetDetection(workloadUIDs []string) (map[string]*common.Detection, error) {
	type BatchRequest struct {
		WorkloadUIDs []string `json:"workload_uids"`
	}

	type BatchResult struct {
		Results map[string]*common.Detection `json:"results"`
	}

	var result BatchResult
	resp, err := c.client.R().
		SetBody(&BatchRequest{WorkloadUIDs: workloadUIDs}).
		SetResult(&result).
		Post("/api/v1/detection/batch")

	if err != nil {
		return nil, fmt.Errorf("failed to batch get detections: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("detection API returned error: %s", resp.String())
	}

	return result.Results, nil
}

// UpdateDetection updates detection result (manual annotation)
func (c *Client) UpdateDetection(workloadUID string, req *common.DetectionRequest) (*common.Detection, error) {
	var result common.Detection
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetBody(req).
		SetResult(&result).
		Put("/api/v1/detection/workloads/{uid}")

	if err != nil {
		return nil, fmt.Errorf("failed to update detection: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("detection API returned error: %s", resp.String())
	}

	return &result, nil
}

// GetDetectionStats retrieves detection statistics
func (c *Client) GetDetectionStats() (*common.Statistics, error) {
	var result common.Statistics
	resp, err := c.client.R().
		SetResult(&result).
		Get("/api/v1/detection/stats")

	if err != nil {
		return nil, fmt.Errorf("failed to get detection stats: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("detection API returned error: %s", resp.String())
	}

	return &result, nil
}

// ============ Performance Analysis APIs ============

// AnalyzePerformance triggers performance analysis for a workload
func (c *Client) AnalyzePerformance(workloadUID string) (*common.PerformanceAnalysis, error) {
	type Request struct {
		WorkloadUID string `json:"workload_uid"`
	}

	var result common.PerformanceAnalysis
	resp, err := c.client.R().
		SetBody(&Request{WorkloadUID: workloadUID}).
		SetResult(&result).
		Post("/api/v1/analysis/performance")

	if err != nil {
		return nil, fmt.Errorf("failed to analyze performance: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("analysis API returned error: %s", resp.String())
	}

	return &result, nil
}

// GetPerformanceReport retrieves the latest performance report
func (c *Client) GetPerformanceReport(workloadUID string) (*common.PerformanceAnalysis, error) {
	var result common.PerformanceAnalysis
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Get("/api/v1/analysis/workloads/{uid}/performance")

	if err != nil {
		return nil, fmt.Errorf("failed to get performance report: %w", err)
	}

	if resp.StatusCode() == 404 {
		return nil, nil
	}

	if resp.IsError() {
		return nil, fmt.Errorf("analysis API returned error: %s", resp.String())
	}

	return &result, nil
}

// GetTrends retrieves performance trends
func (c *Client) GetTrends(workloadUID string) (map[string]interface{}, error) {
	var result map[string]interface{}
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Get("/api/v1/analysis/workloads/{uid}/trends")

	if err != nil {
		return nil, fmt.Errorf("failed to get trends: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("analysis API returned error: %s", resp.String())
	}

	return result, nil
}

// ============ Anomaly Detection APIs ============

// DetectAnomalies triggers anomaly detection for a workload
func (c *Client) DetectAnomalies(workloadUID string) ([]common.Anomaly, error) {
	type Request struct {
		WorkloadUID string `json:"workload_uid"`
	}

	var result []common.Anomaly
	resp, err := c.client.R().
		SetBody(&Request{WorkloadUID: workloadUID}).
		SetResult(&result).
		Post("/api/v1/anomalies/detect")

	if err != nil {
		return nil, fmt.Errorf("failed to detect anomalies: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("anomaly API returned error: %s", resp.String())
	}

	return result, nil
}

// GetAnomalies retrieves all anomalies for a workload
func (c *Client) GetAnomalies(workloadUID string) ([]common.Anomaly, error) {
	var result []common.Anomaly
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Get("/api/v1/anomalies/workloads/{uid}")

	if err != nil {
		return nil, fmt.Errorf("failed to get anomalies: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("anomaly API returned error: %s", resp.String())
	}

	return result, nil
}

// GetLatestAnomalies retrieves the latest anomalies for a workload
func (c *Client) GetLatestAnomalies(workloadUID string, limit int) ([]common.Anomaly, error) {
	var result []common.Anomaly
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetQueryParam("limit", fmt.Sprintf("%d", limit)).
		SetResult(&result).
		Get("/api/v1/anomalies/workloads/{uid}/latest")

	if err != nil {
		return nil, fmt.Errorf("failed to get latest anomalies: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("anomaly API returned error: %s", resp.String())
	}

	return result, nil
}

// ============ Recommendation APIs ============

// GetRecommendations retrieves recommendations for a workload
func (c *Client) GetRecommendations(workloadUID string) ([]common.Recommendation, error) {
	var result []common.Recommendation
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Get("/api/v1/recommendations/workloads/{uid}")

	if err != nil {
		return nil, fmt.Errorf("failed to get recommendations: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("recommendation API returned error: %s", resp.String())
	}

	return result, nil
}

// GenerateRecommendations generates new recommendations for a workload
func (c *Client) GenerateRecommendations(workloadUID string) ([]common.Recommendation, error) {
	var result []common.Recommendation
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Post("/api/v1/recommendations/workloads/{uid}/generate")

	if err != nil {
		return nil, fmt.Errorf("failed to generate recommendations: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("recommendation API returned error: %s", resp.String())
	}

	return result, nil
}

// ============ Diagnostics APIs ============

// AnalyzeWorkload triggers comprehensive diagnostic analysis
func (c *Client) AnalyzeWorkload(workloadUID string) (*common.Diagnostic, error) {
	type Request struct {
		WorkloadUID string `json:"workload_uid"`
	}

	var result common.Diagnostic
	resp, err := c.client.R().
		SetBody(&Request{WorkloadUID: workloadUID}).
		SetResult(&result).
		Post("/api/v1/diagnostics/analyze")

	if err != nil {
		return nil, fmt.Errorf("failed to analyze workload: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("diagnostics API returned error: %s", resp.String())
	}

	return &result, nil
}

// GetDiagnosticReport retrieves the latest diagnostic report
func (c *Client) GetDiagnosticReport(workloadUID string) (*common.Diagnostic, error) {
	var result common.Diagnostic
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Get("/api/v1/diagnostics/workloads/{uid}")

	if err != nil {
		return nil, fmt.Errorf("failed to get diagnostic report: %w", err)
	}

	if resp.StatusCode() == 404 {
		return nil, nil
	}

	if resp.IsError() {
		return nil, fmt.Errorf("diagnostics API returned error: %s", resp.String())
	}

	return &result, nil
}

// GetRootCauses retrieves root cause analysis results
func (c *Client) GetRootCauses(workloadUID string) ([]common.RootCause, error) {
	var result []common.RootCause
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Get("/api/v1/diagnostics/workloads/{uid}/root-causes")

	if err != nil {
		return nil, fmt.Errorf("failed to get root causes: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("diagnostics API returned error: %s", resp.String())
	}

	return result, nil
}

// ============ Model Insights APIs ============

// AnalyzeModel triggers model architecture analysis
func (c *Client) AnalyzeModel(workloadUID string, modelConfig map[string]interface{}) (*common.ModelInsight, error) {
	type Request struct {
		WorkloadUID string                 `json:"workload_uid"`
		ModelConfig map[string]interface{} `json:"model_config"`
	}

	var result common.ModelInsight
	resp, err := c.client.R().
		SetBody(&Request{
			WorkloadUID: workloadUID,
			ModelConfig: modelConfig,
		}).
		SetResult(&result).
		Post("/api/v1/insights/model")

	if err != nil {
		return nil, fmt.Errorf("failed to analyze model: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("insights API returned error: %s", resp.String())
	}

	return &result, nil
}

// GetModelInsights retrieves model insights for a workload
func (c *Client) GetModelInsights(workloadUID string) (*common.ModelInsight, error) {
	var result common.ModelInsight
	resp, err := c.client.R().
		SetPathParam("uid", workloadUID).
		SetResult(&result).
		Get("/api/v1/insights/workloads/{uid}")

	if err != nil {
		return nil, fmt.Errorf("failed to get model insights: %w", err)
	}

	if resp.StatusCode() == 404 {
		return nil, nil
	}

	if resp.IsError() {
		return nil, fmt.Errorf("insights API returned error: %s", resp.String())
	}

	return &result, nil
}

// EstimateMemory estimates memory requirements
func (c *Client) EstimateMemory(params map[string]interface{}) (*common.MemoryEstimate, error) {
	var result common.MemoryEstimate
	resp, err := c.client.R().
		SetBody(params).
		SetResult(&result).
		Post("/api/v1/insights/estimate-memory")

	if err != nil {
		return nil, fmt.Errorf("failed to estimate memory: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("insights API returned error: %s", resp.String())
	}

	return &result, nil
}

// EstimateCompute estimates compute requirements
func (c *Client) EstimateCompute(params map[string]interface{}) (*common.ComputeEstimate, error) {
	var result common.ComputeEstimate
	resp, err := c.client.R().
		SetBody(params).
		SetResult(&result).
		Post("/api/v1/insights/estimate-compute")

	if err != nil {
		return nil, fmt.Errorf("failed to estimate compute: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("insights API returned error: %s", resp.String())
	}

	return &result, nil
}

// ============ Health Check ============

// HealthCheck checks if the AI Advisor service is healthy
func (c *Client) HealthCheck() (bool, error) {
	resp, err := c.client.R().
		Get("/api/v1/health")

	if err != nil {
		return false, fmt.Errorf("health check failed: %w", err)
	}

	return resp.IsSuccess(), nil
}
