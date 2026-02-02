// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/tools-repository/pkg/registry"
)

// Provider interface for tool execution
type Provider interface {
	// Execute executes the tool with given arguments
	Execute(ctx context.Context, tool *registry.Tool, args json.RawMessage) (*registry.ExecuteToolResponse, error)

	// HealthCheck checks if the provider is available
	HealthCheck(ctx context.Context, tool *registry.Tool) error
}

// ProviderFactory creates providers based on type
type ProviderFactory struct {
	httpClient *http.Client
}

// NewProviderFactory creates a new provider factory
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetProvider returns the appropriate provider for a tool
func (f *ProviderFactory) GetProvider(tool *registry.Tool) (Provider, error) {
	switch tool.ProviderType {
	case registry.ProviderHTTP:
		return f.newHTTPProvider(tool)
	case registry.ProviderMCP:
		return f.newMCPProvider(tool)
	case registry.ProviderA2A:
		return f.newA2AProvider(tool)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", tool.ProviderType)
	}
}

// HTTPProvider executes tools via HTTP
type HTTPProvider struct {
	client *http.Client
	config *registry.HTTPProviderConfig
}

func (f *ProviderFactory) newHTTPProvider(tool *registry.Tool) (*HTTPProvider, error) {
	var config registry.HTTPProviderConfig
	if err := json.Unmarshal(tool.ProviderConfig, &config); err != nil {
		return nil, fmt.Errorf("invalid HTTP provider config: %w", err)
	}

	timeout := 30 * time.Second
	if config.Timeout > 0 {
		timeout = time.Duration(config.Timeout) * time.Second
	}

	return &HTTPProvider{
		client: &http.Client{Timeout: timeout},
		config: &config,
	}, nil
}

func (p *HTTPProvider) Execute(ctx context.Context, tool *registry.Tool, args json.RawMessage) (*registry.ExecuteToolResponse, error) {
	startTime := time.Now()

	method := p.config.Method
	if method == "" {
		method = "POST"
	}

	var body io.Reader
	if len(args) > 0 && method != "GET" {
		body = strings.NewReader(string(args))
	}

	req, err := http.NewRequestWithContext(ctx, method, p.config.URL, body)
	if err != nil {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      fmt.Sprintf("failed to create request: %v", err),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	for key, value := range p.config.Headers {
		req.Header.Set(key, value)
	}

	// Apply authentication
	if err := p.applyAuth(req); err != nil {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      fmt.Sprintf("authentication failed: %v", err),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      fmt.Sprintf("request failed: %v", err),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      fmt.Sprintf("failed to read response: %v", err),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	if resp.StatusCode >= 400 {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	return &registry.ExecuteToolResponse{
		Success:    true,
		Output:     respBody,
		DurationMs: int(time.Since(startTime).Milliseconds()),
	}, nil
}

func (p *HTTPProvider) applyAuth(req *http.Request) error {
	switch p.config.AuthType {
	case "bearer":
		var config struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(p.config.AuthConfig, &config); err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+config.Token)

	case "basic":
		var config struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(p.config.AuthConfig, &config); err != nil {
			return err
		}
		req.SetBasicAuth(config.Username, config.Password)

	case "api_key":
		var config struct {
			Header string `json:"header"`
			Key    string `json:"key"`
		}
		if err := json.Unmarshal(p.config.AuthConfig, &config); err != nil {
			return err
		}
		header := config.Header
		if header == "" {
			header = "X-API-Key"
		}
		req.Header.Set(header, config.Key)
	}

	return nil
}

func (p *HTTPProvider) HealthCheck(ctx context.Context, tool *registry.Tool) error {
	req, err := http.NewRequestWithContext(ctx, "HEAD", p.config.URL, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	return nil
}

// MCPProvider executes tools via MCP
type MCPProvider struct {
	config *registry.MCPProviderConfig
}

func (f *ProviderFactory) newMCPProvider(tool *registry.Tool) (*MCPProvider, error) {
	var config registry.MCPProviderConfig
	if err := json.Unmarshal(tool.ProviderConfig, &config); err != nil {
		return nil, fmt.Errorf("invalid MCP provider config: %w", err)
	}

	return &MCPProvider{config: &config}, nil
}

func (p *MCPProvider) Execute(ctx context.Context, tool *registry.Tool, args json.RawMessage) (*registry.ExecuteToolResponse, error) {
	startTime := time.Now()

	// For MCP, we need to connect to the MCP server and call the tool
	// This is a simplified implementation - full implementation would use MCP client

	if p.config.ServerURL == "" {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      "MCP server URL not configured",
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	// Call MCP server via SSE or HTTP
	url := fmt.Sprintf("%s/tools/%s/call", p.config.ServerURL, tool.Name)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(args)))
	if err != nil {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	return &registry.ExecuteToolResponse{
		Success:    resp.StatusCode < 400,
		Output:     respBody,
		Error:      "",
		DurationMs: int(time.Since(startTime).Milliseconds()),
	}, nil
}

func (p *MCPProvider) HealthCheck(ctx context.Context, tool *registry.Tool) error {
	if p.config.ServerURL == "" {
		return fmt.Errorf("MCP server URL not configured")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(p.config.ServerURL + "/health")
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

// A2AProvider executes tools via Agent-to-Agent protocol
type A2AProvider struct {
	config *registry.A2AProviderConfig
}

func (f *ProviderFactory) newA2AProvider(tool *registry.Tool) (*A2AProvider, error) {
	var config registry.A2AProviderConfig
	if err := json.Unmarshal(tool.ProviderConfig, &config); err != nil {
		return nil, fmt.Errorf("invalid A2A provider config: %w", err)
	}

	return &A2AProvider{config: &config}, nil
}

func (p *A2AProvider) Execute(ctx context.Context, tool *registry.Tool, args json.RawMessage) (*registry.ExecuteToolResponse, error) {
	startTime := time.Now()

	// A2A protocol implementation
	// This is a placeholder - full implementation would follow A2A spec

	if p.config.AgentURL == "" {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      "A2A agent URL not configured",
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	// Send task to agent
	taskPayload := map[string]interface{}{
		"task": map[string]interface{}{
			"type":       "tool_call",
			"tool_name":  tool.Name,
			"arguments":  json.RawMessage(args),
		},
	}

	payload, _ := json.Marshal(taskPayload)

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.AgentURL+"/tasks", strings.NewReader(string(payload)))
	if err != nil {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return &registry.ExecuteToolResponse{
			Success:    false,
			Error:      err.Error(),
			DurationMs: int(time.Since(startTime).Milliseconds()),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	return &registry.ExecuteToolResponse{
		Success:    resp.StatusCode < 400,
		Output:     respBody,
		DurationMs: int(time.Since(startTime).Milliseconds()),
	}, nil
}

func (p *A2AProvider) HealthCheck(ctx context.Context, tool *registry.Tool) error {
	if p.config.AgentURL == "" {
		return fmt.Errorf("A2A agent URL not configured")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(p.config.AgentURL + "/.well-known/agent.json")
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}
