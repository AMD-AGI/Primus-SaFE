// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package runner

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// HTTPBackend implements Backend interface for redirect-based execution
type HTTPBackend struct {
	baseURL string
}

// HTTPBackendConfig contains configuration for HTTPBackend
type HTTPBackendConfig struct {
	BaseURL string // Base URL for the execution backend (e.g., Poco)
}

// NewHTTPBackend creates a new HTTPBackend
func NewHTTPBackend(cfg HTTPBackendConfig) *HTTPBackend {
	return &HTTPBackend{
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
	}
}

// GetRunURL returns a redirect URL for running the given tools
func (b *HTTPBackend) GetRunURL(ctx context.Context, tools []*model.Tool) (*RunURLResult, error) {
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools provided")
	}

	// Build tool names for URL parameter
	var toolNames []string
	for _, tool := range tools {
		toolNames = append(toolNames, fmt.Sprintf("%s:%s", tool.Type, tool.Name))
	}

	// Build redirect URL
	redirectURL := fmt.Sprintf("%s/session/new?tools=%s",
		b.baseURL,
		url.QueryEscape(strings.Join(toolNames, ",")),
	)

	return &RunURLResult{
		RedirectURL: redirectURL,
	}, nil
}
