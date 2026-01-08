package github_workflow_collector

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aiclient"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// AIExtractor handles AI-based metrics extraction
type AIExtractor struct {
	// enabled indicates whether AI extraction is available
	enabled bool
}

// NewAIExtractor creates a new AIExtractor instance
func NewAIExtractor() *AIExtractor {
	return &AIExtractor{
		enabled: true,
	}
}

// IsAvailable checks if AI extraction is available
func (e *AIExtractor) IsAvailable(ctx context.Context) bool {
	if !e.enabled {
		return false
	}
	client := aiclient.GetGlobalClient()
	if client == nil {
		return false
	}
	return client.IsAvailable(ctx, aitopics.TopicGithubMetricsExtract)
}

// ExtractWithAI uses AI to extract metrics from files
// Processes each file individually to handle different schemas (e.g., different date columns)
// Returns extracted metrics and optionally a new schema
func (e *AIExtractor) ExtractWithAI(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	files []*PVCFile,
	existingSchema *model.GithubWorkflowMetricSchemas,
) (*aitopics.ExtractMetricsOutput, error) {
	client := aiclient.GetGlobalClient()
	if client == nil {
		return nil, aiclient.ErrAgentUnavailable
	}

	// Process each file individually
	// This allows each file to have its own date columns (e.g., 2025-12-30 vs 2025-12-31)
	var allMetrics []aitopics.ExtractedMetric
	var allErrors []aitopics.ExtractionError
	var generatedSchema *aitopics.MetricSchema
	filesProcessed := 0
	schemaGenerated := false

	aiCtx := aiclient.WithClusterID(ctx, config.ClusterName)

	for i, f := range files {
		// Convert single file to AI input format
		aiFile := aitopics.FileContent{
			Path:      f.Path,
			Name:      f.Name,
			FileType:  detectFileType(f.Name),
			Content:   string(f.Content),
			SizeBytes: int64(len(f.Content)),
		}

		// Build AI request for single file
		aiInput := aitopics.ExtractMetricsInput{
			ConfigID:   config.ID,
			ConfigName: config.Name,
			Files:      []aitopics.FileContent{aiFile},
			Options: &aitopics.ExtractMetricsOptions{
				IncludeRawData:     false,
				IncludeExplanation: false,
			},
		}

		// For first file, use existing schema if available
		// For subsequent files, we don't pass schema - let each file auto-detect its structure
		// This is important because each file may have different date columns
		if i == 0 && existingSchema != nil {
			aiInput.ExistingSchema = convertDBSchemaToAISchema(existingSchema)
		}

		// Invoke AI for this file
		log.Infof("AIExtractor: invoking AI for file %d/%d: %s", i+1, len(files), f.Name)
		resp, err := client.InvokeSync(aiCtx, aitopics.TopicGithubMetricsExtract, aiInput)
		if err != nil {
			log.Warnf("AIExtractor: failed to extract file %s: %v", f.Name, err)
			allErrors = append(allErrors, aitopics.ExtractionError{
				FilePath:    f.Path,
				Error:       err.Error(),
				Recoverable: true,
			})
			continue
		}

		if !resp.IsSuccess() {
			log.Warnf("AIExtractor: AI returned error for file %s: %s", f.Name, resp.Message)
			allErrors = append(allErrors, aitopics.ExtractionError{
				FilePath:    f.Path,
				Error:       resp.Message,
				Recoverable: true,
			})
			continue
		}

		// Parse response
		var fileOutput aitopics.ExtractMetricsOutput
		if err := resp.UnmarshalPayload(&fileOutput); err != nil {
			log.Warnf("AIExtractor: failed to parse response for file %s: %v", f.Name, err)
			allErrors = append(allErrors, aitopics.ExtractionError{
				FilePath:    f.Path,
				Error:       fmt.Sprintf("failed to parse response: %v", err),
				Recoverable: true,
			})
			continue
		}

		// Collect results
		allMetrics = append(allMetrics, fileOutput.Metrics...)
		filesProcessed++

		// Keep the first generated schema
		if fileOutput.Schema != nil && generatedSchema == nil {
			generatedSchema = fileOutput.Schema
			schemaGenerated = fileOutput.SchemaGenerated
		}

		// Collect errors from file processing
		allErrors = append(allErrors, fileOutput.Errors...)
	}

	log.Infof("AIExtractor: extracted %d metrics from %d files", len(allMetrics), filesProcessed)

	// Build combined output
	output := &aitopics.ExtractMetricsOutput{
		Schema:          generatedSchema,
		SchemaGenerated: schemaGenerated,
		Metrics:         allMetrics,
		FilesProcessed:  filesProcessed,
		TotalRecords:    len(allMetrics),
		Errors:          allErrors,
	}

	return output, nil
}

// ExtractWithAISingleFile extracts metrics from a single file (legacy method)
func (e *AIExtractor) ExtractWithAISingleFile(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	file *PVCFile,
	existingSchema *model.GithubWorkflowMetricSchemas,
) (*aitopics.ExtractMetricsOutput, error) {
	return e.ExtractWithAI(ctx, config, []*PVCFile{file}, existingSchema)
}

// extractSingleFileWithAI is the original implementation for reference
func (e *AIExtractor) extractSingleFileWithAI(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	files []*PVCFile,
	existingSchema *model.GithubWorkflowMetricSchemas,
) (*aitopics.ExtractMetricsOutput, error) {
	client := aiclient.GetGlobalClient()
	if client == nil {
		return nil, aiclient.ErrAgentUnavailable
	}

	// Convert PVC files to AI input format
	aiFiles := make([]aitopics.FileContent, 0, len(files))
	for _, f := range files {
		aiFiles = append(aiFiles, aitopics.FileContent{
			Path:      f.Path,
			Name:      f.Name,
			FileType:  detectFileType(f.Name),
			Content:   string(f.Content),
			SizeBytes: int64(len(f.Content)),
		})
	}

	// Build AI request
	aiInput := aitopics.ExtractMetricsInput{
		ConfigID:   config.ID,
		ConfigName: config.Name,
		Files:      aiFiles,
		Options: &aitopics.ExtractMetricsOptions{
			IncludeRawData:     false,
			IncludeExplanation: false,
		},
	}

	// Add existing schema if available
	if existingSchema != nil {
		aiInput.ExistingSchema = convertDBSchemaToAISchema(existingSchema)
	}

	// Add cluster context
	aiCtx := aiclient.WithClusterID(ctx, config.ClusterName)

	// Invoke AI
	log.Infof("AIExtractor: invoking AI to extract metrics from %d files for config %d", len(files), config.ID)
	resp, err := client.InvokeSync(aiCtx, aitopics.TopicGithubMetricsExtract, aiInput)
	if err != nil {
		return nil, fmt.Errorf("AI invocation failed: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, aiclient.NewAPIError(resp.Code, resp.Message)
	}

	// Parse response
	var output aitopics.ExtractMetricsOutput
	if err := resp.UnmarshalPayload(&output); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	log.Infof("AIExtractor: extracted %d metrics from %d files", output.TotalRecords, output.FilesProcessed)
	return &output, nil
}

// GenerateSchemaWithAI uses AI to generate a schema from sample files
func (e *AIExtractor) GenerateSchemaWithAI(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	sampleFiles []*PVCFile,
) (*aitopics.MetricSchema, string, error) {
	client := aiclient.GetGlobalClient()
	if client == nil {
		return nil, "", aiclient.ErrAgentUnavailable
	}

	// Convert PVC files to AI input format
	aiFiles := make([]aitopics.FileContent, 0, len(sampleFiles))
	for _, f := range sampleFiles {
		aiFiles = append(aiFiles, aitopics.FileContent{
			Path:      f.Path,
			Name:      f.Name,
			FileType:  detectFileType(f.Name),
			Content:   string(f.Content),
			SizeBytes: int64(len(f.Content)),
		})
	}

	// Build AI request for schema generation
	aiInput := aitopics.ExtractMetricsInput{
		ConfigID:   config.ID,
		ConfigName: config.Name,
		Files:      aiFiles,
		Options: &aitopics.ExtractMetricsOptions{
			GenerateSchemaOnly: true,
			IncludeExplanation: true,
		},
	}

	// Add cluster context
	aiCtx := aiclient.WithClusterID(ctx, config.ClusterName)

	// Invoke AI
	log.Infof("AIExtractor: invoking AI to generate schema from %d sample files for config %d", len(sampleFiles), config.ID)
	resp, err := client.InvokeSync(aiCtx, aitopics.TopicGithubMetricsExtract, aiInput)
	if err != nil {
		return nil, "", fmt.Errorf("AI invocation failed: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, "", aiclient.NewAPIError(resp.Code, resp.Message)
	}

	// Parse response
	var output aitopics.ExtractMetricsOutput
	if err := resp.UnmarshalPayload(&output); err != nil {
		return nil, "", fmt.Errorf("failed to parse AI response: %w", err)
	}

	if output.Schema == nil {
		return nil, "", fmt.Errorf("AI did not generate a schema")
	}

	log.Infof("AIExtractor: generated schema with %d fields", len(output.Schema.Fields))
	return output.Schema, output.Explanation, nil
}

// SaveAIGeneratedSchema saves an AI-generated schema to the database
func (e *AIExtractor) SaveAIGeneratedSchema(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	aiSchema *aitopics.MetricSchema,
	schemaFacade database.GithubWorkflowSchemaFacadeInterface,
) (*model.GithubWorkflowMetricSchemas, error) {
	// Convert AI schema to database format
	fields, _ := json.Marshal(aiSchema.Fields)
	dimensionFields, _ := json.Marshal(aiSchema.DimensionFields)
	metricFields, _ := json.Marshal(aiSchema.MetricFields)

	schema := &model.GithubWorkflowMetricSchemas{
		ConfigID:        config.ID,
		Name:            aiSchema.Name,
		Fields:          model.ExtJSON(fields),
		DimensionFields: model.ExtJSON(dimensionFields),
		MetricFields:    model.ExtJSON(metricFields),
		IsActive:        true,
		GeneratedBy:     database.SchemaGeneratedByAI,
	}

	if err := schemaFacade.Create(ctx, schema); err != nil {
		return nil, fmt.Errorf("failed to save AI schema: %w", err)
	}

	// Update config with schema ID
	if err := database.GetFacade().GetGithubWorkflowConfig().UpdateMetricSchemaID(ctx, config.ID, schema.ID); err != nil {
		log.Warnf("AIExtractor: failed to update config schema ID: %v", err)
	}

	log.Infof("AIExtractor: saved AI-generated schema %d for config %d", schema.ID, config.ID)
	return schema, nil
}

// ConvertAIMetricsToDBMetrics converts AI-extracted metrics to database format
func (e *AIExtractor) ConvertAIMetricsToDBMetrics(
	configID int64,
	runID int64,
	schemaID int64,
	timestamp time.Time,
	aiMetrics []aitopics.ExtractedMetric,
) []*model.GithubWorkflowMetrics {
	result := make([]*model.GithubWorkflowMetrics, 0, len(aiMetrics))

	for _, m := range aiMetrics {
		metric := &model.GithubWorkflowMetrics{
			ConfigID:   configID,
			RunID:      runID,
			SchemaID:   schemaID,
			Timestamp:  timestamp,
			SourceFile: m.SourceFile,
			Dimensions: convertMapToExtType(m.Dimensions),
			Metrics:    convertMapToExtType(m.Metrics),
		}

		if m.RawData != nil {
			metric.RawData = convertMapToExtType(m.RawData)
		}

		result = append(result, metric)
	}

	return result
}

// detectFileType detects file type from filename
func detectFileType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".json":
		return "json"
	case ".csv":
		return "csv"
	case ".md", ".markdown":
		return "markdown"
	case ".txt":
		return "text"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "unknown"
	}
}

// convertDBSchemaToAISchema converts a database schema to AI format
func convertDBSchemaToAISchema(dbSchema *model.GithubWorkflowMetricSchemas) *aitopics.MetricSchema {
	schema := &aitopics.MetricSchema{
		Name:    dbSchema.Name,
		Version: dbSchema.Version,
	}

	// Parse fields
	var fields []aitopics.SchemaField
	if err := dbSchema.Fields.UnmarshalTo(&fields); err == nil {
		schema.Fields = fields
	}

	// Parse dimension fields
	var dimensionFields []string
	if err := dbSchema.DimensionFields.UnmarshalTo(&dimensionFields); err == nil {
		schema.DimensionFields = dimensionFields
	}

	// Parse metric fields
	var metricFields []string
	if err := dbSchema.MetricFields.UnmarshalTo(&metricFields); err == nil {
		schema.MetricFields = metricFields
	}

	return schema
}

// convertMapToExtType converts a map[string]interface{} to model.ExtType
func convertMapToExtType(m map[string]interface{}) model.ExtType {
	if m == nil {
		return nil
	}
	result := make(model.ExtType)
	for k, v := range m {
		result[k] = v
	}
	return result
}

