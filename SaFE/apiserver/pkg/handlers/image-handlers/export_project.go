/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// ensureExportImageProject creates the project that saved workload images are pushed to,
// in the built-in Harbor, when it is missing. It is created public like the import
// project, so that workloads can pull saved images without a pull credential; a saved
// image is therefore readable by anyone who can reach the registry. A project that
// already exists is left as it is, so an administrator's choice of visibility stands.
func (h *ImageHandler) ensureExportImageProject(ctx context.Context) error {
	harborHost, endpoint, pw, err := h.GetHarborCredentials(ctx)
	if err != nil {
		return fmt.Errorf("failed to get harbor credentials: %w", err)
	}
	if harborHost == "" {
		// No built-in Harbor.
		return nil
	}
	return h.ensureProjectExists(ctx, endpoint, "admin", pw, common.ExportImageProject)
}

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
