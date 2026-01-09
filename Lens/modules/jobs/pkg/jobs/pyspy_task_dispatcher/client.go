// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy_task_dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/go-resty/resty/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pyspyExecuteAPI  = "/v1/pyspy/execute"
	pyspyCheckAPI    = "/v1/pyspy/check"
	pyspyFileAPI     = "/v1/pyspy/file"
)

// PySpyClient wraps the core NodeExporterClient for py-spy specific operations
type PySpyClient struct {
	restyClient *resty.Client
	baseURL     string
}

// NewPySpyClientForNode creates a py-spy client for a specific node using the unified node-exporter discovery
func NewPySpyClientForNode(ctx context.Context, nodeName string, k8sClient client.Client) (*PySpyClient, error) {
	nodeExporterClient, err := clientsets.GetOrInitNodeExportersClient(ctx, nodeName, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get node-exporter client for node %s: %w", nodeName, err)
	}

	// Clone the resty client and set longer timeout for py-spy operations
	restyClient := nodeExporterClient.GetRestyClient()
	restyClient.SetTimeout(10 * time.Minute) // Long timeout for py-spy execution

	return &PySpyClient{
		restyClient: restyClient,
		baseURL:     restyClient.BaseURL,
	}, nil
}

// ExecuteResponse represents the response from node-exporter's execute API
type ExecuteResponse struct {
	Meta struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"meta"`
	Data struct {
		Success    bool   `json:"success"`
		OutputFile string `json:"output_file,omitempty"`
		FileSize   int64  `json:"file_size,omitempty"`
		Error      string `json:"error,omitempty"`
	} `json:"data"`
}

// ExecutePySpy calls node-exporter to execute py-spy
func (c *PySpyClient) ExecutePySpy(ctx context.Context, ext *model.PySpyTaskExt) (*model.PySpyExecuteResponse, error) {
	req := &model.PySpyExecuteRequest{
		TaskID:       ext.TaskID,
		PodUID:       ext.PodUID,
		HostPID:      ext.HostPID,
		ContainerPID: ext.ContainerPID,
		Duration:     ext.Duration,
		Rate:         ext.Rate,
		Format:       ext.Format,
		Native:       ext.Native,
		SubProcesses: ext.SubProcesses,
	}

	// Set timeout based on duration + buffer
	timeout := time.Duration(ext.Duration+60) * time.Second

	log.Infof("Calling node-exporter: %s%s for task %s (timeout: %v)", c.baseURL, pyspyExecuteAPI, ext.TaskID, timeout)

	var executeResp ExecuteResponse
	resp, err := c.restyClient.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&executeResp).
		Post(pyspyExecuteAPI)

	if err != nil {
		return nil, fmt.Errorf("failed to call node-exporter: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode(), resp.String())
	}

	return &model.PySpyExecuteResponse{
		Success:    executeResp.Data.Success,
		OutputFile: executeResp.Data.OutputFile,
		FileSize:   executeResp.Data.FileSize,
		Error:      executeResp.Data.Error,
	}, nil
}

// DownloadFile downloads the profiling file from node-exporter
func (c *PySpyClient) DownloadFile(ctx context.Context, taskID, filename string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/%s", pyspyFileAPI, taskID, filename)
	log.Infof("Downloading file from node-exporter: %s%s", c.baseURL, url)

	resp, err := c.restyClient.R().
		SetContext(ctx).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode(), resp.String())
	}

	content := resp.Body()
	log.Infof("Downloaded file %s/%s: %d bytes", taskID, filename, len(content))
	return content, nil
}

// DeleteFile deletes the profiling file from node-exporter (cleanup after storage)
func (c *PySpyClient) DeleteFile(ctx context.Context, taskID string) error {
	url := fmt.Sprintf("%s/%s", pyspyFileAPI, taskID)
	log.Infof("Deleting file from node-exporter: %s%s", c.baseURL, url)

	resp, err := c.restyClient.R().
		SetContext(ctx).
		Delete(url)

	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode(), resp.String())
	}

	log.Infof("Deleted file for task %s from node-exporter", taskID)
	return nil
}

// CheckCompatibility calls node-exporter to check py-spy compatibility
func (c *PySpyClient) CheckCompatibility(ctx context.Context, podUID string) (*model.PySpyCompatibility, error) {
	req := map[string]string{
		"pod_uid": podUID,
	}

	var result struct {
		Meta struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"meta"`
		Data model.PySpyCompatibility `json:"data"`
	}

	resp, err := c.restyClient.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&result).
		Post(pyspyCheckAPI)

	if err != nil {
		return nil, fmt.Errorf("failed to call node-exporter: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result.Data, nil
}

// StreamDownloadFile downloads and writes file to a writer (for large files)
func (c *PySpyClient) StreamDownloadFile(ctx context.Context, taskID, filename string, writer io.Writer) (int64, error) {
	url := fmt.Sprintf("%s/%s/%s", pyspyFileAPI, taskID, filename)
	log.Infof("Streaming download from node-exporter: %s%s", c.baseURL, url)

	resp, err := c.restyClient.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get(url)

	if err != nil {
		return 0, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.RawBody().Close()

	if resp.StatusCode() != http.StatusOK {
		body, _ := io.ReadAll(resp.RawBody())
		return 0, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode(), string(body))
	}

	n, err := io.Copy(writer, resp.RawBody())
	if err != nil {
		return n, fmt.Errorf("failed to write file content: %w", err)
	}

	log.Infof("Streamed file %s/%s: %d bytes", taskID, filename, n)
	return n, nil
}

// GetFileInfo gets file information from node-exporter
func (c *PySpyClient) GetFileInfo(ctx context.Context, taskID string) (*PySpyFileInfo, error) {
	url := fmt.Sprintf("%s/%s", pyspyFileAPI, taskID)

	var result struct {
		Meta struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"meta"`
		Data PySpyFileInfo `json:"data"`
	}

	resp, err := c.restyClient.R().
		SetContext(ctx).
		SetResult(&result).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result.Data, nil
}

// PySpyFileInfo represents file metadata from node-exporter
type PySpyFileInfo struct {
	TaskID    string    `json:"task_id"`
	FileName  string    `json:"file_name"`
	FilePath  string    `json:"file_path"`
	FileSize  int64     `json:"file_size"`
	Format    string    `json:"format"`
	CreatedAt string    `json:"created_at"`
}

// Legacy compatibility - keep the old interface for backward compatibility during transition
// These will be removed after all callers are updated

// NodeExporterClient is kept for backward compatibility
// Deprecated: Use PySpyClient instead
type NodeExporterClient struct {
	// Not used anymore
}

// NewNodeExporterClient creates a new node-exporter client
// Deprecated: Use NewPySpyClientForNode instead
func NewNodeExporterClient() *NodeExporterClient {
	return &NodeExporterClient{}
}

// Deprecated methods - these should not be used

func (c *NodeExporterClient) ExecutePySpy(ctx context.Context, nodeExporterAddr string, ext *model.PySpyTaskExt) (*model.PySpyExecuteResponse, error) {
	return nil, fmt.Errorf("deprecated: use PySpyClient.ExecutePySpy instead")
}

func (c *NodeExporterClient) DownloadFile(ctx context.Context, nodeExporterAddr, taskID, filename string) ([]byte, error) {
	return nil, fmt.Errorf("deprecated: use PySpyClient.DownloadFile instead")
}

func (c *NodeExporterClient) DeleteFile(ctx context.Context, nodeExporterAddr, taskID string) error {
	return fmt.Errorf("deprecated: use PySpyClient.DeleteFile instead")
}

func (c *NodeExporterClient) CheckCompatibility(ctx context.Context, nodeExporterAddr, podUID string) (*model.PySpyCompatibility, error) {
	return nil, fmt.Errorf("deprecated: use PySpyClient.CheckCompatibility instead")
}

// Helper function to unmarshal JSON safely
func unmarshalJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
