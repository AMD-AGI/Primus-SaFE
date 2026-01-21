// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ============================================================================
// Log Alert Rule Template Endpoints
// ============================================================================

// --- List Templates ---

type ListLogAlertRuleTemplatesRequest struct {
	Category string `json:"category" mcp:"desc=Filter by category"`
	Cluster  string `json:"cluster" mcp:"desc=Cluster name (optional)"`
}

type ListLogAlertRuleTemplatesResponse struct {
	Templates []*dbmodel.LogAlertRuleTemplates `json:"templates"`
	Total     int                              `json:"total"`
}

func handleListLogAlertRuleTemplates(ctx context.Context, req *ListLogAlertRuleTemplatesRequest) (*ListLogAlertRuleTemplatesResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	templates, err := facade.ListLogAlertRuleTemplates(ctx, req.Category)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list templates", errors.CodeDatabaseError)
	}

	return &ListLogAlertRuleTemplatesResponse{
		Templates: templates,
		Total:     len(templates),
	}, nil
}

// --- Get Template ---

type GetLogAlertRuleTemplateRequest struct {
	ID      int64  `json:"id" mcp:"required,desc=Template ID"`
	Cluster string `json:"cluster" mcp:"desc=Cluster name (optional)"`
}

type GetLogAlertRuleTemplateResponse struct {
	*dbmodel.LogAlertRuleTemplates
}

func handleGetLogAlertRuleTemplate(ctx context.Context, req *GetLogAlertRuleTemplateRequest) (*GetLogAlertRuleTemplateResponse, error) {
	if req.ID <= 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid template ID")
	}

	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	template, err := facade.GetLogAlertRuleTemplateByID(ctx, req.ID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get template", errors.CodeDatabaseError)
	}

	if template == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("template not found")
	}

	return &GetLogAlertRuleTemplateResponse{LogAlertRuleTemplates: template}, nil
}

// ============================================================================
// Unified Registration
// ============================================================================

func init() {
	// List Log Alert Rule Templates
	unified.Register(&unified.EndpointDef[ListLogAlertRuleTemplatesRequest, ListLogAlertRuleTemplatesResponse]{
		HTTPPath:    "/log-alert-rule-templates",
		HTTPMethod:  "GET",
		MCPToolName: "lens_log_alert_rule_templates_list",
		Description: "List log alert rule templates with optional category filter",
		Handler:     handleListLogAlertRuleTemplates,
	})

	// Get Log Alert Rule Template
	unified.Register(&unified.EndpointDef[GetLogAlertRuleTemplateRequest, GetLogAlertRuleTemplateResponse]{
		HTTPPath:    "/log-alert-rule-templates/:id",
		HTTPMethod:  "GET",
		MCPToolName: "lens_log_alert_rule_template_detail",
		Description: "Get a specific log alert rule template by ID",
		Handler:     handleGetLogAlertRuleTemplate,
	})
}
