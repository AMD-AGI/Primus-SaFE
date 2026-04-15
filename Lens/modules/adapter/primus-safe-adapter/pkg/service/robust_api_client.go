// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// LinkParentRequest is the payload for POST /api/v1/workloads/link-parent.
type LinkParentRequest struct {
	ParentInstanceID  string `json:"parent_instance_id"`
	ParentName        string `json:"parent_name"`
	ParentNamespace   string `json:"parent_namespace"`
	ParentState       string `json:"parent_state"`
	ParentGPUAllocated int32 `json:"parent_gpu_allocated"`
	ChildName         string `json:"child_name"`
	ChildNamespace    string `json:"child_namespace"`
}

// LinkParentResponse is the response from POST /api/v1/workloads/link-parent.
type LinkParentResponse struct {
	Linked           bool   `json:"linked"`
	ChildInstanceID  string `json:"child_instance_id"`
	ParentInstanceID string `json:"parent_instance_id"`
}

// RobustAPIClient calls the robust-api service.
type RobustAPIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRobustAPIClient creates a client for the given base URL.
// baseURL should NOT have a trailing slash, e.g. "http://host:8085".
func NewRobustAPIClient(baseURL string) *RobustAPIClient {
	return &RobustAPIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// LinkParent calls POST /api/v1/workloads/link-parent.
func (c *RobustAPIClient) LinkParent(ctx context.Context, req *LinkParentRequest) (*LinkParentResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/api/v1/workloads/link-parent"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s returned %d", url, resp.StatusCode)
	}

	var result LinkParentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Global singleton — initialised once from ROBUST_API_URL env var.
// ---------------------------------------------------------------------------

var (
	globalRobustClient *RobustAPIClient
	robustClientOnce   sync.Once
)

// InitRobustAPIClient initialises the global robust-api client from
// the ROBUST_API_URL environment variable. If the variable is unset the
// client stays nil and all LinkParent calls are silently skipped.
func InitRobustAPIClient() {
	robustClientOnce.Do(func() {
		url := os.Getenv("ROBUST_API_URL")
		if url == "" {
			log.Info("ROBUST_API_URL not set, robust-api link-parent integration disabled")
			return
		}
		globalRobustClient = NewRobustAPIClient(url)
		log.Infof("Robust API client initialized: %s", url)
	})
}

// GetRobustAPIClient returns the global client (may be nil).
func GetRobustAPIClient() *RobustAPIClient {
	return globalRobustClient
}
