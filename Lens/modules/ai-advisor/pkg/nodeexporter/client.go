package nodeexporter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
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

// NewClientForNode creates a client for a specific node
func NewClientForNode(nodeName string) *Client {
	// In Kubernetes, DaemonSet pods are accessible via hostNetwork or service
	// Format: http://primus-lens-node-exporter-{node-name}:8989
	baseURL := fmt.Sprintf("http://primus-lens-node-exporter.primus-lens.svc.cluster.local:8989")
	return NewClient(DefaultConfig(baseURL))
}

// ============ Process Tree APIs ============

// ProcessTreeRequest represents process tree request
type ProcessTreeRequest struct {
	PodName          string `json:"pod_name"`
	PodNamespace     string `json:"pod_namespace"`
	PodUID           string `json:"pod_uid"`
	IncludeEnv       bool   `json:"include_env"`
	IncludeCmdline   bool   `json:"include_cmdline"`
	IncludeResources bool   `json:"include_resources"`
}

// ProcessInfo represents process information
type ProcessInfo struct {
	HostPID       int            `json:"host_pid"`
	HostPPID      int            `json:"host_ppid"`
	ContainerPID  int            `json:"container_pid,omitempty"`
	ContainerPPID int            `json:"container_ppid,omitempty"`
	Cmdline       string         `json:"cmdline"`
	Comm          string         `json:"comm"`
	Exe           string         `json:"exe,omitempty"`
	Args          []string       `json:"args,omitempty"`
	Env           []string       `json:"env,omitempty"`
	Cwd           string         `json:"cwd,omitempty"`
	State         string         `json:"state"`
	Threads       int            `json:"threads"`
	CPUTime       uint64         `json:"cpu_time,omitempty"`
	MemoryRSS     uint64         `json:"memory_rss,omitempty"`
	MemoryVirtual uint64         `json:"memory_virtual,omitempty"`
	ContainerID   string         `json:"container_id,omitempty"`
	ContainerName string         `json:"container_name,omitempty"`
	PodUID        string         `json:"pod_uid,omitempty"`
	PodName       string         `json:"pod_name,omitempty"`
	PodNamespace  string         `json:"pod_namespace,omitempty"`
	IsPython      bool           `json:"is_python"`
	IsJava        bool           `json:"is_java"`
	StartTime     int64          `json:"start_time,omitempty"`
	Children      []*ProcessInfo `json:"children,omitempty"`
}

// ContainerProcessTree represents container process tree
type ContainerProcessTree struct {
	ContainerID   string         `json:"container_id"`
	ContainerName string         `json:"container_name"`
	ImageName     string         `json:"image_name,omitempty"`
	RootProcess   *ProcessInfo   `json:"root_process"`
	AllProcesses  []*ProcessInfo `json:"all_processes"`
	ProcessCount  int            `json:"process_count"`
	PythonCount   int            `json:"python_count"`
}

// PodProcessTree represents pod process tree
type PodProcessTree struct {
	PodName        string                  `json:"pod_name"`
	PodNamespace   string                  `json:"pod_namespace"`
	PodUID         string                  `json:"pod_uid"`
	NodeName       string                  `json:"node_name,omitempty"`
	Containers     []*ContainerProcessTree `json:"containers"`
	TotalProcesses int                     `json:"total_processes"`
	TotalPython    int                     `json:"total_python"`
	CollectedAt    time.Time               `json:"collected_at"`
}

// GetPodProcessTree retrieves the process tree for a pod
func (c *Client) GetPodProcessTree(ctx context.Context, req *ProcessTreeRequest) (*PodProcessTree, error) {
	var response struct {
		Code int             `json:"code"`
		Data *PodProcessTree `json:"data"`
		Msg  string          `json:"msg,omitempty"`
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
func (c *Client) FindPythonProcesses(ctx context.Context, podUID string) ([]*ProcessInfo, error) {
	type Request struct {
		PodUID string `json:"pod_uid"`
	}

	var response struct {
		Code int            `json:"code"`
		Data []*ProcessInfo `json:"data"`
		Msg  string         `json:"msg,omitempty"`
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

// InspectRequest represents inspection request
type InspectRequest struct {
	PID     int      `json:"pid"`
	Scripts []string `json:"scripts"`
	Timeout int      `json:"timeout,omitempty"` // seconds
}

// InspectionResult represents inspection result
type InspectionResult struct {
	PID       int                    `json:"pid"`
	Success   bool                   `json:"success"`
	Script    string                 `json:"script"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  float64                `json:"duration"` // seconds
}

// InspectPythonProcess inspects a Python process with given scripts
func (c *Client) InspectPythonProcess(ctx context.Context, pid int, scripts []string, timeout int) (map[string]*InspectionResult, error) {
	req := &InspectRequest{
		PID:     pid,
		Scripts: scripts,
		Timeout: timeout,
	}

	var response struct {
		Code int                          `json:"code"`
		Data map[string]*InspectionResult `json:"data"`
		Msg  string                       `json:"msg,omitempty"`
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

// ScriptMetadata represents inspection script metadata
type ScriptMetadata struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Description  string                 `json:"description"`
	Author       string                 `json:"author,omitempty"`
	Category     string                 `json:"category,omitempty"` // universal, framework_specific
	Capabilities []string               `json:"capabilities"`
	Frameworks   []string               `json:"frameworks"` // Empty means universal
	Tags         []string               `json:"tags,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
}

// ListAvailableScripts lists all available inspection scripts
func (c *Client) ListAvailableScripts(ctx context.Context) ([]*ScriptMetadata, error) {
	var response struct {
		Code int               `json:"code"`
		Data []*ScriptMetadata `json:"data"`
		Msg  string            `json:"msg,omitempty"`
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
func (c *Client) SearchScripts(ctx context.Context, capability, framework string) ([]*ScriptMetadata, error) {
	var response struct {
		Code int               `json:"code"`
		Data []*ScriptMetadata `json:"data"`
		Msg  string            `json:"msg,omitempty"`
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
func ParseInspectionData[T any](result *InspectionResult) (*T, error) {
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
func LogInspectionResult(result *InspectionResult) {
	if result.Success {
		log.Infof("Inspection successful: script=%s, pid=%d, duration=%.2fs",
			result.Script, result.PID, result.Duration)
	} else {
		log.Warnf("Inspection failed: script=%s, pid=%d, error=%s",
			result.Script, result.PID, result.Error)
	}
}
