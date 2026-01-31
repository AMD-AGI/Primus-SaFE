// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// Weekly Reports - GPU Utilization
	unified.Register(&unified.EndpointDef[WeeklyReportsListRequest, WeeklyReportsListResponse]{
		Name:        "weekly_reports",
		Description: "List GPU utilization weekly reports with pagination",
		HTTPMethod:  "GET",
		HTTPPath:    "/weekly-reports/gpu_utilization",
		MCPToolName: "lens_weekly_reports",
		Handler:     handleWeeklyReportsList,
	})

	unified.Register(&unified.EndpointDef[WeeklyReportLatestRequest, WeeklyReportDetailResponse]{
		Name:        "weekly_report_latest",
		Description: "Get the latest GPU utilization weekly report for a cluster",
		HTTPMethod:  "GET",
		HTTPPath:    "/weekly-reports/gpu_utilization/latest",
		MCPToolName: "lens_weekly_report_latest",
		Handler:     handleWeeklyReportLatest,
	})

	unified.Register(&unified.EndpointDef[WeeklyReportDetailRequest, WeeklyReportDetailResponse]{
		Name:        "weekly_report_detail",
		Description: "Get GPU utilization weekly report metadata by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/weekly-reports/gpu_utilization/:id",
		MCPToolName: "lens_weekly_report_detail",
		Handler:     handleWeeklyReportDetail,
	})

	unified.Register(&unified.EndpointDef[WeeklyReportJSONRequest, map[string]interface{}]{
		Name:        "weekly_report_json",
		Description: "Get GPU utilization weekly report JSON data by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/weekly-reports/gpu_utilization/:id/json",
		MCPToolName: "lens_weekly_report_json",
		Handler:     handleWeeklyReportJSON,
	})
}

// ======================== Request Types ========================

type WeeklyReportsListRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name filter"`
	Status  string `json:"status" form:"status" mcp:"description=Report status filter"`
	Page    int    `json:"page" form:"page" mcp:"description=Page number (default 1)"`
	Size    int    `json:"size" form:"size" mcp:"description=Page size (default 20 max 100)"`
}

type WeeklyReportLatestRequest struct {
	Cluster string `json:"cluster" form:"cluster" binding:"required" mcp:"description=Cluster name,required"`
}

type WeeklyReportDetailRequest struct {
	ID string `json:"id" form:"id" param:"id" binding:"required" mcp:"description=Report ID,required"`
}

type WeeklyReportJSONRequest struct {
	ID string `json:"id" form:"id" param:"id" binding:"required" mcp:"description=Report ID,required"`
}

// ======================== Response Types ========================

type WeeklyReportsListResponse struct {
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	Size    int                `json:"size"`
	Reports []WeeklyReportItem `json:"reports"`
}

type WeeklyReportDetailResponse struct {
	ID          string                 `json:"id"`
	ClusterName string                 `json:"cluster_name"`
	PeriodStart string                 `json:"period_start"`
	PeriodEnd   string                 `json:"period_end"`
	GeneratedAt string                 `json:"generated_at"`
	Status      string                 `json:"status"`
	HasHTML     bool                   `json:"has_html"`
	HasPDF      bool                   `json:"has_pdf"`
	HasJSON     bool                   `json:"has_json"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	JSONContent map[string]interface{} `json:"json_content,omitempty"`
}

// ======================== Helper Functions ========================

// getControlPlaneFacade returns the control plane facade for weekly reports
func getControlPlaneFacade() *cpdb.ControlPlaneFacade {
	cpClient := clientsets.GetControlPlaneClientSet()
	if cpClient == nil {
		return nil
	}
	return cpClient.Facade
}

// ======================== Handler Implementations ========================

func handleWeeklyReportsList(ctx context.Context, req *WeeklyReportsListRequest) (*WeeklyReportsListResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}

	size := req.Size
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	facade := getControlPlaneFacade()
	if facade == nil {
		return nil, errors.NewError().WithCode(errors.ServiceUnavailable).WithMessage("Control plane not available")
	}

	reports, total, err := facade.GetGpuUsageWeeklyReport().List(ctx, req.Cluster, req.Status, page, size)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to query reports", errors.CodeDatabaseError)
	}

	items := make([]WeeklyReportItem, 0, len(reports))
	for _, r := range reports {
		items = append(items, WeeklyReportItem{
			ID:          r.ID,
			ClusterName: r.ClusterName,
			PeriodStart: r.PeriodStart.Format("2006-01-02"),
			PeriodEnd:   r.PeriodEnd.Format("2006-01-02"),
			GeneratedAt: r.GeneratedAt.Format("2006-01-02 15:04:05"),
			Status:      r.Status,
			HasHTML:     len(r.HTMLContent) > 0,
			HasPDF:      len(r.PdfContent) > 0,
			HasJSON:     r.JSONContent != nil,
			Metadata:    r.Metadata,
		})
	}

	return &WeeklyReportsListResponse{
		Total:   total,
		Page:    page,
		Size:    size,
		Reports: items,
	}, nil
}

func handleWeeklyReportLatest(ctx context.Context, req *WeeklyReportLatestRequest) (*WeeklyReportDetailResponse, error) {
	if req.Cluster == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("cluster parameter is required")
	}

	facade := getControlPlaneFacade()
	if facade == nil {
		return nil, errors.NewError().WithCode(errors.ServiceUnavailable).WithMessage("Control plane not available")
	}

	report, err := facade.GetGpuUsageWeeklyReport().GetLatestByCluster(ctx, req.Cluster)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to query latest report", errors.CodeDatabaseError)
	}

	if report == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("No report found for this cluster")
	}

	return &WeeklyReportDetailResponse{
		ID:          report.ID,
		ClusterName: report.ClusterName,
		PeriodStart: report.PeriodStart.Format("2006-01-02"),
		PeriodEnd:   report.PeriodEnd.Format("2006-01-02"),
		GeneratedAt: report.GeneratedAt.Format("2006-01-02 15:04:05"),
		Status:      report.Status,
		HasHTML:     len(report.HTMLContent) > 0,
		HasPDF:      len(report.PdfContent) > 0,
		HasJSON:     report.JSONContent != nil,
		Metadata:    report.Metadata,
		JSONContent: report.JSONContent,
	}, nil
}

func handleWeeklyReportDetail(ctx context.Context, req *WeeklyReportDetailRequest) (*WeeklyReportDetailResponse, error) {
	facade := getControlPlaneFacade()
	if facade == nil {
		return nil, errors.NewError().WithCode(errors.ServiceUnavailable).WithMessage("Control plane not available")
	}

	report, err := facade.GetGpuUsageWeeklyReport().GetByID(ctx, req.ID)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to query report", errors.CodeDatabaseError)
	}

	if report == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("Report not found")
	}

	return &WeeklyReportDetailResponse{
		ID:          report.ID,
		ClusterName: report.ClusterName,
		PeriodStart: report.PeriodStart.Format("2006-01-02"),
		PeriodEnd:   report.PeriodEnd.Format("2006-01-02"),
		GeneratedAt: report.GeneratedAt.Format("2006-01-02 15:04:05"),
		Status:      report.Status,
		HasHTML:     len(report.HTMLContent) > 0,
		HasPDF:      len(report.PdfContent) > 0,
		HasJSON:     report.JSONContent != nil,
		Metadata:    report.Metadata,
		JSONContent: report.JSONContent,
	}, nil
}

func handleWeeklyReportJSON(ctx context.Context, req *WeeklyReportJSONRequest) (*map[string]interface{}, error) {
	facade := getControlPlaneFacade()
	if facade == nil {
		return nil, errors.NewError().WithCode(errors.ServiceUnavailable).WithMessage("Control plane not available")
	}

	report, err := facade.GetGpuUsageWeeklyReport().GetByID(ctx, req.ID)
	if err != nil {
		return nil, errors.WrapError(err, "Failed to query report", errors.CodeDatabaseError)
	}

	if report == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("Report not found")
	}

	if report.JSONContent == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("JSON content not available")
	}

	result := map[string]interface{}(report.JSONContent)
	return &result, nil
}
