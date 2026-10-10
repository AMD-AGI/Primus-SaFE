/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"fmt"
	"strings"
)

// ensureProjectExists creates projectName, public, when it is missing.
func (h *ImageHandler) ensureProjectExists(ctx context.Context, harborHost, username, pw, projectName string) error {
	var project struct {
		Name string `json:"name"`
	}
	err := h.harborRequest(ctx, harborHost, "/api/v2.0/projects/"+projectName, username, pw, &project)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "404") {
		return fmt.Errorf("failed to check project %s: %w", projectName, err)
	}
	return h.harborPost(ctx, harborHost, "/api/v2.0/projects", username, pw, map[string]any{
		"project_name": projectName,
		"metadata":     map[string]string{"public": "true"},
	})
}
