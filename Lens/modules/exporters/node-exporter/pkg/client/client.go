package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
	"github.com/go-resty/resty/v2"
)

// Client is the HTTP client for node-exporter service
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
		Timeout:       60 * time.Second, // Longer timeout for process inspection
		RetryCount:    2,
		RetryWaitTime: 2 * time.Second,
		Debug:         false,
	}
}

// NewClient creates a new node-exporter HTTP client
func NewClient(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig("http://primus-lens-node-exporter:8989")
	}

	restyClient := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetTimeout(cfg.Timeout).
		SetRetryCount(cfg.RetryCount).
		SetRetryWaitTime(cfg.RetryWaitTime).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")

	if cfg.Debug {
		restyClient.SetDebug(true)
	}

	return &Client{
		client:  restyClient,
		baseURL: cfg.BaseURL,
	}
}

// NewClientForNode creates a client for a specific node
func NewClientForNode(nodeName string) *Client {
	// In Kubernetes, DaemonSet pods are accessible via hostNetwork or service
	// Format: http://primus-lens-node-exporter-{node-name}:8989
	baseURL := "http://primus-lens-node-exporter.primus-lens.svc.cluster.local:8989"
	return NewClient(DefaultConfig(baseURL))
}

// GetRestyClient returns the underlying resty client for advanced usage
func (c *Client) GetRestyClient() *resty.Client {
	return c.client.Clone()
}

// ============ Process Tree APIs ============

// GetPodProcessTree retrieves the process tree for a pod
func (c *Client) GetPodProcessTree(ctx context.Context, req *types.ProcessTreeRequest) (*types.PodProcessTree, error) {
	var response struct {
		Code int                   `json:"code"`
		Data *types.PodProcessTree `json:"data"`
		Msg  string                `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&response).
		Post("/process-tree/pod")

	if err != nil {
		return nil, fmt.Errorf("failed to get pod process tree: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("process tree API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("process tree API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// FindPythonProcesses finds all Python processes in a pod
func (c *Client) FindPythonProcesses(ctx context.Context, podUID string) ([]*types.ProcessInfo, error) {
	type Request struct {
		PodUID string `json:"pod_uid"`
	}

	var response struct {
		Code int                  `json:"code"`
		Data []*types.ProcessInfo `json:"data"`
		Msg  string               `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(&Request{PodUID: podUID}).
		SetResult(&response).
		Post("/process-tree/python")

	if err != nil {
		return nil, fmt.Errorf("failed to find Python processes: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("process tree API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("process tree API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// ============ Python Inspector APIs ============

// InspectPythonProcess inspects a Python process with given scripts
func (c *Client) InspectPythonProcess(ctx context.Context, pid int, scripts []string, timeout int) (map[string]*types.InspectionResult, error) {
	req := &types.InspectRequest{
		PID:     pid,
		Scripts: scripts,
		Timeout: timeout,
	}

	var response struct {
		Code int                                `json:"code"`
		Data map[string]*types.InspectionResult `json:"data"`
		Msg  string                             `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&response).
		Post("/inspector/inspect")

	if err != nil {
		return nil, fmt.Errorf("failed to inspect Python process: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("inspector API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("inspector API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// ListAvailableScripts lists all available inspection scripts
func (c *Client) ListAvailableScripts(ctx context.Context) ([]*types.ScriptMetadata, error) {
	var response struct {
		Code int                     `json:"code"`
		Data []*types.ScriptMetadata `json:"data"`
		Msg  string                  `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&response).
		Get("/inspector/scripts")

	if err != nil {
		return nil, fmt.Errorf("failed to list scripts: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("inspector API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("inspector API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// SearchScripts searches for scripts by capability or framework
func (c *Client) SearchScripts(ctx context.Context, capability, framework string) ([]*types.ScriptMetadata, error) {
	var response struct {
		Code int                     `json:"code"`
		Data []*types.ScriptMetadata `json:"data"`
		Msg  string                  `json:"msg,omitempty"`
	}

	req := c.client.R().
		SetContext(ctx).
		SetResult(&response)

	if capability != "" {
		req.SetQueryParam("capability", capability)
	}
	if framework != "" {
		req.SetQueryParam("framework", framework)
	}

	resp, err := req.Get("/inspector/scripts/search")

	if err != nil {
		return nil, fmt.Errorf("failed to search scripts: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("inspector API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("inspector API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// ============ Helper Methods ============

// GetNodeExporterForPod finds the node-exporter instance for a given pod
// This requires knowing which node the pod is running on
func GetNodeExporterForPod(ctx context.Context, nodeName string) *Client {
	// In most deployments, node-exporter runs as DaemonSet
	// We can access it via service or directly via node name
	return NewClientForNode(nodeName)
}

// ParseInspectionData parses inspection result data into a typed structure
func ParseInspectionData[T any](result *types.InspectionResult) (*T, error) {
	if result == nil || !result.Success {
		return nil, fmt.Errorf("inspection failed: %s", result.Error)
	}

	dataBytes, err := json.Marshal(result.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inspection data: %w", err)
	}

	var typed T
	if err := json.Unmarshal(dataBytes, &typed); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inspection data: %w", err)
	}

	return &typed, nil
}

// LogInspectionResult logs inspection result for debugging
// Note: This uses a simple fmt.Printf. Callers may want to use their own logger.
func LogInspectionResult(result *types.InspectionResult) {
	if result.Success {
		fmt.Printf("Inspection successful: script=%s, pid=%d, duration=%.2fs\n",
			result.Script, result.PID, result.Duration)
	} else {
		fmt.Printf("Inspection failed: script=%s, pid=%d, error=%s\n",
			result.Script, result.PID, result.Error)
	}
}

// ============ Container Filesystem APIs ============

// ReadContainerFile reads a file from container filesystem
func (c *Client) ReadContainerFile(ctx context.Context, req *types.ContainerFileReadRequest) (*types.ContainerFileReadResponse, error) {
	var response struct {
		Code int                              `json:"code"`
		Data *types.ContainerFileReadResponse `json:"data"`
		Msg  string                           `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&response).
		Post("/container-fs/read")

	if err != nil {
		return nil, fmt.Errorf("failed to read container file: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("container fs API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("container fs API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// ListContainerDirectory lists files in a container directory
func (c *Client) ListContainerDirectory(ctx context.Context, req *types.ContainerDirectoryListRequest) (*types.ContainerDirectoryListResponse, error) {
	var response struct {
		Code int                                   `json:"code"`
		Data *types.ContainerDirectoryListResponse `json:"data"`
		Msg  string                                `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&response).
		Post("/container-fs/list")

	if err != nil {
		return nil, fmt.Errorf("failed to list container directory: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("container fs API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("container fs API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// GetContainerFileInfo gets file metadata from container
func (c *Client) GetContainerFileInfo(ctx context.Context, pid int, path string) (*types.ContainerFileInfo, error) {
	req := struct {
		PID  int    `json:"pid"`
		Path string `json:"path"`
	}{
		PID:  pid,
		Path: path,
	}

	var response struct {
		Code int                      `json:"code"`
		Data *types.ContainerFileInfo `json:"data"`
		Msg  string                   `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&response).
		Post("/container-fs/info")

	if err != nil {
		return nil, fmt.Errorf("failed to get container file info: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("container fs API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("container fs API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// GetTensorBoardLogs retrieves TensorBoard event files from container
func (c *Client) GetTensorBoardLogs(ctx context.Context, pid int, logDir string) (*types.TensorBoardLogInfo, error) {
	req := struct {
		PID    int    `json:"pid"`
		LogDir string `json:"log_dir"`
	}{
		PID:    pid,
		LogDir: logDir,
	}

	var response struct {
		Code int                       `json:"code"`
		Data *types.TensorBoardLogInfo `json:"data"`
		Msg  string                    `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&response).
		Post("/container-fs/tensorboard/logs")

	if err != nil {
		return nil, fmt.Errorf("failed to get TensorBoard logs: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("container fs API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("container fs API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// ReadTensorBoardEvent reads a TensorBoard event file
func (c *Client) ReadTensorBoardEvent(ctx context.Context, pid int, eventFile string, offset, length int64) (*types.ContainerFileReadResponse, error) {
	req := struct {
		PID       int    `json:"pid"`
		EventFile string `json:"event_file"`
		Offset    int64  `json:"offset,omitempty"`
		Length    int64  `json:"length,omitempty"`
	}{
		PID:       pid,
		EventFile: eventFile,
		Offset:    offset,
		Length:    length,
	}

	var response struct {
		Code int                              `json:"code"`
		Data *types.ContainerFileReadResponse `json:"data"`
		Msg  string                           `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&response).
		Post("/container-fs/tensorboard/event")

	if err != nil {
		return nil, fmt.Errorf("failed to read TensorBoard event: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("container fs API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("container fs API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}
