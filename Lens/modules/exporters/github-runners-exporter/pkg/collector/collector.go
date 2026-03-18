// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package collector provides GitHub workflow metrics collection functionality.
// This package was migrated from jobs/pkg/jobs/github_workflow_collector.
package collector

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

// WorkflowCollector processes pending runs and collects metrics from Pod PVC
type WorkflowCollector struct {
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

// NewWorkflowCollector creates a new WorkflowCollector instance
func NewWorkflowCollector() *WorkflowCollector {
	return &WorkflowCollector{
		pvcReader:        NewPVCReader(),
		tempPodManager:   NewTempPodManager(),
		schemaAnalyzer:   NewSchemaAnalyzer(),
		metricsExtractor: NewMetricsExtractor(),
		githubFetcher:    nil, // Will be initialized in CollectRun()
		clientSets:       nil,
	}
}

// Initialize initializes the collector with K8s clientsets
func (c *WorkflowCollector) Initialize(clientSets *clientsets.K8SClientSet) {
	if clientSets != nil {
		c.clientSets = clientSets
		InitGithubClientManager(clientSets)
		c.githubFetcher = NewGithubFetcher(clientSets)
	}
}

// CollectRun processes a single workflow run and collects its metrics
// Returns the number of metrics created and any error
func (c *WorkflowCollector) CollectRun(ctx context.Context, run *model.GithubWorkflowRuns) (int, error) {
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	configFacade := database.GetFacade().GetGithubWorkflowConfig()
	schemaFacade := database.GetFacade().GetGithubWorkflowSchema()
	metricsFacade := database.GetFacade().GetGithubWorkflowMetrics()

	// Check retry count
	if run.RetryCount >= MaxRetries {
		log.Warnf("WorkflowCollector: run %d exceeded max retries, marking as failed", run.ID)
		if err := runFacade.MarkFailed(ctx, run.ID, "exceeded max retry count"); err != nil {
			log.Errorf("WorkflowCollector: failed to mark run %d as failed: %v", run.ID, err)
		}
		return 0, fmt.Errorf("exceeded max retries")
	}

	// Dynamically find config for this run
	config := c.findConfigForRun(ctx, run, configFacade)

	if config == nil {
		// No config = no metrics collection, mark as completed without metrics
		log.Infof("WorkflowCollector: run %d (runner_set_id=%d) has no config, marking as completed without metrics",
			run.ID, run.RunnerSetID)
		if err := runFacade.MarkCompleted(ctx, run.ID, 0, 0, 0); err != nil {
			log.Errorf("WorkflowCollector: failed to mark run %d as completed: %v", run.ID, err)
			return 0, err
		}
		return 0, nil
	}

	log.Infof("WorkflowCollector: processing run %d with config %s (id=%d)",
		run.ID, config.Name, config.ID)

	// Process this run with config
	metricsCreated, err := c.processRun(ctx, config, run, schemaFacade, metricsFacade, runFacade)
	if err != nil {
		log.Errorf("WorkflowCollector: failed to process run %d: %v", run.ID, err)
		if err := runFacade.IncrementRetryCount(ctx, run.ID); err != nil {
			log.Errorf("WorkflowCollector: failed to increment retry count for run %d: %v", run.ID, err)
		}
		return 0, err
	}

	// Fetch GitHub data (commits, workflow run details) after successful processing
	if c.githubFetcher != nil {
		if err := c.githubFetcher.FetchAndStoreGithubData(ctx, config, run); err != nil {
			log.Warnf("WorkflowCollector: failed to fetch GitHub data for run %d: %v", run.ID, err)
			// Don't fail the job, just log the warning
		}
	}

	// Auto-generate dashboard summary with regression analysis
	// This runs after metrics extraction and GitHub data fetching
	if err := c.generateDashboardSummary(ctx, config, run); err != nil {
		log.Warnf("WorkflowCollector: failed to generate dashboard summary for run %d: %v", run.ID, err)
		// Don't fail the job, dashboard can be regenerated later
	}

	return metricsCreated, nil
}

// findConfigForRun dynamically finds a config for the given run
// Returns nil if no matching config is found
func (c *WorkflowCollector) findConfigForRun(ctx context.Context, run *model.GithubWorkflowRuns, configFacade database.GithubWorkflowConfigFacadeInterface) *model.GithubWorkflowConfigs {
	// Option 1: Use run.ConfigID if set (from exporter matching)
	if run.ConfigID != 0 {
		config, err := configFacade.GetByID(ctx, run.ConfigID)
		if err == nil && config != nil && config.Enabled {
			return config
		}
		// If config_id is set but config not found or disabled, fall through to dynamic lookup
		log.Debugf("WorkflowCollector: run %d has config_id=%d but config not found or disabled, trying dynamic lookup",
			run.ID, run.ConfigID)
	}

	// Option 2: Find by runner_set (namespace + name)
	if run.RunnerSetNamespace == "" || run.RunnerSetName == "" {
		log.Warnf("WorkflowCollector: run %d missing runner_set_namespace or runner_set_name", run.ID)
		return nil
	}

	configs, err := configFacade.ListByRunnerSet(ctx, run.RunnerSetNamespace, run.RunnerSetName)
	if err != nil {
		log.Errorf("WorkflowCollector: failed to list configs for runner set %s/%s: %v",
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
		if c.matchesFilters(run, cfg) {
			log.Debugf("WorkflowCollector: run %d matched config %s (id=%d) via filters",
				run.ID, cfg.Name, cfg.ID)
			return cfg
		}
	}

	// No matching config found
	return nil
}

// matchesFilters checks if a run matches the workflow and branch filters of a config
func (c *WorkflowCollector) matchesFilters(run *model.GithubWorkflowRuns, cfg *model.GithubWorkflowConfigs) bool {
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
func (c *WorkflowCollector) processRun(
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
		log.Infof("WorkflowCollector: rendered file patterns for run %d: %v -> %v",
			run.ID, filePatterns, renderedPatterns)
	}
	filePatterns = renderedPatterns

	if len(filePatterns) == 0 {
		log.Warnf("WorkflowCollector: no file patterns configured for config %d", config.ID)
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusSkipped, "no file patterns configured"); err != nil {
			return 0, fmt.Errorf("failed to mark as skipped: %w", err)
		}
		return 0, nil
	}

	// EphemeralRunner pods are deleted after completion, so we must create a temporary pod
	// to mount the same PVC and read result files
	log.Infof("WorkflowCollector: creating temp pod to read PVC for run %d", run.ID)

	if c.tempPodManager == nil {
		log.Errorf("WorkflowCollector: temp pod manager not initialized")
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusSkipped, "temp pod manager not available"); err != nil {
			return 0, fmt.Errorf("failed to mark as skipped: %w", err)
		}
		return 0, nil
	}

	// Get volume info from AutoscalingRunnerSet
	volumeInfo, err := c.tempPodManager.GetVolumeInfoFromARS(ctx, config.RunnerSetNamespace, config.RunnerSetName)
	if err != nil {
		log.Warnf("WorkflowCollector: failed to get volume info from ARS: %v", err)
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusSkipped, fmt.Sprintf("failed to get ARS volume info: %v", err)); err != nil {
			return 0, fmt.Errorf("failed to mark as skipped: %w", err)
		}
		return 0, nil
	}

	// Create temporary pod to mount PVC
	podInfo, err := c.tempPodManager.CreateTempPod(ctx, config, run.ID, volumeInfo)
	if err != nil {
		log.Warnf("WorkflowCollector: failed to create temp pod: %v", err)
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusFailed, fmt.Sprintf("failed to create temp pod: %v", err)); err != nil {
			return 0, fmt.Errorf("failed to mark as failed: %w", err)
		}
		return 0, nil
	}

	// Ensure temp pod is cleaned up after use
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.tempPodManager.DeleteTempPod(cleanupCtx, podInfo.Namespace, podInfo.Name); err != nil {
			log.Warnf("WorkflowCollector: failed to cleanup temp pod %s/%s: %v", podInfo.Namespace, podInfo.Name, err)
		}
	}()

	log.Infof("WorkflowCollector: using temp pod %s/%s on node %s",
		podInfo.Namespace, podInfo.Name, podInfo.NodeName)

	// Determine base paths to search
	basePaths := getBasePaths(filePatterns)
	if len(basePaths) == 0 {
		basePaths = []string{DefaultBasePath}
	}

	// List matching files
	matchingFiles, err := c.pvcReader.ListMatchingFiles(ctx, podInfo, basePaths, filePatterns)
	if err != nil {
		return 0, fmt.Errorf("failed to list matching files: %w", err)
	}

	if len(matchingFiles) == 0 {
		log.Infof("WorkflowCollector: no matching files found for run %d", run.ID)
		if err := runFacade.MarkCompleted(ctx, run.ID, 0, 0, 0); err != nil {
			return 0, fmt.Errorf("failed to mark as completed: %w", err)
		}
		return 0, nil
	}

	log.Infof("WorkflowCollector: found %d matching files for run %d", len(matchingFiles), run.ID)

	// Read files from pod
	files, err := c.pvcReader.ReadFiles(ctx, podInfo, matchingFiles, MaxFileSizeBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to read files: %w", err)
	}

	log.Infof("WorkflowCollector: read %d files from pod", len(files))

	// Check if schema analyzer (AI) is available
	aiAvailable := c.schemaAnalyzer != nil && c.schemaAnalyzer.IsAvailable(ctx)

	if aiAvailable {
		// Use schema-versioning approach (AI for schema analysis, Go for data extraction)
		log.Infof("WorkflowCollector: using schema-versioning extraction for run %d", run.ID)

		metricsCreated, err := c.processRunWithSchemaVersioning(
			ctx, config, run, files, matchingFiles, schemaFacade, metricsFacade, runFacade,
		)
		if err != nil {
			return 0, fmt.Errorf("schema-versioning extraction failed: %w", err)
		}
		return metricsCreated, nil
	}

	// AI not available - try fallback: use existing active schema from DB
	log.Warnf("WorkflowCollector: schema analyzer not available for run %d, trying fallback with existing schema", run.ID)

	activeSchema, err := schemaFacade.GetActiveByConfig(ctx, config.ID)
	if err != nil || activeSchema == nil {
		// No active schema in DB and no AI to create one - cannot proceed
		log.Warnf("WorkflowCollector: no active schema found for config %d and AI unavailable, will retry later", config.ID)
		return 0, fmt.Errorf("schema analyzer not available and no active schema in DB for config %d", config.ID)
	}

	log.Infof("WorkflowCollector: using existing active schema (id=%d, hash=%s) for run %d",
		activeSchema.ID, activeSchema.SchemaHash, run.ID)

	// Extract metrics using existing schema (Go-based, no AI needed)
	metrics, err := c.metricsExtractor.ExtractMetricsFromDBSchema(files, activeSchema)
	if err != nil {
		return 0, fmt.Errorf("metrics extraction with existing schema failed: %w", err)
	}

	// Determine timestamp for metrics
	metricsTimestamp := run.WorkloadCompletedAt
	if metricsTimestamp.IsZero() {
		metricsTimestamp = time.Now()
	}

	// Store metrics
	metricsCreated := 0
	dbMetrics := c.metricsExtractor.ConvertToDBMetrics(config.ID, run.ID, activeSchema.ID, metricsTimestamp, metrics)
	for _, metric := range dbMetrics {
		if err := metricsFacade.Create(ctx, metric); err != nil {
			log.Warnf("WorkflowCollector: failed to create metric: %v", err)
			continue
		}
		metricsCreated++
	}

	// Update schema last_seen_at
	if err := schemaFacade.UpdateLastSeen(ctx, activeSchema.ID); err != nil {
		log.Warnf("WorkflowCollector: failed to update schema last_seen: %v", err)
	}

	// Update record count
	if err := schemaFacade.IncrementRecordCount(ctx, activeSchema.ID, int64(metricsCreated)); err != nil {
		log.Warnf("WorkflowCollector: failed to increment record count: %v", err)
	}

	// Mark as completed
	filesProcessed := len(files)
	if err := runFacade.MarkCompleted(ctx, run.ID, int32(len(matchingFiles)), int32(filesProcessed), int32(metricsCreated)); err != nil {
		return metricsCreated, fmt.Errorf("failed to mark as completed: %w", err)
	}

	log.Infof("WorkflowCollector: completed run %d with fallback schema (files: %d/%d, metrics: %d, schema: %d)",
		run.ID, filesProcessed, len(matchingFiles), metricsCreated, activeSchema.ID)

	return metricsCreated, nil
}

// processRunWithSchemaVersioning processes a run using the new schema-versioning approach
// This uses AI only for schema analysis, then Go for metrics extraction (saves LLM tokens)
func (c *WorkflowCollector) processRunWithSchemaVersioning(
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
		log.Warnf("WorkflowCollector: failed to update status to extracting: %v", err)
	}

	// Step 1: Prepare file samples for schema analysis (only headers + few rows)
	var fileSamples []*FileSample
	for _, file := range files {
		sample, err := c.schemaAnalyzer.PrepareFileSample(file)
		if err != nil {
			log.Warnf("WorkflowCollector: failed to prepare sample for %s: %v", file.Path, err)
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
		log.Warnf("WorkflowCollector: failed to list existing schemas: %v", err)
		existingSchemas = []*database.SchemaHashInfo{}
	}

	// Step 3: Call AI Crew for schema analysis ONLY (no metrics extraction)
	schemaResult, err := c.schemaAnalyzer.AnalyzeSchema(ctx, &SchemaAnalysisInput{
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
		// Update last_seen_at and set as active (schema being used should be active)
		if err := schemaFacade.UpdateLastSeen(ctx, schemaID); err != nil {
			log.Warnf("WorkflowCollector: failed to update last_seen: %v", err)
		}
		if err := schemaFacade.SetActive(ctx, config.ID, schemaID); err != nil {
			log.Warnf("WorkflowCollector: failed to set matched schema active: %v", err)
		}
		log.Infof("WorkflowCollector: matched existing schema (id=%d, hash=%s)",
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
			// Update last_seen_at and set as active
			if err := schemaFacade.UpdateLastSeen(ctx, schemaID); err != nil {
				log.Warnf("WorkflowCollector: failed to update last_seen: %v", err)
			}
			if err := schemaFacade.SetActive(ctx, config.ID, schemaID); err != nil {
				log.Warnf("WorkflowCollector: failed to set existing schema active: %v", err)
			}
			log.Infof("WorkflowCollector: found existing schema with same hash (id=%d, hash=%s)",
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
					log.Infof("WorkflowCollector: schema created by concurrent process (id=%d, hash=%s)",
						schemaID, schemaResult.SchemaHash[:8])
				} else {
					return 0, fmt.Errorf("failed to create new schema: %w", err)
				}
			} else {
				schemaID = currentSchema.ID
				// Set this schema as active
				if err := schemaFacade.SetActive(ctx, config.ID, schemaID); err != nil {
					log.Warnf("WorkflowCollector: failed to set schema active: %v", err)
				}
				log.Infof("WorkflowCollector: created new schema version (id=%d, hash=%s)",
					schemaID, schemaResult.SchemaHash[:8])
			}
		}
	}

	// Step 5: Extract metrics using Go (NO AI, NO LLM tokens)
	metrics, err := c.metricsExtractor.ExtractMetrics(files, schemaResult.Schema)
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
	dbMetrics := c.metricsExtractor.ConvertToDBMetrics(config.ID, run.ID, schemaID, metricsTimestamp, metrics)

	for _, metric := range dbMetrics {
		if err := metricsFacade.Create(ctx, metric); err != nil {
			log.Warnf("WorkflowCollector: failed to create metric: %v", err)
			continue
		}
		metricsCreated++
	}

	// Step 8: Update schema record count
	if err := schemaFacade.IncrementRecordCount(ctx, schemaID, int64(metricsCreated)); err != nil {
		log.Warnf("WorkflowCollector: failed to increment record count: %v", err)
	}

	// Mark as completed
	filesProcessed := len(fileSamples)
	if err := runFacade.MarkCompleted(ctx, run.ID, int32(len(matchingFiles)), int32(filesProcessed), int32(metricsCreated)); err != nil {
		return metricsCreated, fmt.Errorf("failed to mark as completed: %w", err)
	}

	log.Infof("WorkflowCollector: completed run %d with schema-versioning (files: %d/%d, metrics: %d, schema: %d)",
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
