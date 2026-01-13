// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package github_workflow_collector

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/types"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
)

const (
	// MaxRunsPerBatch limits the number of runs processed per batch
	MaxRunsPerBatch = 10
	// MaxRetries is the maximum number of retries for a failed run
	MaxRetries = 3
	// MaxFileSizeBytes is the maximum file size to process (5MB)
	MaxFileSizeBytes = 5 * 1024 * 1024
	// DefaultBasePath is the default base path to search for files in the container
	DefaultBasePath = "/workspace"
)

// renderFilePatterns replaces date placeholders in file patterns with actual values
// Supported placeholders:
//   - {date}     -> 2025-12-30
//   - {year}     -> 2025
//   - {month}    -> 12
//   - {day}      -> 30
//   - {yyyymmdd} -> 20251230
func renderFilePatterns(patterns []string, referenceTime time.Time) []string {
	if referenceTime.IsZero() {
		referenceTime = time.Now()
	}

	replacements := map[string]string{
		"{date}":     referenceTime.Format("2006-01-02"),
		"{year}":     referenceTime.Format("2006"),
		"{month}":    referenceTime.Format("01"),
		"{day}":      referenceTime.Format("02"),
		"{yyyymmdd}": referenceTime.Format("20060102"),
	}

	rendered := make([]string, len(patterns))
	for i, pattern := range patterns {
		rendered[i] = pattern
		for placeholder, value := range replacements {
			rendered[i] = strings.ReplaceAll(rendered[i], placeholder, value)
		}
	}

	return rendered
}

// GithubWorkflowCollectorJob processes pending runs and collects metrics from Pod PVC
type GithubWorkflowCollectorJob struct {
	// pvcReader reads files from temporary Pod via node-exporter
	pvcReader *PVCReader
	// tempPodManager manages temporary pods for reading PVC (EphemeralRunner pods are deleted after completion)
	tempPodManager *TempPodManager
	// schemaAnalyzer handles AI-based schema analysis (new simplified approach)
	schemaAnalyzer *SchemaAnalyzer
	// metricsExtractor extracts metrics based on schema (Go-based, no LLM)
	metricsExtractor *MetricsExtractor
	// githubFetcher fetches GitHub data (commits, workflow runs)
	githubFetcher *GithubFetcher
	// clientSets is the k8s client set
	clientSets *clientsets.K8SClientSet
}

// NewGithubWorkflowCollectorJob creates a new GithubWorkflowCollectorJob instance
func NewGithubWorkflowCollectorJob() *GithubWorkflowCollectorJob {
	return &GithubWorkflowCollectorJob{
		pvcReader:        NewPVCReader(),
		tempPodManager:   NewTempPodManager(),
		schemaAnalyzer:   NewSchemaAnalyzer(),
		metricsExtractor: NewMetricsExtractor(),
		githubFetcher:    nil, // Will be initialized in Run()
		clientSets:       nil,
	}
}

// Run executes the GitHub workflow collector job
func (j *GithubWorkflowCollectorJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	startTime := time.Now()
	stats := common.NewExecutionStats()

	log.Info("GithubWorkflowCollectorJob: starting collection")

	// Initialize GitHub fetcher if not already done
	if j.githubFetcher == nil && clientSets != nil {
		j.clientSets = clientSets
		InitGithubClientManager(clientSets)
		j.githubFetcher = NewGithubFetcher(clientSets)
	}

	// Get ALL pending runs (not per-config)
	runFacade := database.GetFacade().GetGithubWorkflowRun()

	// Step 0: Batch mark pending runs without config as completed (optimization)
	// This prevents runs without matching configs from blocking the queue
	if affected, err := runFacade.MarkPendingWithoutConfigAsCompleted(ctx); err != nil {
		log.Warnf("GithubWorkflowCollectorJob: failed to batch mark pending without config: %v", err)
	} else if affected > 0 {
		log.Infof("GithubWorkflowCollectorJob: batch marked %d pending runs without config as completed", affected)
	}
	pendingRuns, err := runFacade.ListPending(ctx, &database.GithubWorkflowRunFilter{
		Status: database.WorkflowRunStatusPending,
		Limit:  MaxRunsPerBatch,
	})
	if err != nil {
		log.Errorf("GithubWorkflowCollectorJob: failed to list pending runs: %v", err)
		stats.ErrorCount++
		return stats, err
	}

	if len(pendingRuns) == 0 {
		log.Debug("GithubWorkflowCollectorJob: no pending runs found")
		return stats, nil
	}

	log.Infof("GithubWorkflowCollectorJob: processing %d pending runs", len(pendingRuns))

	schemaFacade := database.GetFacade().GetGithubWorkflowSchema()
	metricsFacade := database.GetFacade().GetGithubWorkflowMetrics()

	totalProcessed := 0
	totalCompleted := 0
	totalFailed := 0
	totalSkipped := 0
	totalMetrics := 0

	for _, run := range pendingRuns {
		// Check retry count
		if run.RetryCount >= MaxRetries {
			log.Warnf("GithubWorkflowCollectorJob: run %d exceeded max retries, marking as failed", run.ID)
			if err := runFacade.MarkFailed(ctx, run.ID, "exceeded max retry count"); err != nil {
				log.Errorf("GithubWorkflowCollectorJob: failed to mark run %d as failed: %v", run.ID, err)
			}
			totalFailed++
			continue
		}

		// Dynamically find config for this run
		config := j.findConfigForRun(ctx, run)

		if config == nil {
			// No config = no metrics collection, mark as completed without metrics
			log.Infof("GithubWorkflowCollectorJob: run %d (runner_set_id=%d) has no config, marking as completed without metrics",
				run.ID, run.RunnerSetID)
			if err := runFacade.MarkCompleted(ctx, run.ID, 0, 0, 0); err != nil {
				log.Errorf("GithubWorkflowCollectorJob: failed to mark run %d as completed: %v", run.ID, err)
				stats.ErrorCount++
			}
			totalSkipped++
			totalProcessed++
			continue
		}

		log.Infof("GithubWorkflowCollectorJob: processing run %d with config %s (id=%d)",
			run.ID, config.Name, config.ID)

		// Process this run with config
		metricsCreated, err := j.processRun(ctx, config, run, schemaFacade, metricsFacade, runFacade)
		if err != nil {
			log.Errorf("GithubWorkflowCollectorJob: failed to process run %d: %v", run.ID, err)
			if err := runFacade.IncrementRetryCount(ctx, run.ID); err != nil {
				log.Errorf("GithubWorkflowCollectorJob: failed to increment retry count for run %d: %v", run.ID, err)
			}
			stats.ErrorCount++
			totalFailed++
			continue
		}

		// Fetch GitHub data (commits, workflow run details) after successful processing
		if j.githubFetcher != nil {
			if err := j.githubFetcher.FetchAndStoreGithubData(ctx, config, run); err != nil {
				log.Warnf("GithubWorkflowCollectorJob: failed to fetch GitHub data for run %d: %v", run.ID, err)
				// Don't fail the job, just log the warning
			}
		}

		totalCompleted++
		totalProcessed++
		totalMetrics += metricsCreated
	}

	stats.RecordsProcessed = int64(totalProcessed)
	stats.ItemsUpdated = int64(totalCompleted)
	stats.ItemsCreated = int64(totalMetrics)
	stats.ProcessDuration = time.Since(startTime).Seconds()
	stats.AddMessage(fmt.Sprintf("Processed %d runs, completed %d, skipped %d (no config), failed %d, metrics created: %d",
		totalProcessed, totalCompleted, totalSkipped, totalFailed, totalMetrics))

	log.Infof("GithubWorkflowCollectorJob: completed - processed: %d, completed: %d, skipped: %d, failed: %d, metrics: %d",
		totalProcessed, totalCompleted, totalSkipped, totalFailed, totalMetrics)

	return stats, nil
}

// findConfigForRun dynamically finds a config for the given run
// Returns nil if no matching config is found
func (j *GithubWorkflowCollectorJob) findConfigForRun(ctx context.Context, run *model.GithubWorkflowRuns) *model.GithubWorkflowConfigs {
	configFacade := database.GetFacade().GetGithubWorkflowConfig()

	// Option 1: Use run.ConfigID if set (from exporter matching)
	if run.ConfigID != 0 {
		config, err := configFacade.GetByID(ctx, run.ConfigID)
		if err == nil && config != nil && config.Enabled {
			return config
		}
		// If config_id is set but config not found or disabled, fall through to dynamic lookup
		log.Debugf("GithubWorkflowCollectorJob: run %d has config_id=%d but config not found or disabled, trying dynamic lookup",
			run.ID, run.ConfigID)
	}

	// Option 2: Find by runner_set (namespace + name)
	if run.RunnerSetNamespace == "" || run.RunnerSetName == "" {
		log.Warnf("GithubWorkflowCollectorJob: run %d missing runner_set_namespace or runner_set_name", run.ID)
		return nil
	}

	configs, err := configFacade.ListByRunnerSet(ctx, run.RunnerSetNamespace, run.RunnerSetName)
	if err != nil {
		log.Errorf("GithubWorkflowCollectorJob: failed to list configs for runner set %s/%s: %v",
			run.RunnerSetNamespace, run.RunnerSetName, err)
		return nil
	}

	if len(configs) == 0 {
		return nil
	}

	// Option 3: Match by workflow/branch filters
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		if j.matchesFilters(run, cfg) {
			log.Debugf("GithubWorkflowCollectorJob: run %d matched config %s (id=%d) via filters",
				run.ID, cfg.Name, cfg.ID)
			return cfg
		}
	}

	// No matching config found
	return nil
}

// matchesFilters checks if a run matches the workflow and branch filters of a config
func (j *GithubWorkflowCollectorJob) matchesFilters(run *model.GithubWorkflowRuns, cfg *model.GithubWorkflowConfigs) bool {
	// Check workflow_filter
	if cfg.WorkflowFilter != "" && run.WorkflowName != "" {
		if run.WorkflowName != cfg.WorkflowFilter {
			return false
		}
	}

	// Check branch_filter
	if cfg.BranchFilter != "" && run.HeadBranch != "" {
		if run.HeadBranch != cfg.BranchFilter {
			return false
		}
	}

	// If no filters are set, or all filters match, return true
	return true
}

// processRun processes a single pending run
func (j *GithubWorkflowCollectorJob) processRun(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	run *model.GithubWorkflowRuns,
	schemaFacade database.GithubWorkflowSchemaFacadeInterface,
	metricsFacade database.GithubWorkflowMetricsFacadeInterface,
	runFacade database.GithubWorkflowRunFacadeInterface,
) (int, error) {
	// Mark as collecting
	if err := runFacade.MarkCollecting(ctx, run.ID); err != nil {
		return 0, fmt.Errorf("failed to mark as collecting: %w", err)
	}

	// Parse file patterns from config
	var filePatterns []string
	if err := config.FilePatterns.UnmarshalTo(&filePatterns); err != nil {
		return 0, fmt.Errorf("failed to parse file patterns: %w", err)
	}

	// Render date placeholders in file patterns using workload completion time
	// This allows patterns like /path/{date}/**/summary.csv to only match current day's data
	referenceTime := run.WorkloadCompletedAt
	if referenceTime.IsZero() {
		referenceTime = run.WorkloadStartedAt
	}
	renderedPatterns := renderFilePatterns(filePatterns, referenceTime)
	if len(renderedPatterns) > 0 && renderedPatterns[0] != filePatterns[0] {
		log.Infof("GithubWorkflowCollectorJob: rendered file patterns for run %d: %v -> %v",
			run.ID, filePatterns, renderedPatterns)
	}
	filePatterns = renderedPatterns

	if len(filePatterns) == 0 {
		log.Warnf("GithubWorkflowCollectorJob: no file patterns configured for config %d", config.ID)
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusSkipped, "no file patterns configured"); err != nil {
			return 0, fmt.Errorf("failed to mark as skipped: %w", err)
		}
		return 0, nil
	}

	// EphemeralRunner pods are deleted after completion, so we must create a temporary pod
	// to mount the same PVC and read result files
	log.Infof("GithubWorkflowCollectorJob: creating temp pod to read PVC for run %d", run.ID)

	if j.tempPodManager == nil {
		log.Errorf("GithubWorkflowCollectorJob: temp pod manager not initialized")
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusSkipped, "temp pod manager not available"); err != nil {
			return 0, fmt.Errorf("failed to mark as skipped: %w", err)
		}
		return 0, nil
	}

	// Get volume info from AutoscalingRunnerSet
	volumeInfo, err := j.tempPodManager.GetVolumeInfoFromARS(ctx, config.RunnerSetNamespace, config.RunnerSetName)
	if err != nil {
		log.Warnf("GithubWorkflowCollectorJob: failed to get volume info from ARS: %v", err)
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusSkipped, fmt.Sprintf("failed to get ARS volume info: %v", err)); err != nil {
			return 0, fmt.Errorf("failed to mark as skipped: %w", err)
		}
		return 0, nil
	}

	// Create temporary pod to mount PVC
	podInfo, err := j.tempPodManager.CreateTempPod(ctx, config, run.ID, volumeInfo)
	if err != nil {
		log.Warnf("GithubWorkflowCollectorJob: failed to create temp pod: %v", err)
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusFailed, fmt.Sprintf("failed to create temp pod: %v", err)); err != nil {
			return 0, fmt.Errorf("failed to mark as failed: %w", err)
		}
		return 0, nil
	}

	// Ensure temp pod is cleaned up after use
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := j.tempPodManager.DeleteTempPod(cleanupCtx, podInfo.Namespace, podInfo.Name); err != nil {
			log.Warnf("GithubWorkflowCollectorJob: failed to cleanup temp pod %s/%s: %v", podInfo.Namespace, podInfo.Name, err)
		}
	}()

	log.Infof("GithubWorkflowCollectorJob: using temp pod %s/%s on node %s",
		podInfo.Namespace, podInfo.Name, podInfo.NodeName)

	// Determine base paths to search
	basePaths := getBasePaths(filePatterns)
	if len(basePaths) == 0 {
		basePaths = []string{DefaultBasePath}
	}

	// List matching files
	matchingFiles, err := j.pvcReader.ListMatchingFiles(ctx, podInfo, basePaths, filePatterns)
	if err != nil {
		return 0, fmt.Errorf("failed to list matching files: %w", err)
	}

	if len(matchingFiles) == 0 {
		log.Infof("GithubWorkflowCollectorJob: no matching files found for run %d", run.ID)
		if err := runFacade.MarkCompleted(ctx, run.ID, 0, 0, 0); err != nil {
			return 0, fmt.Errorf("failed to mark as completed: %w", err)
		}
		return 0, nil
	}

	log.Infof("GithubWorkflowCollectorJob: found %d matching files for run %d", len(matchingFiles), run.ID)

	// Read files from pod
	files, err := j.pvcReader.ReadFiles(ctx, podInfo, matchingFiles, MaxFileSizeBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to read files: %w", err)
	}

	log.Infof("GithubWorkflowCollectorJob: read %d files from pod", len(files))

	// Check if schema analyzer is available
	if j.schemaAnalyzer == nil || !j.schemaAnalyzer.IsAvailable(ctx) {
		// Schema analyzer not available - skip processing, will retry later
		log.Warnf("GithubWorkflowCollectorJob: schema analyzer not available for run %d, will retry later", run.ID)
		return 0, fmt.Errorf("schema analyzer not available")
	}

	// Use schema-versioning approach (AI for schema analysis, Go for data extraction)
	log.Infof("GithubWorkflowCollectorJob: using schema-versioning extraction for run %d", run.ID)

	metricsCreated, err := j.processRunWithSchemaVersioning(
		ctx, config, run, files, matchingFiles, schemaFacade, metricsFacade, runFacade,
	)
	if err != nil {
		return 0, fmt.Errorf("schema-versioning extraction failed: %w", err)
	}

	return metricsCreated, nil
}

// processRunWithSchemaVersioning processes a run using the new schema-versioning approach
// This uses AI only for schema analysis, then Go for metrics extraction (saves LLM tokens)
func (j *GithubWorkflowCollectorJob) processRunWithSchemaVersioning(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	run *model.GithubWorkflowRuns,
	files []*PVCFile,
	matchingFiles []*types.ContainerFileInfo,
	schemaFacade database.GithubWorkflowSchemaFacadeInterface,
	metricsFacade database.GithubWorkflowMetricsFacadeInterface,
	runFacade database.GithubWorkflowRunFacadeInterface,
) (int, error) {
	// Mark as extracting
	if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusExtracting, ""); err != nil {
		log.Warnf("GithubWorkflowCollectorJob: failed to update status to extracting: %v", err)
	}

	// Step 1: Prepare file samples for schema analysis (only headers + few rows)
	var fileSamples []*FileSample
	for _, file := range files {
		sample, err := j.schemaAnalyzer.PrepareFileSample(file)
		if err != nil {
			log.Warnf("GithubWorkflowCollectorJob: failed to prepare sample for %s: %v", file.Path, err)
			continue
		}
		fileSamples = append(fileSamples, sample)
	}

	if len(fileSamples) == 0 {
		return 0, fmt.Errorf("no valid file samples for schema analysis")
	}

	// Step 2: Get existing schemas for matching
	existingSchemas, err := schemaFacade.ListByConfigWithHash(ctx, config.ID)
	if err != nil {
		log.Warnf("GithubWorkflowCollectorJob: failed to list existing schemas: %v", err)
		existingSchemas = []*database.SchemaHashInfo{}
	}

	// Step 3: Call AI Crew for schema analysis ONLY (no metrics extraction)
	schemaResult, err := j.schemaAnalyzer.AnalyzeSchema(ctx, &SchemaAnalysisInput{
		ConfigID:        config.ID,
		ConfigName:      config.Name,
		FileSamples:     fileSamples,
		ExistingSchemas: existingSchemas,
	})
	if err != nil {
		return 0, fmt.Errorf("schema analysis failed: %w", err)
	}

	if !schemaResult.Success {
		return 0, fmt.Errorf("schema analysis failed: %s", schemaResult.Error)
	}

	// Step 4: Get or create schema version
	var schemaID int64
	var currentSchema *model.GithubWorkflowMetricSchemas

	if schemaResult.SchemaMatched && schemaResult.MatchedSchemaID != nil {
		// Use existing schema
		schemaID = *schemaResult.MatchedSchemaID
		currentSchema, err = schemaFacade.GetByID(ctx, schemaID)
		if err != nil {
			return 0, fmt.Errorf("failed to get matched schema: %w", err)
		}
		// Update last_seen_at
		if err := schemaFacade.UpdateLastSeen(ctx, schemaID); err != nil {
			log.Warnf("GithubWorkflowCollectorJob: failed to update last_seen: %v", err)
		}
		log.Infof("GithubWorkflowCollectorJob: matched existing schema (id=%d, hash=%s)",
			schemaID, schemaResult.SchemaHash[:8])
	} else {
		// First check if schema with this hash already exists (handle concurrent creation)
		existingSchema, err := schemaFacade.GetByConfigAndHash(ctx, config.ID, schemaResult.SchemaHash)
		if err != nil {
			return 0, fmt.Errorf("failed to check existing schema: %w", err)
		}

		if existingSchema != nil {
			// Schema already exists (created by concurrent process), use it
			currentSchema = existingSchema
			schemaID = existingSchema.ID
			// Update last_seen_at
			if err := schemaFacade.UpdateLastSeen(ctx, schemaID); err != nil {
				log.Warnf("GithubWorkflowCollectorJob: failed to update last_seen: %v", err)
			}
			log.Infof("GithubWorkflowCollectorJob: found existing schema with same hash (id=%d, hash=%s)",
				schemaID, schemaResult.SchemaHash[:8])
		} else {
			// Create new schema version
			currentSchema = ConvertSchemaToDBModel(schemaResult.Schema, config.ID)
			currentSchema.SchemaHash = schemaResult.SchemaHash
			currentSchema.FirstSeenAt = time.Now()
			currentSchema.LastSeenAt = time.Now()

			if err := schemaFacade.Create(ctx, currentSchema); err != nil {
				// Handle race condition: another process may have created the schema
				if existingSchema, lookupErr := schemaFacade.GetByConfigAndHash(ctx, config.ID, schemaResult.SchemaHash); lookupErr == nil && existingSchema != nil {
					currentSchema = existingSchema
					schemaID = existingSchema.ID
					log.Infof("GithubWorkflowCollectorJob: schema created by concurrent process (id=%d, hash=%s)",
						schemaID, schemaResult.SchemaHash[:8])
				} else {
					return 0, fmt.Errorf("failed to create new schema: %w", err)
				}
			} else {
				schemaID = currentSchema.ID
				// Set this schema as active
				if err := schemaFacade.SetActive(ctx, config.ID, schemaID); err != nil {
					log.Warnf("GithubWorkflowCollectorJob: failed to set schema active: %v", err)
				}
				log.Infof("GithubWorkflowCollectorJob: created new schema version (id=%d, hash=%s)",
					schemaID, schemaResult.SchemaHash[:8])
			}
		}
	}

	// Step 5: Extract metrics using Go (NO AI, NO LLM tokens)
	metrics, err := j.metricsExtractor.ExtractMetrics(files, schemaResult.Schema)
	if err != nil {
		return 0, fmt.Errorf("metrics extraction failed: %w", err)
	}

	// Step 6: Determine timestamp for metrics
	metricsTimestamp := run.WorkloadCompletedAt
	if metricsTimestamp.IsZero() {
		metricsTimestamp = time.Now()
	}

	// Step 7: Store metrics with schema_id
	metricsCreated := 0
	dbMetrics := j.metricsExtractor.ConvertToDBMetrics(config.ID, run.ID, schemaID, metricsTimestamp, metrics)

	for _, metric := range dbMetrics {
		if err := metricsFacade.Create(ctx, metric); err != nil {
			log.Warnf("GithubWorkflowCollectorJob: failed to create metric: %v", err)
			continue
		}
		metricsCreated++
	}

	// Step 8: Update schema record count
	if err := schemaFacade.IncrementRecordCount(ctx, schemaID, int64(metricsCreated)); err != nil {
		log.Warnf("GithubWorkflowCollectorJob: failed to increment record count: %v", err)
	}

	// Mark as completed
	filesProcessed := len(fileSamples)
	if err := runFacade.MarkCompleted(ctx, run.ID, int32(len(matchingFiles)), int32(filesProcessed), int32(metricsCreated)); err != nil {
		return metricsCreated, fmt.Errorf("failed to mark as completed: %w", err)
	}

	log.Infof("GithubWorkflowCollectorJob: completed run %d with schema-versioning (files: %d/%d, metrics: %d, schema: %d)",
		run.ID, filesProcessed, len(matchingFiles), metricsCreated, schemaID)

	return metricsCreated, nil
}

// getBasePaths extracts base paths from file patterns
func getBasePaths(patterns []string) []string {
	pathSet := make(map[string]bool)

	for _, pattern := range patterns {
		// Handle absolute paths
		if strings.HasPrefix(pattern, "/") {
			// Extract the directory part before any wildcard
			parts := strings.Split(pattern, "/")
			var basePath []string
			for _, part := range parts {
				if strings.ContainsAny(part, "*?[") {
					break
				}
				basePath = append(basePath, part)
			}
			if len(basePath) > 1 { // More than just "/"
				pathSet[strings.Join(basePath, "/")] = true
			} else {
				pathSet[DefaultBasePath] = true
			}
		} else {
			// Relative patterns search in default path
			pathSet[DefaultBasePath] = true
		}
	}

	var paths []string
	for path := range pathSet {
		paths = append(paths, path)
	}

	if len(paths) == 0 {
		paths = []string{DefaultBasePath}
	}

	return paths
}

// parseFileContent parses file content based on file extension
func parseFileContent(file *PVCFile) ([]map[string]interface{}, error) {
	ext := strings.ToLower(filepath.Ext(file.Name))

	switch ext {
	case ".json":
		return parseJSON(file.Content)
	case ".csv":
		return parseCSV(file.Content)
	case ".md", ".markdown":
		return parseMarkdown(file.Content)
	default:
		// Try JSON first, then CSV
		if records, err := parseJSON(file.Content); err == nil {
			return records, nil
		}
		return parseCSV(file.Content)
	}
}

// parseJSON parses JSON content into records
func parseJSON(content []byte) ([]map[string]interface{}, error) {
	// Try parsing as array first
	var arrayResult []map[string]interface{}
	if err := json.Unmarshal(content, &arrayResult); err == nil {
		return arrayResult, nil
	}

	// Try parsing as single object
	var singleResult map[string]interface{}
	if err := json.Unmarshal(content, &singleResult); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return []map[string]interface{}{singleResult}, nil
}

// parseCSV parses CSV content into records
func parseCSV(content []byte) ([]map[string]interface{}, error) {
	reader := csv.NewReader(strings.NewReader(string(content)))
	reader.TrimLeadingSpace = true

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV must have at least header and one data row")
	}

	headers := records[0]
	var results []map[string]interface{}

	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) != len(headers) {
			log.Warnf("CSV row %d has %d columns, expected %d", i+1, len(row), len(headers))
			continue
		}

		record := make(map[string]interface{})
		for j, header := range headers {
			value := row[j]
			// Try to parse as number
			if num, err := strconv.ParseFloat(value, 64); err == nil {
				record[header] = num
			} else if num, err := strconv.ParseInt(value, 10, 64); err == nil {
				record[header] = num
			} else {
				record[header] = value
			}
		}
		results = append(results, record)
	}

	return results, nil
}

// parseMarkdown parses markdown tables into records
func parseMarkdown(content []byte) ([]map[string]interface{}, error) {
	lines := strings.Split(string(content), "\n")

	var results []map[string]interface{}
	var headers []string
	inTable := false

	// Regex for markdown table row: | col1 | col2 | col3 |
	tableRowRegex := regexp.MustCompile(`^\s*\|(.+)\|\s*$`)
	separatorRegex := regexp.MustCompile(`^\s*\|[-:\s|]+\|\s*$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if !tableRowRegex.MatchString(line) {
			inTable = false
			headers = nil
			continue
		}

		// Extract cells
		match := tableRowRegex.FindStringSubmatch(line)
		if len(match) < 2 {
			continue
		}

		cells := strings.Split(match[1], "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}

		// Skip separator row
		if separatorRegex.MatchString(line) {
			continue
		}

		if !inTable {
			// This is the header row
			headers = cells
			inTable = true
			continue
		}

		// This is a data row
		if len(cells) != len(headers) {
			continue
		}

		record := make(map[string]interface{})
		for j, header := range headers {
			if header == "" {
				continue
			}
			value := cells[j]
			// Try to parse as number
			if num, err := strconv.ParseFloat(value, 64); err == nil {
				record[header] = num
			} else if num, err := strconv.ParseInt(value, 10, 64); err == nil {
				record[header] = num
			} else {
				record[header] = value
			}
		}

		if len(record) > 0 {
			results = append(results, record)
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no tables found in markdown")
	}

	return results, nil
}

// Schedule returns the cron schedule for this job
// Runs every 1 minute to process pending runs
func (j *GithubWorkflowCollectorJob) Schedule() string {
	return "@every 1m"
}
