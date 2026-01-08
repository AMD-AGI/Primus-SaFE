// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// Global HTTP reporter instance
var globalHTTPReporter *HTTPReporter

// HTTPReporter provides HTTP-based container event reporting
type HTTPReporter struct {
	baseURL      string
	nodeName     string
	nodeIP       string
	httpClient   *http.Client
	batchEnabled bool
	batchSize    int
	batchTimeout time.Duration
	eventBuffer  []ContainerEventRequest
	bufferMutex  sync.Mutex
	stopChan     chan struct{}
}

// ContainerEventRequest represents the HTTP request for container events
type ContainerEventRequest struct {
	Type        string                 `json:"type"`
	Source      string                 `json:"source"`
	Node        string                 `json:"node"`
	ContainerID string                 `json:"container_id"`
	Data        map[string]interface{} `json:"data"`
}

// BatchContainerEventsRequest represents a batch of container events
type BatchContainerEventsRequest struct {
	Events []ContainerEventRequest `json:"events"`
}

// NewHTTPReporter creates a new HTTP reporter instance
func NewHTTPReporter(baseURL, nodeName, nodeIP string) *HTTPReporter {
	return &HTTPReporter{
		baseURL:  baseURL,
		nodeName: nodeName,
		nodeIP:   nodeIP,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Increased from 10s to 60s for batch processing
		},
		batchEnabled: true,
		batchSize:    10,
		batchTimeout: 5 * time.Second,
		eventBuffer:  make([]ContainerEventRequest, 0, 10),
		stopChan:     make(chan struct{}),
	}
}

// Start starts the background batch processing goroutine
func (r *HTTPReporter) Start() {
	if r.batchEnabled {
		go r.batchProcessor()
	}
}

// Stop stops the reporter and flushes remaining events
func (r *HTTPReporter) Stop() {
	close(r.stopChan)
	r.flush()
}

// ReportContainerEvent reports a single container event
func (r *HTTPReporter) ReportContainerEvent(ctx context.Context, container *model.Container, eventType string) error {
	// Convert container to map
	data := make(map[string]interface{})
	dataBytes, err := json.Marshal(container)
	if err != nil {
		log.Errorf("Failed to marshal container: %v", err)
		return err
	}
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		log.Errorf("Failed to unmarshal container data: %v", err)
		return err
	}

	event := ContainerEventRequest{
		Type:        eventType,
		Source:      "k8s",
		Node:        r.nodeName,
		ContainerID: container.Id,
		Data:        data,
	}

	// Use batch mode if enabled
	if r.batchEnabled {
		return r.addToBuffer(event)
	}

	// Send immediately if batch mode is disabled
	return r.sendSingleEvent(ctx, event)
}

// ReportDockerContainerEvent reports a Docker container event
func (r *HTTPReporter) ReportDockerContainerEvent(ctx context.Context, containerInfo *model.DockerContainerInfo, eventType string) error {
	// Convert container to map
	data := make(map[string]interface{})
	dataBytes, err := json.Marshal(containerInfo)
	if err != nil {
		log.Errorf("Failed to marshal docker container: %v", err)
		return err
	}
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		log.Errorf("Failed to unmarshal docker container data: %v", err)
		return err
	}

	event := ContainerEventRequest{
		Type:        eventType,
		Source:      "docker",
		Node:        r.nodeName,
		ContainerID: containerInfo.ID,
		Data:        data,
	}

	// Use batch mode if enabled
	if r.batchEnabled {
		return r.addToBuffer(event)
	}

	// Send immediately if batch mode is disabled
	return r.sendSingleEvent(ctx, event)
}

// addToBuffer adds an event to the buffer for batch processing
func (r *HTTPReporter) addToBuffer(event ContainerEventRequest) error {
	r.bufferMutex.Lock()
	defer r.bufferMutex.Unlock()

	r.eventBuffer = append(r.eventBuffer, event)

	// Flush if buffer is full
	if len(r.eventBuffer) >= r.batchSize {
		return r.flushLocked()
	}

	return nil
}

// batchProcessor runs in background to periodically flush the buffer
func (r *HTTPReporter) batchProcessor() {
	ticker := time.NewTicker(r.batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.flush()
		case <-r.stopChan:
			return
		}
	}
}

// flush flushes the event buffer
func (r *HTTPReporter) flush() error {
	r.bufferMutex.Lock()
	defer r.bufferMutex.Unlock()
	return r.flushLocked()
}

// flushLocked flushes the buffer (must be called with mutex locked)
func (r *HTTPReporter) flushLocked() error {
	if len(r.eventBuffer) == 0 {
		return nil
	}

	// Copy buffer and clear it
	events := make([]ContainerEventRequest, len(r.eventBuffer))
	copy(events, r.eventBuffer)
	r.eventBuffer = r.eventBuffer[:0]

	// Send batch
	return r.sendBatchEvents(context.Background(), events)
}

// sendSingleEvent sends a single container event via HTTP
func (r *HTTPReporter) sendSingleEvent(ctx context.Context, event ContainerEventRequest) error {
	url := fmt.Sprintf("%s/v1/container-events", r.baseURL)

	reqBody, err := json.Marshal(event)
	if err != nil {
		log.Errorf("Failed to marshal event: %v", err)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		log.Errorf("Failed to create request: %v", err)
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Name", r.nodeName)
	req.Header.Set("X-Node-IP", r.nodeIP)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		log.Errorf("Failed to send event: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Errorf("Failed to report event: status=%d, body=%s", resp.StatusCode, string(body))
		return fmt.Errorf("failed to report event: status=%d", resp.StatusCode)
	}

	log.Debugf("Successfully reported container event: container=%s, type=%s", event.ContainerID, event.Type)
	return nil
}

// sendBatchEvents sends a batch of container events via HTTP
func (r *HTTPReporter) sendBatchEvents(ctx context.Context, events []ContainerEventRequest) error {
	if len(events) == 0 {
		return nil
	}

	url := fmt.Sprintf("%s/v1/container-events/batch", r.baseURL)

	batchReq := BatchContainerEventsRequest{
		Events: events,
	}

	reqBody, err := json.Marshal(batchReq)
	if err != nil {
		log.Errorf("Failed to marshal batch events: %v", err)
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		log.Errorf("Failed to create request: %v", err)
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Name", r.nodeName)
	req.Header.Set("X-Node-IP", r.nodeIP)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		log.Errorf("Failed to send batch events: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Accept both 200 (full success) and 206 (partial success)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		log.Errorf("Failed to report batch events: status=%d, body=%s", resp.StatusCode, string(body))
		return fmt.Errorf("failed to report batch events: status=%d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusPartialContent {
		log.Warnf("Batch events partially processed: %s", string(body))
	} else {
		log.Infof("Successfully reported %d container events in batch", len(events))
	}

	return nil
}

// SetBatchConfig configures batch processing parameters
func (r *HTTPReporter) SetBatchConfig(enabled bool, size int, timeout time.Duration) {
	r.bufferMutex.Lock()
	defer r.bufferMutex.Unlock()

	r.batchEnabled = enabled
	r.batchSize = size
	r.batchTimeout = timeout

	// Resize buffer if needed
	if size > cap(r.eventBuffer) {
		newBuffer := make([]ContainerEventRequest, len(r.eventBuffer), size)
		copy(newBuffer, r.eventBuffer)
		r.eventBuffer = newBuffer
	}
}

// InitHTTP initializes the global HTTP reporter
func InitHTTP(baseURL, nodeName, nodeIP string) error {
	globalHTTPReporter = NewHTTPReporter(baseURL, nodeName, nodeIP)

	// Configure default batch settings
	globalHTTPReporter.SetBatchConfig(true, 10, 5*time.Second)

	// Start batch processor
	globalHTTPReporter.Start()

	log.Infof("HTTP reporter initialized: url=%s, node=%s", baseURL, nodeName)
	return nil
}

// ReportContainer reports a Kubernetes container event using the global reporter
func ReportContainer(ctx context.Context, container *model.Container, eventType string) error {
	if globalHTTPReporter == nil {
		return fmt.Errorf("HTTP reporter not initialized, call InitHTTP first")
	}
	return globalHTTPReporter.ReportContainerEvent(ctx, container, eventType)
}

// ReportDockerContainer reports a Docker container event using the global reporter
func ReportDockerContainer(ctx context.Context, containerInfo *model.DockerContainerInfo, eventType string) error {
	if globalHTTPReporter == nil {
		return fmt.Errorf("HTTP reporter not initialized, call InitHTTP first")
	}
	return globalHTTPReporter.ReportDockerContainerEvent(ctx, containerInfo, eventType)
}

// FlushEvents flushes any buffered events immediately
func FlushEvents() error {
	if globalHTTPReporter == nil {
		return fmt.Errorf("HTTP reporter not initialized")
	}
	return globalHTTPReporter.flush()
}

// Shutdown gracefully shuts down the HTTP reporter
func Shutdown() {
	if globalHTTPReporter != nil {
		globalHTTPReporter.Stop()
		log.Info("HTTP reporter shut down successfully")
	}
}

// SetBatchConfig configures batch processing parameters for the global reporter
func SetBatchConfig(enabled bool, size int, timeout time.Duration) {
	if globalHTTPReporter != nil {
		globalHTTPReporter.SetBatchConfig(enabled, size, timeout)
	}
}
