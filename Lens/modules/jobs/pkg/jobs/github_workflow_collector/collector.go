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

// GithubWorkflowCollectorJob processes pending runs and collects metrics from Pod PVC
type GithubWorkflowCollectorJob struct {
	// pvcReader reads files from temporary Pod via node-exporter
	pvcReader *PVCReader
	// tempPodManager manages temporary pods for reading PVC (EphemeralRunner pods are deleted after completion)
	tempPodManager *TempPodManager
	// aiExtractor handles AI-based metrics extraction
	aiExtractor *AIExtractor
	// githubFetcher fetches GitHub data (commits, workflow runs)
	githubFetcher *GithubFetcher
	// clientSets is the k8s client set
	clientSets *clientsets.K8SClientSet
}

// NewGithubWorkflowCollectorJob creates a new GithubWorkflowCollectorJob instance
func NewGithubWorkflowCollectorJob() *GithubWorkflowCollectorJob {
	return &GithubWorkflowCollectorJob{
		pvcReader:      NewPVCReader(),
		tempPodManager: NewTempPodManager(),
		aiExtractor:    NewAIExtractor(),
		githubFetcher:  nil, // Will be initialized in Run()
		clientSets:     nil,
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

	// Get all enabled configs
	configFacade := database.GetFacade().GetGithubWorkflowConfig()
	configs, err := configFacade.ListEnabled(ctx)
	if err != nil {
		log.Errorf("GithubWorkflowCollectorJob: failed to list enabled configs: %v", err)
		stats.ErrorCount++
		return stats, err
	}

	if len(configs) == 0 {
		log.Debug("GithubWorkflowCollectorJob: no enabled configs found")
		return stats, nil
	}

	runFacade := database.GetFacade().GetGithubWorkflowRun()
	schemaFacade := database.GetFacade().GetGithubWorkflowSchema()
	metricsFacade := database.GetFacade().GetGithubWorkflowMetrics()

	totalProcessed := 0
	totalCompleted := 0
	totalFailed := 0
	totalMetrics := 0

	for _, config := range configs {
		// Get pending runs for this config
		pendingRuns, err := runFacade.ListPendingByConfig(ctx, config.ID, MaxRunsPerBatch)
		if err != nil {
			log.Errorf("GithubWorkflowCollectorJob: failed to list pending runs for config %d: %v", config.ID, err)
			stats.ErrorCount++
			continue
		}

		if len(pendingRuns) == 0 {
			continue
		}

		log.Infof("GithubWorkflowCollectorJob: processing %d pending runs for config %s", len(pendingRuns), config.Name)

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

			// Process this run
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
	}

	stats.RecordsProcessed = int64(totalProcessed)
	stats.ItemsUpdated = int64(totalCompleted)
	stats.ItemsCreated = int64(totalMetrics)
	stats.ProcessDuration = time.Since(startTime).Seconds()
	stats.AddMessage(fmt.Sprintf("Processed %d runs, completed %d, failed %d, metrics created: %d",
		totalProcessed, totalCompleted, totalFailed, totalMetrics))

	log.Infof("GithubWorkflowCollectorJob: completed - processed: %d, completed: %d, failed: %d, metrics: %d",
		totalProcessed, totalCompleted, totalFailed, totalMetrics)

	return stats, nil
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

	// Get existing schema
	schema, err := schemaFacade.GetActiveByConfig(ctx, config.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to get active schema: %w", err)
	}

	metricsCreated := 0
	filesProcessed := 0

	// Try AI extraction first (if available)
	if j.aiExtractor != nil && j.aiExtractor.IsAvailable(ctx) {
		log.Infof("GithubWorkflowCollectorJob: using AI extraction for run %d", run.ID)

		// Mark as extracting
		if err := runFacade.UpdateStatus(ctx, run.ID, database.WorkflowRunStatusExtracting, ""); err != nil {
			log.Warnf("GithubWorkflowCollectorJob: failed to update status to extracting: %v", err)
		}

		// Use AI extraction
		aiOutput, aiErr := j.aiExtractor.ExtractWithAI(ctx, config, files, schema)
		if aiErr != nil {
			log.Warnf("GithubWorkflowCollectorJob: AI extraction failed for run %d: %v, falling back to basic extraction", run.ID, aiErr)
		} else {
			// Process AI results
			filesProcessed = aiOutput.FilesProcessed

			// If AI generated a new schema, save it
			if aiOutput.SchemaGenerated && aiOutput.Schema != nil && schema == nil {
				newSchema, saveErr := j.aiExtractor.SaveAIGeneratedSchema(ctx, config, aiOutput.Schema, schemaFacade)
				if saveErr != nil {
					log.Warnf("GithubWorkflowCollectorJob: failed to save AI schema: %v", saveErr)
				} else {
					schema = newSchema
				}
			}

			// Use existing or placeholder schema for storing metrics
			if schema == nil {
				schema, err = j.createPlaceholderSchema(ctx, config, schemaFacade)
				if err != nil {
					return 0, fmt.Errorf("failed to create placeholder schema: %w", err)
				}
			}

			// Determine timestamp for metrics
			// Use WorkloadCompletedAt if available, otherwise fall back to current time
			metricsTimestamp := run.WorkloadCompletedAt
			if metricsTimestamp.IsZero() {
				metricsTimestamp = time.Now()
			}

			// Convert and store AI-extracted metrics
			dbMetrics := j.aiExtractor.ConvertAIMetricsToDBMetrics(
				config.ID, run.ID, schema.ID, metricsTimestamp, aiOutput.Metrics,
			)

			for _, metric := range dbMetrics {
				if err := metricsFacade.Create(ctx, metric); err != nil {
					log.Warnf("GithubWorkflowCollectorJob: failed to create AI-extracted metric: %v", err)
					continue
				}
				metricsCreated++
			}

			// Log any extraction errors
			for _, extractErr := range aiOutput.Errors {
				log.Warnf("GithubWorkflowCollectorJob: AI extraction error for %s: %s", extractErr.FilePath, extractErr.Error)
			}

			// Mark as completed
			if err := runFacade.MarkCompleted(ctx, run.ID, int32(len(matchingFiles)), int32(filesProcessed), int32(metricsCreated)); err != nil {
				return metricsCreated, fmt.Errorf("failed to mark as completed: %w", err)
			}

			log.Infof("GithubWorkflowCollectorJob: completed run %d with AI extraction (files: %d/%d, metrics: %d)",
				run.ID, filesProcessed, len(matchingFiles), metricsCreated)

			return metricsCreated, nil
		}
	}

	// Fallback: Basic extraction without AI
	log.Infof("GithubWorkflowCollectorJob: using basic extraction for run %d", run.ID)

	// Ensure we have a schema
	if schema == nil {
		schema, err = j.createPlaceholderSchema(ctx, config, schemaFacade)
		if err != nil {
			return 0, fmt.Errorf("failed to create placeholder schema: %w", err)
		}
	}

	// Parse files and extract metrics using basic parser
	for _, file := range files {
		records, err := parseFileContent(file)
		if err != nil {
			log.Warnf("GithubWorkflowCollectorJob: failed to parse file %s: %v", file.Path, err)
			continue
		}

		filesProcessed++

		// Store metrics
		for _, record := range records {
			metric := &model.GithubWorkflowMetrics{
				ConfigID:   config.ID,
				RunID:      run.ID,
				SchemaID:   schema.ID,
				Timestamp:  run.WorkloadCompletedAt,
				SourceFile: file.Path,
				Dimensions: buildDimensions(record, schema),
				Metrics:    buildMetrics(record, schema),
				RawData:    buildRawData(record),
			}

			if err := metricsFacade.Create(ctx, metric); err != nil {
				log.Warnf("GithubWorkflowCollectorJob: failed to create metric from file %s: %v", file.Path, err)
				continue
			}

			metricsCreated++
		}
	}

	// Mark as completed
	if err := runFacade.MarkCompleted(ctx, run.ID, int32(len(matchingFiles)), int32(filesProcessed), int32(metricsCreated)); err != nil {
		return metricsCreated, fmt.Errorf("failed to mark as completed: %w", err)
	}

	log.Infof("GithubWorkflowCollectorJob: completed run %d with basic extraction (files: %d/%d, metrics: %d)",
		run.ID, filesProcessed, len(matchingFiles), metricsCreated)

	return metricsCreated, nil
}

// createPlaceholderSchema creates a placeholder schema for a config
func (j *GithubWorkflowCollectorJob) createPlaceholderSchema(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	schemaFacade database.GithubWorkflowSchemaFacadeInterface,
) (*model.GithubWorkflowMetricSchemas, error) {
	schema := &model.GithubWorkflowMetricSchemas{
		ConfigID:        config.ID,
		Name:            fmt.Sprintf("%s-auto", config.Name),
		Fields:          model.ExtJSON(`[]`),
		DimensionFields: model.ExtJSON(`[]`),
		MetricFields:    model.ExtJSON(`[]`),
		IsActive:        true,
		GeneratedBy:     database.SchemaGeneratedBySystem,
	}

	if err := schemaFacade.Create(ctx, schema); err != nil {
		return nil, err
	}

	// Update config with schema ID
	if err := database.GetFacade().GetGithubWorkflowConfig().UpdateMetricSchemaID(ctx, config.ID, schema.ID); err != nil {
		log.Warnf("GithubWorkflowCollectorJob: failed to update config schema ID: %v", err)
	}

	return schema, nil
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

// buildDimensions builds dimension map from record based on schema
func buildDimensions(record map[string]interface{}, schema *model.GithubWorkflowMetricSchemas) model.ExtType {
	result := make(model.ExtType)

	// Parse dimension fields from schema
	var dimensionFields []string
	if err := schema.DimensionFields.UnmarshalTo(&dimensionFields); err != nil || len(dimensionFields) == 0 {
		// If no schema defined, try to infer dimensions (string fields)
		for key, value := range record {
			if _, ok := value.(string); ok {
				result[key] = value
			}
		}
		return result
	}

	// Use schema-defined dimensions
	for _, field := range dimensionFields {
		if value, ok := record[field]; ok {
			result[field] = value
		}
	}

	return result
}

// buildMetrics builds metrics map from record based on schema
func buildMetrics(record map[string]interface{}, schema *model.GithubWorkflowMetricSchemas) model.ExtType {
	result := make(model.ExtType)

	// Parse metric fields from schema
	var metricFields []string
	if err := schema.MetricFields.UnmarshalTo(&metricFields); err != nil || len(metricFields) == 0 {
		// If no schema defined, try to infer metrics (numeric fields)
		for key, value := range record {
			switch value.(type) {
			case float64, int64, int, float32:
				result[key] = value
			}
		}
		return result
	}

	// Use schema-defined metrics
	for _, field := range metricFields {
		if value, ok := record[field]; ok {
			result[field] = value
		}
	}

	return result
}

// buildRawData builds raw data from record
func buildRawData(record map[string]interface{}) model.ExtType {
	result := make(model.ExtType)
	for k, v := range record {
		result[k] = v
	}
	return result
}

// Schedule returns the cron schedule for this job
// Runs every 1 minute to process pending runs
func (j *GithubWorkflowCollectorJob) Schedule() string {
	return "@every 1m"
}
