package profiler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// NodeExporterClient communicates with node-exporter to read profiler files
type NodeExporterClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewNodeExporterClient creates a new node-exporter client
func NewNodeExporterClient(baseURL string) *NodeExporterClient {
	return &NodeExporterClient{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for large files
		},
		baseURL: baseURL,
	}
}

// ReadFileRequest represents a file read request to node-exporter
type ReadFileRequest struct {
	PodUID        string `json:"pod_uid"`
	PodName       string `json:"pod_name,omitempty"`
	PodNamespace  string `json:"pod_namespace,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	Path          string `json:"path"`
	Offset        int64  `json:"offset,omitempty"`
	Length        int64  `json:"length,omitempty"`
}

// ReadFileResponse represents the response from node-exporter
type ReadFileResponse struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    *ReadFileData `json:"data"`
}

// ReadFileData represents file read data
type ReadFileData struct {
	Content   string    `json:"content"` // Base64 encoded
	FileInfo  *FileInfo `json:"file_info"`
	BytesRead int64     `json:"bytes_read"`
	EOF       bool      `json:"eof"`
	IsBinary  bool      `json:"is_binary"`
}

// FileInfo represents file metadata
type FileInfo struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

// ReadProfilerFile reads a profiler file from node-exporter
func (c *NodeExporterClient) ReadProfilerFile(
	ctx context.Context,
	podUID string,
	filePath string,
) ([]byte, error) {
	req := &ReadFileRequest{
		PodUID: podUID,
		Path:   filePath,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Call node-exporter API
	url := fmt.Sprintf("%s/api/v1/container-fs/read", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	log.Debugf("Reading profiler file from node-exporter: pod=%s, path=%s", podUID, filePath)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("node-exporter returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var readResp ReadFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&readResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if readResp.Code != 0 {
		return nil, fmt.Errorf("node-exporter error: %s", readResp.Message)
	}

	// Decode base64 content
	content, err := base64.StdEncoding.DecodeString(readResp.Data.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %w", err)
	}

	log.Infof("Successfully read profiler file: path=%s, size=%d bytes", filePath, len(content))

	return content, nil
}

// ReadProfilerFileChunked reads a large profiler file in chunks
func (c *NodeExporterClient) ReadProfilerFileChunked(
	ctx context.Context,
	podUID string,
	filePath string,
	chunkSize int64,
) ([]byte, error) {
	var fullContent []byte
	offset := int64(0)

	for {
		req := &ReadFileRequest{
			PodUID: podUID,
			Path:   filePath,
			Offset: offset,
			Length: chunkSize,
		}

		reqBody, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		url := fmt.Sprintf("%s/api/v1/container-fs/read", c.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		var readResp ReadFileResponse
		if err := json.NewDecoder(resp.Body).Decode(&readResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		if readResp.Code != 0 {
			return nil, fmt.Errorf("node-exporter error: %s", readResp.Message)
		}

		// Decode chunk
		chunk, err := base64.StdEncoding.DecodeString(readResp.Data.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to decode chunk: %w", err)
		}

		fullContent = append(fullContent, chunk...)
		offset += readResp.Data.BytesRead

		log.Debugf("Read chunk: offset=%d, bytes=%d, eof=%v", offset, readResp.Data.BytesRead, readResp.Data.EOF)

		if readResp.Data.EOF {
			break
		}
	}

	log.Infof("Successfully read profiler file in chunks: path=%s, total_size=%d bytes", filePath, len(fullContent))

	return fullContent, nil
}

// GetFileInfo gets file info from node-exporter
func (c *NodeExporterClient) GetFileInfo(
	ctx context.Context,
	podUID string,
	filePath string,
) (*FileInfo, error) {
	req := &ReadFileRequest{
		PodUID: podUID,
		Path:   filePath,
		Length: 0, // Just get info, don't read content
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/container-fs/read", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var readResp ReadFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&readResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if readResp.Code != 0 {
		return nil, fmt.Errorf("node-exporter error: %s", readResp.Message)
	}

	return readResp.Data.FileInfo, nil
}
