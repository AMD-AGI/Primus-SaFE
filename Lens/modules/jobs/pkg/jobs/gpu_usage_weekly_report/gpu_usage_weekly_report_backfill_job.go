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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"github.com/google/uuid"
)

// GpuUsageWeeklyReportBackfillConfig is the configuration for the backfill job
type GpuUsageWeeklyReportBackfillConfig struct {
	// Enabled controls whether the job is enabled
	Enabled bool `json:"enabled"`

	// Cron is the cron schedule for the backfill job (default: "0 3 * * *" - daily at 3:00 AM)
	Cron string `json:"cron"`

	// MaxWeeksToBackfill limits how many weeks to backfill in one run (0 = no limit)
	MaxWeeksToBackfill int `json:"max_weeks_to_backfill"`

	// WeeklyReportConfig is the configuration for generating reports
	WeeklyReportConfig *config.WeeklyReportConfig `json:"-"`
}

// GpuUsageWeeklyReportBackfillJob backfills missing weekly reports for all clusters
type GpuUsageWeeklyReportBackfillJob struct {
	config *GpuUsageWeeklyReportBackfillConfig
}

// NewGpuUsageWeeklyReportBackfillJob creates a new backfill job instance
func NewGpuUsageWeeklyReportBackfillJob(cfg *GpuUsageWeeklyReportBackfillConfig) *GpuUsageWeeklyReportBackfillJob {
	return &GpuUsageWeeklyReportBackfillJob{
		config: cfg,
	}
}

// Run executes the GPU usage weekly report backfill job
func (j *GpuUsageWeeklyReportBackfillJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	startTime := time.Now()
	stats := common.NewExecutionStats()

	if j.config == nil || !j.config.Enabled {
		log.Debug("GpuUsageWeeklyReportBackfillJob: backfill is disabled, skipping")
		stats.AddMessage("Backfill is disabled")
		return stats, nil
	}

	log.Info("GpuUsageWeeklyReportBackfillJob: starting weekly report backfill")

	// Step 1: Get all clusters with data from cluster_gpu_hourly_stats
	clusters, err := j.getDistinctClusters(ctx)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportBackfillJob: failed to get clusters: %v", err)
		return stats, err
	}

	if len(clusters) == 0 {
		log.Info("GpuUsageWeeklyReportBackfillJob: no clusters with data found")
		stats.AddMessage("No clusters with data found")
		return stats, nil
	}

	log.Infof("GpuUsageWeeklyReportBackfillJob: found %d clusters with data", len(clusters))

	totalCreated := int64(0)
	totalSkipped := int64(0)
	totalFailed := int64(0)

	// Step 2: Process each cluster
	for _, clusterName := range clusters {
		created, skipped, failed := j.processCluster(ctx, clusterName)
		totalCreated += created
		totalSkipped += skipped
		totalFailed += failed
	}

	stats.ItemsCreated = totalCreated
	stats.AddCustomMetric("items_skipped", totalSkipped)
	stats.ErrorCount = totalFailed

	duration := time.Since(startTime)
	log.Infof("GpuUsageWeeklyReportBackfillJob: completed in %v, created=%d, skipped=%d, failed=%d",
		duration, totalCreated, totalSkipped, totalFailed)
	stats.AddMessage(fmt.Sprintf("Backfill completed: created=%d, skipped=%d, failed=%d",
		totalCreated, totalSkipped, totalFailed))

	return stats, nil
}

// processCluster processes backfill for a single cluster
func (j *GpuUsageWeeklyReportBackfillJob) processCluster(ctx context.Context, clusterName string) (created, skipped, failed int64) {
	log.Infof("GpuUsageWeeklyReportBackfillJob: processing cluster: %s", clusterName)

	// Get data time range for this cluster
	minTime, maxTime, err := j.getClusterDataTimeRange(ctx, clusterName)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportBackfillJob: failed to get time range for cluster %s: %v", clusterName, err)
		return 0, 0, 1
	}

	if minTime.IsZero() || maxTime.IsZero() {
		log.Infof("GpuUsageWeeklyReportBackfillJob: no data found for cluster %s", clusterName)
		return 0, 0, 0
	}

	log.Infof("GpuUsageWeeklyReportBackfillJob: cluster %s data range: %s to %s",
		clusterName, minTime.Format(time.RFC3339), maxTime.Format(time.RFC3339))

	// Calculate all complete natural weeks in the data range
	weeks := j.calculateNaturalWeeks(minTime, maxTime)
	if len(weeks) == 0 {
		log.Infof("GpuUsageWeeklyReportBackfillJob: no complete weeks found for cluster %s", clusterName)
		return 0, 0, 0
	}

	log.Infof("GpuUsageWeeklyReportBackfillJob: found %d complete weeks for cluster %s", len(weeks), clusterName)

	// Get existing reports for this cluster
	existingReports, err := j.getExistingReports(ctx, clusterName)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportBackfillJob: failed to get existing reports for cluster %s: %v", clusterName, err)
		return 0, 0, 1
	}

	// Find missing weeks
	missingWeeks := j.findMissingWeeks(weeks, existingReports)
	if len(missingWeeks) == 0 {
		log.Infof("GpuUsageWeeklyReportBackfillJob: no missing weeks for cluster %s", clusterName)
		return 0, int64(len(weeks)), 0
	}

	log.Infof("GpuUsageWeeklyReportBackfillJob: found %d missing weeks for cluster %s", len(missingWeeks), clusterName)

	// Apply max weeks limit if configured
	if j.config.MaxWeeksToBackfill > 0 && len(missingWeeks) > j.config.MaxWeeksToBackfill {
		log.Infof("GpuUsageWeeklyReportBackfillJob: limiting backfill to %d weeks (out of %d)",
			j.config.MaxWeeksToBackfill, len(missingWeeks))
		missingWeeks = missingWeeks[:j.config.MaxWeeksToBackfill]
	}

	// Generate reports for missing weeks
	for _, week := range missingWeeks {
		err := j.generateReportForWeek(ctx, clusterName, week)
		if err != nil {
			log.Errorf("GpuUsageWeeklyReportBackfillJob: failed to generate report for week %s - %s: %v",
				week.StartTime.Format("2006-01-02"), week.EndTime.Format("2006-01-02"), err)
			failed++
		} else {
			created++
		}
	}

	skipped = int64(len(weeks) - len(missingWeeks))
	return created, skipped, failed
}

// getDistinctClusters gets all distinct cluster names from cluster_gpu_hourly_stats
func (j *GpuUsageWeeklyReportBackfillJob) getDistinctClusters(ctx context.Context) ([]string, error) {
	db := sql.GetDefaultDB()

	var clusters []string
	err := db.WithContext(ctx).
		Table("cluster_gpu_hourly_stats").
		Distinct("cluster_name").
		Pluck("cluster_name", &clusters).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query distinct clusters: %w", err)
	}

	return clusters, nil
}

// getClusterDataTimeRange gets the min and max time for a cluster's data
func (j *GpuUsageWeeklyReportBackfillJob) getClusterDataTimeRange(ctx context.Context, clusterName string) (minTime, maxTime time.Time, err error) {
	db := sql.GetDefaultDB()

	type TimeRange struct {
		MinTime time.Time
		MaxTime time.Time
	}

	var result TimeRange
	err = db.WithContext(ctx).
		Table("cluster_gpu_hourly_stats").
		Select("MIN(stat_hour) as min_time, MAX(stat_hour) as max_time").
		Where("cluster_name = ?", clusterName).
		Scan(&result).Error

	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to query time range: %w", err)
	}

	return result.MinTime, result.MaxTime, nil
}

// calculateNaturalWeeks calculates all complete natural weeks (Mon 00:00 to Sun 23:59:59) in the time range
func (j *GpuUsageWeeklyReportBackfillJob) calculateNaturalWeeks(minTime, maxTime time.Time) []ReportPeriod {
	var weeks []ReportPeriod

	// Find the first Monday at or after minTime
	firstMonday := j.getNextMonday(minTime)

	// Iterate through all complete weeks
	current := firstMonday
	for {
		// Week start: Monday 00:00:00
		weekStart := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
		// Week end: Sunday 23:59:59.999999999
		weekEnd := weekStart.AddDate(0, 0, 7).Add(-time.Nanosecond)

		// Check if this week is complete (weekEnd should be before or equal to maxTime)
		if weekEnd.After(maxTime) {
			break
		}

		weeks = append(weeks, ReportPeriod{
			StartTime: weekStart,
			EndTime:   weekEnd,
		})

		// Move to next week
		current = current.AddDate(0, 0, 7)
	}

	return weeks
}

// getNextMonday returns the next Monday at or after the given time
func (j *GpuUsageWeeklyReportBackfillJob) getNextMonday(t time.Time) time.Time {
	// Get the weekday (Sunday = 0, Monday = 1, ..., Saturday = 6)
	weekday := int(t.Weekday())

	// Calculate days until next Monday
	daysUntilMonday := (8 - weekday) % 7
	if weekday == int(time.Monday) && t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		// Already at Monday 00:00:00
		daysUntilMonday = 0
	} else if weekday == int(time.Monday) {
		// It's Monday but not at midnight, use this Monday
		daysUntilMonday = 0
	}

	// Return the Monday at 00:00:00
	monday := t.AddDate(0, 0, daysUntilMonday)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
}

// getExistingReports gets all existing reports for a cluster
func (j *GpuUsageWeeklyReportBackfillJob) getExistingReports(ctx context.Context, clusterName string) ([]*dbmodel.GpuUsageWeeklyReports, error) {
	facade := database.GetFacade().GetGpuUsageWeeklyReport()

	// Get all reports for this cluster (no pagination limit for backfill)
	reports, _, err := facade.List(ctx, clusterName, "", 0, 10000)
	if err != nil {
		return nil, err
	}

	return reports, nil
}

// findMissingWeeks finds weeks that don't have existing reports
func (j *GpuUsageWeeklyReportBackfillJob) findMissingWeeks(weeks []ReportPeriod, existingReports []*dbmodel.GpuUsageWeeklyReports) []ReportPeriod {
	// Create a set of existing week starts for quick lookup
	existingWeekStarts := make(map[string]struct{})
	for _, report := range existingReports {
		// Only consider successful reports
		if report.Status == "generated" || report.Status == "sent" {
			// Use week start date as key (format: 2006-01-02)
			key := report.PeriodStart.Format("2006-01-02")
			existingWeekStarts[key] = struct{}{}
		}
	}

	// Find missing weeks
	var missingWeeks []ReportPeriod
	for _, week := range weeks {
		key := week.StartTime.Format("2006-01-02")
		if _, exists := existingWeekStarts[key]; !exists {
			missingWeeks = append(missingWeeks, week)
		}
	}

	return missingWeeks
}

// generateReportForWeek generates a weekly report for a specific week
func (j *GpuUsageWeeklyReportBackfillJob) generateReportForWeek(ctx context.Context, clusterName string, period ReportPeriod) error {
	log.Infof("GpuUsageWeeklyReportBackfillJob: generating report for cluster %s, week %s to %s",
		clusterName, period.StartTime.Format("2006-01-02"), period.EndTime.Format("2006-01-02"))

	// Generate report ID
	reportID := fmt.Sprintf("rpt_backfill_%s_%s_%s",
		period.StartTime.Format("20060102"),
		clusterName,
		uuid.New().String()[:8])

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
		return fmt.Errorf("failed to create report record: %w", err)
	}

	// Call Conductor API to get report data
	generator := NewReportGenerator(j.config.WeeklyReportConfig)
	reportData, err := generator.Generate(ctx, clusterName, period)
	if err != nil {
		report.Status = "failed"
		report.ErrorMessage = fmt.Sprintf("Failed to generate report data: %v", err)
		facade.Update(ctx, report)
		return fmt.Errorf("failed to generate report data: %w", err)
	}

	// Get average GPU count from database
	avgGpuCount, err := j.getAverageGpuCountFromDB(ctx, clusterName, period)
	if err != nil {
		log.Warnf("GpuUsageWeeklyReportBackfillJob: failed to get average GPU count from DB: %v", err)
	} else if avgGpuCount > 0 {
		if reportData.Summary == nil {
			reportData.Summary = &ReportSummary{}
		}
		reportData.Summary.TotalGPUs = avgGpuCount
	}

	// Render HTML
	renderer := NewReportRenderer(j.config.WeeklyReportConfig)
	htmlContent, err := renderer.RenderHTML(ctx, reportData)
	if err != nil {
		report.Status = "failed"
		report.ErrorMessage = fmt.Sprintf("Failed to render HTML: %v", err)
		facade.Update(ctx, report)
		return fmt.Errorf("failed to render HTML: %w", err)
	}
	report.HTMLContent = htmlContent

	// Render PDF if enabled
	if j.shouldRenderPDF() {
		pdfContent, err := renderer.RenderPDF(ctx, htmlContent)
		if err != nil {
			log.Warnf("GpuUsageWeeklyReportBackfillJob: failed to render PDF: %v", err)
		} else {
			report.PdfContent = pdfContent
		}
	}

	// Store JSON content and metadata
	report.JSONContent = reportData.ToExtType()
	report.Metadata = reportData.GenerateMetadata()
	report.Status = "generated"

	// Save report
	err = facade.Update(ctx, report)
	if err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}

	log.Infof("GpuUsageWeeklyReportBackfillJob: successfully generated report %s", reportID)
	return nil
}

// getAverageGpuCountFromDB calculates the average GPU count from cluster_gpu_hourly_stats table
func (j *GpuUsageWeeklyReportBackfillJob) getAverageGpuCountFromDB(ctx context.Context, clusterName string, period ReportPeriod) (int, error) {
	aggFacade := database.GetFacade().GetGpuAggregation().WithCluster(clusterName)

	stats, err := aggFacade.GetClusterHourlyStats(ctx, period.StartTime, period.EndTime)
	if err != nil {
		return 0, fmt.Errorf("failed to query cluster hourly stats: %w", err)
	}

	if len(stats) == 0 {
		return 0, nil
	}

	var totalGpuCapacity int64 = 0
	for _, stat := range stats {
		totalGpuCapacity += int64(stat.TotalGpuCapacity)
	}

	avgGpuCount := int(totalGpuCapacity / int64(len(stats)))
	return avgGpuCount, nil
}

// shouldRenderPDF checks if PDF rendering is enabled in output formats
func (j *GpuUsageWeeklyReportBackfillJob) shouldRenderPDF() bool {
	if j.config == nil || j.config.WeeklyReportConfig == nil || len(j.config.WeeklyReportConfig.OutputFormats) == 0 {
		return true // default to enabled
	}

	for _, format := range j.config.WeeklyReportConfig.OutputFormats {
		if format == "pdf" {
			return true
		}
	}
	return false
}

// Schedule returns the cron schedule for this job
func (j *GpuUsageWeeklyReportBackfillJob) Schedule() string {
	if j.config != nil && j.config.Cron != "" {
		return j.config.Cron
	}
	// Default: run once per day at 3:00 AM to backfill any missing reports
	return "0 3 * * *"
}

