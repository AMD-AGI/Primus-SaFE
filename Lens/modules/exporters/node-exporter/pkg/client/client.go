// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package client

import (
	"context"
	"encoding/base64"
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
		Post("/v1/process-tree/pod")

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
		Post("/v1/process-tree/python")

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

// FindTensorboardFiles finds all tensorboard event files opened by processes in a pod
func (c *Client) FindTensorboardFiles(ctx context.Context, podUID, podName, podNamespace string) (*types.TensorboardFilesResponse, error) {
	type Request struct {
		PodUID       string `json:"pod_uid"`
		PodName      string `json:"pod_name,omitempty"`
		PodNamespace string `json:"pod_namespace,omitempty"`
	}

	var response struct {
		Code int                             `json:"code"`
		Data *types.TensorboardFilesResponse `json:"data"`
		Msg  string                          `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(&Request{
			PodUID:       podUID,
			PodName:      podName,
			PodNamespace: podNamespace,
		}).
		SetResult(&response).
		Post("/v1/process-tree/tensorboard")

	if err != nil {
		return nil, fmt.Errorf("failed to find tensorboard files: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("process tree API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("process tree API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// GetProcessEnvironment gets environment variables for processes in a pod
func (c *Client) GetProcessEnvironment(ctx context.Context, podUID string, pid int, filterPrefix string) (*types.ProcessEnvResponse, error) {
	type Request struct {
		PodUID       string `json:"pod_uid"`
		PID          int    `json:"pid,omitempty"`
		FilterPrefix string `json:"filter_prefix,omitempty"`
	}

	var response struct {
		Code int                       `json:"code"`
		Data *types.ProcessEnvResponse `json:"data"`
		Msg  string                    `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(&Request{
			PodUID:       podUID,
			PID:          pid,
			FilterPrefix: filterPrefix,
		}).
		SetResult(&response).
		Post("/v1/process-tree/env")

	if err != nil {
		return nil, fmt.Errorf("failed to get process environment: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("process tree API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("process tree API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// GetProcessArguments gets command line arguments for processes in a pod
func (c *Client) GetProcessArguments(ctx context.Context, podUID string, pid int) (*types.ProcessArgsResponse, error) {
	type Request struct {
		PodUID string `json:"pod_uid"`
		PID    int    `json:"pid,omitempty"`
	}

	var response struct {
		Code int                        `json:"code"`
		Data *types.ProcessArgsResponse `json:"data"`
		Msg  string                     `json:"msg,omitempty"`
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetBody(&Request{
			PodUID: podUID,
			PID:    pid,
		}).
		SetResult(&response).
		Post("/v1/process-tree/args")

	if err != nil {
		return nil, fmt.Errorf("failed to get process arguments: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("process tree API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("process tree API returned code %d: %s", response.Code, response.Msg)
	}

	return response.Data, nil
}

// ============ Container Filesystem APIs ============

// ReadContainerFile reads a file from container filesystem
// The Content field in the response is automatically decoded from base64
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
		Post("/v1/container-fs/read")

	if err != nil {
		return nil, fmt.Errorf("failed to read container file: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("container fs API error: %s", resp.String())
	}

	if response.Code != 0 {
		return nil, fmt.Errorf("container fs API returned code %d: %s", response.Code, response.Msg)
	}

	// Decode base64 content automatically
	// The server always sends base64-encoded data to prevent corruption
	if response.Data != nil && response.Data.Content != "" {
		decoded, err := base64.StdEncoding.DecodeString(response.Data.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 content: %w", err)
		}
		response.Data.Content = string(decoded)
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
		Post("/v1/container-fs/list")

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
func (c *Client) GetContainerFileInfo(ctx context.Context, podUID, podName, podNamespace, containerName, path string) (*types.ContainerFileInfo, error) {
	req := struct {
		PodUID        string `json:"pod_uid,omitempty"`
		PodName       string `json:"pod_name,omitempty"`
		PodNamespace  string `json:"pod_namespace,omitempty"`
		ContainerName string `json:"container_name,omitempty"`
		Path          string `json:"path"`
	}{
		PodUID:        podUID,
		PodName:       podName,
		PodNamespace:  podNamespace,
		ContainerName: containerName,
		Path:          path,
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
		Post("/v1/container-fs/info")

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
func (c *Client) GetTensorBoardLogs(ctx context.Context, podUID, podName, podNamespace, containerName, logDir string) (*types.TensorBoardLogInfo, error) {
	req := struct {
		PodUID        string `json:"pod_uid,omitempty"`
		PodName       string `json:"pod_name,omitempty"`
		PodNamespace  string `json:"pod_namespace,omitempty"`
		ContainerName string `json:"container_name,omitempty"`
		LogDir        string `json:"log_dir"`
	}{
		PodUID:        podUID,
		PodName:       podName,
		PodNamespace:  podNamespace,
		ContainerName: containerName,
		LogDir:        logDir,
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
		Post("/v1/container-fs/tensorboard/logs")

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
func (c *Client) ReadTensorBoardEvent(ctx context.Context, podUID, podName, podNamespace, containerName, eventFile string, offset, length int64) (*types.ContainerFileReadResponse, error) {
	req := struct {
		PodUID        string `json:"pod_uid,omitempty"`
		PodName       string `json:"pod_name,omitempty"`
		PodNamespace  string `json:"pod_namespace,omitempty"`
		ContainerName string `json:"container_name,omitempty"`
		EventFile     string `json:"event_file"`
		Offset        int64  `json:"offset,omitempty"`
		Length        int64  `json:"length,omitempty"`
	}{
		PodUID:        podUID,
		PodName:       podName,
		PodNamespace:  podNamespace,
		ContainerName: containerName,
		EventFile:     eventFile,
		Offset:        offset,
		Length:        length,
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
		Post("/v1/container-fs/tensorboard/event")

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
