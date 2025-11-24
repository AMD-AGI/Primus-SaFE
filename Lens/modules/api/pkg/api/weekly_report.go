package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// ListWeeklyReportsResponse represents the response for listing weekly reports
type ListWeeklyReportsResponse struct {
	Success bool               `json:"success"`
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	Size    int                `json:"size"`
	Reports []WeeklyReportItem `json:"reports"`
	Error   string             `json:"error,omitempty"`
}

// WeeklyReportItem represents a weekly report item in the list
type WeeklyReportItem struct {
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
}

// GetWeeklyReportResponse represents the response for getting a single weekly report
type GetWeeklyReportResponse struct {
	Success bool                   `json:"success"`
	Report  map[string]interface{} `json:"report,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// ListWeeklyReports handles GET /weekly-reports
// Query parameters:
// - cluster: cluster name (optional)
// - status: report status (optional)
// - page: page number, default 1
// - size: page size, default 20, max 100
func ListWeeklyReports(c *gin.Context) {
	clusterName := c.Query("cluster")
	status := c.Query("status")

	// Parse pagination parameters
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	size, err := strconv.Atoi(c.DefaultQuery("size", "20"))
	if err != nil || size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	// Query database
	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	reports, total, err := facade.List(context.Background(), clusterName, status, page, size)

	if err != nil {
		log.Errorf("ListWeeklyReports: failed to query reports: %v", err)
		c.JSON(http.StatusInternalServerError, ListWeeklyReportsResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to query reports: %v", err),
		})
		return
	}

	// Convert to response format
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

	c.JSON(http.StatusOK, ListWeeklyReportsResponse{
		Success: true,
		Total:   total,
		Page:    page,
		Size:    size,
		Reports: items,
	})
}

// GetWeeklyReport handles GET /weekly-reports/:id
func GetWeeklyReport(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetByID(context.Background(), id)

	if err != nil {
		log.Errorf("GetWeeklyReport: failed to query report %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, GetWeeklyReportResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to query report: %v", err),
		})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, GetWeeklyReportResponse{
			Success: false,
			Error:   "Report not found",
		})
		return
	}

	// Return metadata without binary content
	reportData := map[string]interface{}{
		"id":           report.ID,
		"cluster_name": report.ClusterName,
		"period_start": report.PeriodStart.Format("2006-01-02"),
		"period_end":   report.PeriodEnd.Format("2006-01-02"),
		"generated_at": report.GeneratedAt.Format("2006-01-02 15:04:05"),
		"status":       report.Status,
		"has_html":     len(report.HTMLContent) > 0,
		"has_pdf":      len(report.PdfContent) > 0,
		"has_json":     report.JSONContent != nil,
		"metadata":     report.Metadata,
		"json_content": report.JSONContent,
	}

	c.JSON(http.StatusOK, GetWeeklyReportResponse{
		Success: true,
		Report:  reportData,
	})
}

// DownloadWeeklyReportHTML handles GET /weekly-reports/:id/html
// Download the HTML version of the report
func DownloadWeeklyReportHTML(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetByID(context.Background(), id)

	if err != nil {
		log.Errorf("DownloadWeeklyReportHTML: failed to query report %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to query report: %v", err),
		})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Report not found",
		})
		return
	}

	if len(report.HTMLContent) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "HTML content not available",
		})
		return
	}

	// Generate filename
	filename := fmt.Sprintf("gpu_usage_report_%s_%s.html",
		report.ClusterName,
		report.PeriodEnd.Format("20060102"))

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%s", filename))
	c.Data(http.StatusOK, "text/html; charset=utf-8", report.HTMLContent)
}

// DownloadWeeklyReportPDF handles GET /weekly-reports/:id/pdf
// Download the PDF version of the report
func DownloadWeeklyReportPDF(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetByID(context.Background(), id)

	if err != nil {
		log.Errorf("DownloadWeeklyReportPDF: failed to query report %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to query report: %v", err),
		})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Report not found",
		})
		return
	}

	if len(report.PdfContent) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "PDF content not available",
		})
		return
	}

	// Generate filename
	filename := fmt.Sprintf("gpu_usage_report_%s_%s.pdf",
		report.ClusterName,
		report.PeriodEnd.Format("20060102"))

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "application/pdf", report.PdfContent)
}

// DownloadWeeklyReportJSON handles GET /weekly-reports/:id/json
// Download the JSON data of the report
func DownloadWeeklyReportJSON(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetByID(context.Background(), id)

	if err != nil {
		log.Errorf("DownloadWeeklyReportJSON: failed to query report %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to query report: %v", err),
		})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Report not found",
		})
		return
	}

	if report.JSONContent == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "JSON content not available",
		})
		return
	}

	jsonBytes, err := json.MarshalIndent(report.JSONContent, "", "  ")
	if err != nil {
		log.Errorf("DownloadWeeklyReportJSON: failed to serialize JSON: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to serialize JSON",
		})
		return
	}

	// Generate filename
	filename := fmt.Sprintf("gpu_usage_report_%s_%s.json",
		report.ClusterName,
		report.PeriodEnd.Format("20060102"))

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "application/json", jsonBytes)
}

// GetLatestWeeklyReport handles GET /weekly-reports/latest
// Get the latest report for the specified cluster
func GetLatestWeeklyReport(c *gin.Context) {
	clusterName := c.Query("cluster")

	if clusterName == "" {
		c.JSON(http.StatusBadRequest, GetWeeklyReportResponse{
			Success: false,
			Error:   "Missing cluster parameter",
		})
		return
	}

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetLatestByCluster(context.Background(), clusterName)

	if err != nil {
		log.Errorf("GetLatestWeeklyReport: failed to query latest report for cluster %s: %v", clusterName, err)
		c.JSON(http.StatusInternalServerError, GetWeeklyReportResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to query latest report: %v", err),
		})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, GetWeeklyReportResponse{
			Success: false,
			Error:   "No report found for this cluster",
		})
		return
	}

	// Return metadata
	reportData := map[string]interface{}{
		"id":           report.ID,
		"cluster_name": report.ClusterName,
		"period_start": report.PeriodStart.Format("2006-01-02"),
		"period_end":   report.PeriodEnd.Format("2006-01-02"),
		"generated_at": report.GeneratedAt.Format("2006-01-02 15:04:05"),
		"status":       report.Status,
		"has_html":     len(report.HTMLContent) > 0,
		"has_pdf":      len(report.PdfContent) > 0,
		"has_json":     report.JSONContent != nil,
		"metadata":     report.Metadata,
		"json_content": report.JSONContent,
	}

	c.JSON(http.StatusOK, GetWeeklyReportResponse{
		Success: true,
		Report:  reportData,
	})
}
