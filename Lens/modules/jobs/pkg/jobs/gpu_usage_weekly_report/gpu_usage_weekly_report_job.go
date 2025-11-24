package gpu_usage_weekly_report

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"github.com/google/uuid"
)

type GpuUsageWeeklyReportJob struct {
	config *config.WeeklyReportConfig
}

// NewGpuUsageWeeklyReportJob creates a new GpuUsageWeeklyReportJob instance
func NewGpuUsageWeeklyReportJob(cfg *config.WeeklyReportConfig) *GpuUsageWeeklyReportJob {
	return &GpuUsageWeeklyReportJob{
		config: cfg,
	}
}

// Run executes the GPU usage weekly report generation job
func (j *GpuUsageWeeklyReportJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	startTime := time.Now()
	stats := common.NewExecutionStats()

	// Check if weekly report is enabled
	if j.config == nil || !j.config.Enabled {
		log.Debug("GpuUsageWeeklyReportJob: weekly report is disabled, skipping")
		stats.AddMessage("Weekly report is disabled")
		return stats, nil
	}

	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	log.Infof("GpuUsageWeeklyReportJob: starting weekly report generation for cluster: %s", clusterName)

	// Generate report ID
	reportID := fmt.Sprintf("rpt_%d_%s_%s", time.Now().Unix(), clusterName, uuid.New().String()[:8])

	// Calculate report period
	period := j.calculatePeriod()
	log.Infof("GpuUsageWeeklyReportJob: report period: %s to %s",
		period.StartTime.Format(time.RFC3339),
		period.EndTime.Format(time.RFC3339))

	// Create initial report record
	report := &dbmodel.GpuUsageWeeklyReports{
		ID:          reportID,
		ClusterName: clusterName,
		PeriodStart: period.StartTime,
		PeriodEnd:   period.EndTime,
		GeneratedAt: time.Now(),
		Status:      "pending",
	}

	facade := database.GetFacade().GetGpuUsageWeeklyReport()
	err := facade.Create(ctx, report)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportJob: failed to create report record: %v", err)
		return stats, err
	}
	stats.ItemsCreated = 1

	// Step 1: Call Conductor API to get report data
	log.Info("GpuUsageWeeklyReportJob: calling Conductor API to fetch report data")
	generator := NewReportGenerator(j.config)
	reportData, err := generator.Generate(ctx, clusterName, period)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportJob: failed to generate report data: %v", err)
		report.Status = "failed"
		report.ErrorMessage = fmt.Sprintf("Failed to generate report data: %v", err)
		facade.Update(ctx, report)
		return stats, err
	}
	log.Infof("GpuUsageWeeklyReportJob: successfully fetched report data from Conductor API")

	// Step 1.5: Get average GPU count from cluster_gpu_hourly_stats table
	avgGpuCount, err := j.getAverageGpuCountFromDB(ctx, clusterName, period)
	if err != nil {
		log.Warnf("GpuUsageWeeklyReportJob: failed to get average GPU count from DB: %v", err)
		// Non-critical error, continue with existing data
	} else if avgGpuCount > 0 {
		// Update TotalGPUs with database average if available
		if reportData.Summary == nil {
			reportData.Summary = &ReportSummary{}
		}
		// Use database value as the source of truth for total GPU count
		reportData.Summary.TotalGPUs = avgGpuCount
		log.Infof("GpuUsageWeeklyReportJob: updated total GPU count from database: %d", avgGpuCount)
	}

	// Step 2: Render report in multiple formats
	log.Info("GpuUsageWeeklyReportJob: rendering report in multiple formats")
	renderer := NewReportRenderer(j.config)

	// Render HTML
	htmlContent, err := renderer.RenderHTML(ctx, reportData)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportJob: failed to render HTML: %v", err)
		report.Status = "failed"
		report.ErrorMessage = fmt.Sprintf("Failed to render HTML: %v", err)
		facade.Update(ctx, report)
		return stats, err
	}
	report.HTMLContent = htmlContent
	log.Info("GpuUsageWeeklyReportJob: HTML rendering completed")

	// Render PDF if enabled
	if j.shouldRenderPDF() {
		pdfContent, err := renderer.RenderPDF(ctx, htmlContent)
		if err != nil {
			log.Errorf("GpuUsageWeeklyReportJob: failed to render PDF: %v", err)
			// PDF rendering failure is not critical, continue
		} else {
			report.PdfContent = pdfContent
			log.Info("GpuUsageWeeklyReportJob: PDF rendering completed")
		}
	}

	// Step 3: Store JSON content and metadata
	report.JSONContent = reportData.ToExtType()
	report.Metadata = reportData.GenerateMetadata()
	report.Status = "generated"

	// Step 4: Save report to database
	err = facade.Update(ctx, report)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportJob: failed to save report: %v", err)
		return stats, err
	}
	stats.ItemsUpdated = 1

	duration := time.Since(startTime)
	log.Infof("GpuUsageWeeklyReportJob: report generation completed in %v, report ID: %s", duration, reportID)
	stats.RecordsProcessed = 1
	stats.AddMessage(fmt.Sprintf("Successfully generated report: %s", reportID))

	return stats, nil
}

// Schedule returns the cron schedule for this job
func (j *GpuUsageWeeklyReportJob) Schedule() string {
	if j.config != nil && j.config.Cron != "" {
		return j.config.Cron
	}
	// Default: every Monday at 9:00 AM
	return "0 9 * * 1"
}

// calculatePeriod calculates the report period based on configuration
func (j *GpuUsageWeeklyReportJob) calculatePeriod() ReportPeriod {
	endTime := time.Now()
	days := 7 // default to 7 days

	if j.config != nil && j.config.TimeRangeDays > 0 {
		days = j.config.TimeRangeDays
	}

	startTime := endTime.AddDate(0, 0, -days)

	return ReportPeriod{
		StartTime: startTime,
		EndTime:   endTime,
	}
}

// shouldRenderPDF checks if PDF rendering is enabled in output formats
func (j *GpuUsageWeeklyReportJob) shouldRenderPDF() bool {
	if j.config == nil || len(j.config.OutputFormats) == 0 {
		return true // default to enabled
	}

	for _, format := range j.config.OutputFormats {
		if format == "pdf" {
			return true
		}
	}
	return false
}

// getAverageGpuCountFromDB calculates the average GPU count from cluster_gpu_hourly_stats table
func (j *GpuUsageWeeklyReportJob) getAverageGpuCountFromDB(ctx context.Context, clusterName string, period ReportPeriod) (int, error) {
	// Get GpuAggregation facade with cluster filter
	aggFacade := database.GetFacade().GetGpuAggregation().WithCluster(clusterName)

	// Query cluster hourly stats for the period
	stats, err := aggFacade.GetClusterHourlyStats(ctx, period.StartTime, period.EndTime)
	if err != nil {
		return 0, fmt.Errorf("failed to query cluster hourly stats: %w", err)
	}

	if len(stats) == 0 {
		log.Warn("GpuUsageWeeklyReportJob: no cluster hourly stats found in database for the period")
		return 0, nil
	}

	// Calculate average total_gpu_capacity
	var totalGpuCapacity int64 = 0
	for _, stat := range stats {
		totalGpuCapacity += int64(stat.TotalGpuCapacity)
	}

	avgGpuCount := int(totalGpuCapacity / int64(len(stats)))
	log.Infof("GpuUsageWeeklyReportJob: calculated average GPU count from %d hourly stats: %d", len(stats), avgGpuCount)

	return avgGpuCount, nil
}
