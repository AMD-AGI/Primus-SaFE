// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ============================================================================
// Alert Rule Advice Endpoints
// ============================================================================

// --- List Advices ---

type ListAlertRuleAdvicesRequest struct {
	Cluster      string `json:"cluster" mcp:"desc=Filter by cluster name"`
	RuleType     string `json:"rule_type" mcp:"desc=Filter by rule type (metric/log)"`
	Status       string `json:"status" mcp:"desc=Filter by status (pending/applied/rejected/expired)"`
	Severity     string `json:"severity" mcp:"desc=Filter by severity"`
	Category     string `json:"category" mcp:"desc=Filter by category"`
	Source       string `json:"source" mcp:"desc=Filter by source (system/ai)"`
	DateFrom     string `json:"date_from" mcp:"desc=Filter by start date (YYYY-MM-DD)"`
	DateTo       string `json:"date_to" mcp:"desc=Filter by end date (YYYY-MM-DD)"`
	Offset       int    `json:"offset" mcp:"desc=Pagination offset"`
	Limit        int    `json:"limit" mcp:"desc=Pagination limit (default: 20)"`
}

type ListAlertRuleAdvicesResponse struct {
	Advices []*dbmodel.AlertRuleAdvices `json:"advices"`
	Total   int64                       `json:"total"`
	Offset  int                         `json:"offset"`
	Limit   int                         `json:"limit"`
}

func handleListAlertRuleAdvices(ctx context.Context, req *ListAlertRuleAdvicesRequest) (*ListAlertRuleAdvicesResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	filter := &database.AlertRuleAdvicesFilter{
		Offset: req.Offset,
		Limit:  limit,
	}

	if req.Cluster != "" {
		filter.ClusterName = req.Cluster
	}
	if req.RuleType != "" {
		filter.RuleType = req.RuleType
	}
	if req.Status != "" {
		filter.Status = req.Status
	}
	if req.Category != "" {
		filter.Category = req.Category
	}

	facade := database.GetFacade().GetAlertRuleAdvice()

	advices, total, err := facade.ListAlertRuleAdvicess(ctx, filter)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list advices", errors.CodeDatabaseError)
	}

	return &ListAlertRuleAdvicesResponse{
		Advices: advices,
		Total:   total,
		Offset:  req.Offset,
		Limit:   limit,
	}, nil
}

// --- Get Advice ---

type GetAlertRuleAdviceRequest struct {
	ID int64 `json:"id" mcp:"required,desc=Advice ID"`
}

type GetAlertRuleAdviceResponse struct {
	*dbmodel.AlertRuleAdvices
}

func handleGetAlertRuleAdvice(ctx context.Context, req *GetAlertRuleAdviceRequest) (*GetAlertRuleAdviceResponse, error) {
	if req.ID <= 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid advice ID")
	}

	facade := database.GetFacade().GetAlertRuleAdvice()

	advice, err := facade.GetAlertRuleAdvicesByID(ctx, req.ID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get advice", errors.CodeDatabaseError)
	}

	if advice == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("advice not found")
	}

	return &GetAlertRuleAdviceResponse{AlertRuleAdvices: advice}, nil
}

// --- Get Advice Summary ---

type GetAdviceSummaryRequest struct {
	Cluster  string `json:"cluster" mcp:"desc=Filter by cluster name"`
	RuleType string `json:"rule_type" mcp:"desc=Filter by rule type"`
	Status   string `json:"status" mcp:"desc=Filter by status"`
}

type GetAdviceSummaryResponse struct {
	TotalCount    int64            `json:"total_count"`
	PendingCount  int64            `json:"pending_count"`
	AppliedCount  int64            `json:"applied_count"`
	RejectedCount int64            `json:"rejected_count"`
	ExpiredCount  int64            `json:"expired_count"`
	ByCategory    map[string]int64 `json:"by_category"`
	BySeverity    map[string]int64 `json:"by_severity"`
}

func handleGetAdviceSummary(ctx context.Context, req *GetAdviceSummaryRequest) (*GetAdviceSummaryResponse, error) {
	filter := &database.AlertRuleAdvicesFilter{}

	if req.Cluster != "" {
		filter.ClusterName = req.Cluster
	}
	if req.RuleType != "" {
		filter.RuleType = req.RuleType
	}
	if req.Status != "" {
		filter.Status = req.Status
	}

	facade := database.GetFacade().GetAlertRuleAdvice()

	summary, err := facade.GetAdviceSummary(ctx, filter)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get summary", errors.CodeDatabaseError)
	}

	// Convert summary to response
	resp := &GetAdviceSummaryResponse{
		ByCategory: make(map[string]int64),
		BySeverity: make(map[string]int64),
	}

	if summary != nil {
		resp.TotalCount = summary.Total
		resp.ExpiredCount = summary.ExpiredCount
		if summary.ByCategory != nil {
			resp.ByCategory = summary.ByCategory
		}
		if summary.BySeverity != nil {
			resp.BySeverity = summary.BySeverity
		}
		// Calculate pending/applied/rejected from ByStatus
		if summary.ByStatus != nil {
			resp.PendingCount = summary.ByStatus["pending"]
			resp.AppliedCount = summary.ByStatus["applied"]
			resp.RejectedCount = summary.ByStatus["rejected"]
		}
	}

	return resp, nil
}

// --- Get Advice Statistics ---

type GetAdviceStatisticsRequest struct {
	Cluster  string `json:"cluster" mcp:"desc=Cluster name"`
	DateFrom string `json:"date_from" mcp:"desc=Start date (YYYY-MM-DD)"`
	DateTo   string `json:"date_to" mcp:"desc=End date (YYYY-MM-DD)"`
}

type GetAdviceStatisticsResponse struct {
	Statistics interface{} `json:"statistics"`
	DateFrom   string      `json:"date_from"`
	DateTo     string      `json:"date_to"`
}

func handleGetAdviceStatistics(ctx context.Context, req *GetAdviceStatisticsRequest) (*GetAdviceStatisticsResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Parse time range with defaults
	now := time.Now()
	dateFrom := now.AddDate(0, 0, -30) // Default: last 30 days
	dateTo := now

	if req.DateFrom != "" {
		if t, err := time.Parse("2006-01-02", req.DateFrom); err == nil {
			dateFrom = t
		}
	}
	if req.DateTo != "" {
		if t, err := time.Parse("2006-01-02", req.DateTo); err == nil {
			dateTo = t
		}
	}

	facade := database.GetFacade().GetAlertRuleAdvice()

	stats, err := facade.GetAdviceStatistics(ctx, clusterName, dateFrom, dateTo)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get statistics", errors.CodeDatabaseError)
	}

	return &GetAdviceStatisticsResponse{
		Statistics: stats,
		DateFrom:   dateFrom.Format("2006-01-02"),
		DateTo:     dateTo.Format("2006-01-02"),
	}, nil
}

// ============================================================================
// Unified Registration
// ============================================================================

func init() {
	// List Alert Rule Advices
	unified.Register(&unified.EndpointDef[ListAlertRuleAdvicesRequest, ListAlertRuleAdvicesResponse]{
		HTTPPath:    "/alert-rule-advices",
		HTTPMethod:  "GET",
		MCPToolName: "lens_alert_rule_advices_list",
		Description: "List alert rule advices with filtering options",
		Handler:     handleListAlertRuleAdvices,
	})

	// Get Alert Rule Advice
	unified.Register(&unified.EndpointDef[GetAlertRuleAdviceRequest, GetAlertRuleAdviceResponse]{
		HTTPPath:    "/alert-rule-advices/:id",
		HTTPMethod:  "GET",
		MCPToolName: "lens_alert_rule_advice_detail",
		Description: "Get a specific alert rule advice by ID",
		Handler:     handleGetAlertRuleAdvice,
	})

	// Get Advice Summary
	unified.Register(&unified.EndpointDef[GetAdviceSummaryRequest, GetAdviceSummaryResponse]{
		HTTPPath:    "/alert-rule-advices/summary",
		HTTPMethod:  "GET",
		MCPToolName: "lens_alert_rule_advice_summary",
		Description: "Get summary of alert rule advices by status, category, and severity",
		Handler:     handleGetAdviceSummary,
	})

	// Get Advice Statistics
	unified.Register(&unified.EndpointDef[GetAdviceStatisticsRequest, GetAdviceStatisticsResponse]{
		HTTPPath:    "/alert-rule-advices/statistics",
		HTTPMethod:  "GET",
		MCPToolName: "lens_alert_rule_advice_statistics",
		Description: "Get statistics of alert rule advices over time",
		Handler:     handleGetAdviceStatistics,
	})
}
