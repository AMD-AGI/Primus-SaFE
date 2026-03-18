// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package runner

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// Runner handles tool execution
type Runner struct {
	backend Backend
}

// Backend defines the interface for execution backend
type Backend interface {
	// GetRunURL returns a redirect URL for running the given tools
	GetRunURL(ctx context.Context, tools []*model.Tool) (*RunURLResult, error)
}

// RunURLResult contains the redirect URL for running tools
type RunURLResult struct {
	RedirectURL string `json:"redirect_url"`
	SessionID   string `json:"session_id"`
}

// NewRunner creates a new Runner
func NewRunner(backend Backend) *Runner {
	return &Runner{
		backend: backend,
	}
}

// GetRunURL returns a redirect URL for running the given tools
func (r *Runner) GetRunURL(ctx context.Context, tools []*model.Tool) (*RunURLResult, error) {
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools provided")
	}

	// Validate tools are active
	for _, tool := range tools {
		if tool.Status != model.AppStatusActive {
			return nil, fmt.Errorf("tool %s is not active", tool.Name)
		}
	}

	// Get redirect URL from backend
	return r.backend.GetRunURL(ctx, tools)
}
