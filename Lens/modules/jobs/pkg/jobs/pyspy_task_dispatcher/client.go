package pyspy_task_dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// NodeExporterClient is an HTTP client for calling node-exporter APIs
type NodeExporterClient struct {
	httpClient *http.Client
}

// NewNodeExporterClient creates a new node-exporter client
func NewNodeExporterClient() *NodeExporterClient {
	return &NodeExporterClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // Long timeout for py-spy execution
		},
	}
}

// ExecuteResponse represents the response from node-exporter's execute API
type ExecuteResponse struct {
	Code int `json:"code"`
	Data struct {
		Success    bool   `json:"success"`
		OutputFile string `json:"output_file,omitempty"`
		FileSize   int64  `json:"file_size,omitempty"`
		Error      string `json:"error,omitempty"`
	} `json:"data"`
	Message string `json:"message,omitempty"`
}

// ExecutePySpy calls node-exporter to execute py-spy
func (c *NodeExporterClient) ExecutePySpy(ctx context.Context, nodeExporterAddr string, ext *model.PySpyTaskExt) (*model.PySpyExecuteResponse, error) {
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
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := fmt.Sprintf("http://%s/v1/pyspy/execute", nodeExporterAddr)
	log.Infof("Calling node-exporter: %s for task %s", url, ext.TaskID)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call node-exporter: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var executeResp ExecuteResponse
	if err := json.Unmarshal(respBody, &executeResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &model.PySpyExecuteResponse{
		Success:    executeResp.Data.Success,
		OutputFile: executeResp.Data.OutputFile,
		FileSize:   executeResp.Data.FileSize,
		Error:      executeResp.Data.Error,
	}, nil
}

// DownloadFile downloads the profiling file from node-exporter
func (c *NodeExporterClient) DownloadFile(ctx context.Context, nodeExporterAddr, taskID, filename string) ([]byte, error) {
	url := fmt.Sprintf("http://%s/v1/pyspy/file/%s/%s", nodeExporterAddr, taskID, filename)
	log.Infof("Downloading file from node-exporter: %s", url)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode, string(body))
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	log.Infof("Downloaded file %s/%s: %d bytes", taskID, filename, len(content))
	return content, nil
}

// DeleteFile deletes the profiling file from node-exporter (cleanup after storage)
func (c *NodeExporterClient) DeleteFile(ctx context.Context, nodeExporterAddr, taskID string) error {
	url := fmt.Sprintf("http://%s/v1/pyspy/file/%s", nodeExporterAddr, taskID)
	log.Infof("Deleting file from node-exporter: %s", url)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode, string(body))
	}

	log.Infof("Deleted file for task %s from node-exporter", taskID)
	return nil
}

// CheckCompatibility calls node-exporter to check py-spy compatibility
func (c *NodeExporterClient) CheckCompatibility(ctx context.Context, nodeExporterAddr, podUID string) (*model.PySpyCompatibility, error) {
	url := fmt.Sprintf("http://%s/v1/pyspy/check", nodeExporterAddr)

	req := map[string]string{
		"pod_uid": podUID,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call node-exporter: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code int                     `json:"code"`
		Data model.PySpyCompatibility `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result.Data, nil
}

