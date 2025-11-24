package gpu_usage_weekly_report

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// RegisterTestAPI registers test endpoints for weekly report generation
// This can be called from the main jobs service to add test/debug endpoints
func RegisterTestAPI(r *gin.RouterGroup, cfg *config.WeeklyReportConfig) {
	if cfg == nil {
		log.Warn("Weekly report config is nil, skipping test API registration")
		return
	}

	testGroup := r.Group("/api/v1/weekly-reports")
	{
		testGroup.POST("/generate", generateReportHandler(cfg))
		testGroup.GET("/list", listReportsHandler)
		testGroup.GET("/:id", getReportHandler)
		testGroup.GET("/:id/html", downloadHTMLHandler)
		testGroup.GET("/:id/pdf", downloadPDFHandler)
		testGroup.GET("/:id/json", downloadJSONHandler)
	}

	log.Info("Weekly report test API registered at /api/v1/weekly-reports")
}

// GenerateReportRequest represents the request to generate a report
type GenerateReportRequest struct {
	ClusterName   string `json:"cluster_name"`
	TimeRangeDays int    `json:"time_range_days"`
}

// GenerateReportResponse represents the response after generating a report
type GenerateReportResponse struct {
	Success  bool   `json:"success"`
	ReportID string `json:"report_id,omitempty"`
	Message  string `json:"message"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration,omitempty"`
}

// generateReportHandler handles POST /api/v1/weekly-reports/generate
func generateReportHandler(cfg *config.WeeklyReportConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := &testHandler{config: cfg}
		h.generateReport(c)
	}
}

type testHandler struct {
	config *config.WeeklyReportConfig
}

func (h *testHandler) generateReport(c *gin.Context) {
	startTime := time.Now()

	var req GenerateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, GenerateReportResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Use current cluster if not specified
	clusterName := req.ClusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	log.Infof("Test API: generating report for cluster: %s", clusterName)

	// Create job instance with config
	job := NewGpuUsageWeeklyReportJob(h.config)

	// Override time range if specified
	if req.TimeRangeDays > 0 {
		tempConfig := *h.config
		tempConfig.TimeRangeDays = req.TimeRangeDays
		job.config = &tempConfig
	}

	// Get clientsets
	cm := clientsets.GetClusterManager()
	currentCluster := cm.GetCurrentClusterClients()

	// Run the job
	ctx := context.Background()
	stats, err := job.Run(ctx, currentCluster.K8SClientSet, currentCluster.StorageClientSet)

	duration := time.Since(startTime)

	if err != nil {
		log.Errorf("Test API: report generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, GenerateReportResponse{
			Success:  false,
			Error:    err.Error(),
			Duration: duration.String(),
		})
		return
	}

	// Extract report ID from stats messages
	reportID := ""
	if stats != nil && len(stats.Messages) > 0 {
		// Parse report ID from message like "Successfully generated report: rpt_xxx"
		for _, msg := range stats.Messages {
			if len(msg) > 30 {
				reportID = msg[len(msg)-30:]
			}
		}
	}

	log.Infof("Test API: report generated successfully in %v", duration)

	c.JSON(http.StatusOK, GenerateReportResponse{
		Success:  true,
		ReportID: reportID,
		Message:  "Report generated successfully",
		Duration: duration.String(),
	})
}

// ListReportsResponse represents the response for list reports
type ListReportsResponse struct {
	Success bool             `json:"success"`
	Total   int64            `json:"total"`
	Reports []ReportListItem `json:"reports"`
	Error   string           `json:"error,omitempty"`
}

// ReportListItem represents a report in the list
type ReportListItem struct {
	ID          string    `json:"id"`
	ClusterName string    `json:"cluster_name"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	GeneratedAt time.Time `json:"generated_at"`
	Status      string    `json:"status"`
	HasHTML     bool      `json:"has_html"`
	HasPDF      bool      `json:"has_pdf"`
	HasJSON     bool      `json:"has_json"`
}

// listReportsHandler handles GET /api/v1/weekly-reports/list
func listReportsHandler(c *gin.Context) {
	clusterName := c.Query("cluster")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	reports, total, err := facade.List(context.Background(), clusterName, "", 1, 20)

	if err != nil {
		c.JSON(http.StatusInternalServerError, ListReportsResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	items := make([]ReportListItem, 0, len(reports))
	for _, r := range reports {
		items = append(items, ReportListItem{
			ID:          r.ID,
			ClusterName: r.ClusterName,
			PeriodStart: r.PeriodStart,
			PeriodEnd:   r.PeriodEnd,
			GeneratedAt: r.GeneratedAt,
			Status:      r.Status,
			HasHTML:     len(r.HTMLContent) > 0,
			HasPDF:      len(r.PdfContent) > 0,
			HasJSON:     r.JSONContent != nil,
		})
	}

	c.JSON(http.StatusOK, ListReportsResponse{
		Success: true,
		Total:   total,
		Reports: items,
	})
}

// GetReportResponse represents the response for get report
type GetReportResponse struct {
	Success bool                   `json:"success"`
	Report  map[string]interface{} `json:"report,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// getReportHandler handles GET /api/v1/weekly-reports/:id
func getReportHandler(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetByID(context.Background(), id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, GetReportResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, GetReportResponse{
			Success: false,
			Error:   "Report not found",
		})
		return
	}

	// Return metadata without binary content
	reportData := map[string]interface{}{
		"id":           report.ID,
		"cluster_name": report.ClusterName,
		"period_start": report.PeriodStart,
		"period_end":   report.PeriodEnd,
		"generated_at": report.GeneratedAt,
		"status":       report.Status,
		"has_html":     len(report.HTMLContent) > 0,
		"has_pdf":      len(report.PdfContent) > 0,
		"has_json":     report.JSONContent != nil,
		"metadata":     report.Metadata,
		"json_content": report.JSONContent,
	}

	c.JSON(http.StatusOK, GetReportResponse{
		Success: true,
		Report:  reportData,
	})
}

// downloadHTMLHandler handles GET /api/v1/weekly-reports/:id/html
func downloadHTMLHandler(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetByID(context.Background(), id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		return
	}

	if len(report.HTMLContent) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "HTML content not available"})
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%s.html", report.ID))
	c.Data(http.StatusOK, "text/html; charset=utf-8", report.HTMLContent)
}

// downloadPDFHandler handles GET /api/v1/weekly-reports/:id/pdf
func downloadPDFHandler(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetByID(context.Background(), id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		return
	}

	if len(report.PdfContent) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "PDF content not available"})
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", report.ID))
	c.Data(http.StatusOK, "application/pdf", report.PdfContent)
}

// downloadJSONHandler handles GET /api/v1/weekly-reports/:id/json
func downloadJSONHandler(c *gin.Context) {
	id := c.Param("id")

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	report, err := facade.GetByID(context.Background(), id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if report == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		return
	}

	if report.JSONContent == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "JSON content not available"})
		return
	}

	jsonBytes, err := json.MarshalIndent(report.JSONContent, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize JSON"})
		return
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", report.ID))
	c.Data(http.StatusOK, "application/json", jsonBytes)
}
