// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package github_workflow_collector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aiclient"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// SchemaAnalyzer handles schema analysis using AI Crew
type SchemaAnalyzer struct {
	enabled bool
}

// NewSchemaAnalyzer creates a new SchemaAnalyzer instance
func NewSchemaAnalyzer() *SchemaAnalyzer {
	return &SchemaAnalyzer{
		enabled: true,
	}
}

// SchemaAnalysisInput is the input for schema analysis
type SchemaAnalysisInput struct {
	ConfigID        int64
	ConfigName      string
	FileSamples     []*FileSample
	ExistingSchemas []*database.SchemaHashInfo
}

// FileSample contains file header and sample data for schema analysis
type FileSample struct {
	FilePath    string
	FileType    string
	Headers     []string
	SampleRows  []map[string]interface{}
	ColumnTypes map[string]string
}

// SchemaAnalysisOutput is the output from schema analysis
type SchemaAnalysisOutput struct {
	Success         bool
	Error           string
	Schema          *SchemaDefinition
	SchemaHash      string
	SchemaMatched   bool
	MatchedSchemaID *int64
}

// SchemaDefinition defines the schema structure
type SchemaDefinition struct {
	Name            string   `json:"name"`
	DimensionFields []string `json:"dimension_fields"`
	MetricFields    []string `json:"metric_fields"`
	IsWideTable     bool     `json:"is_wide_table"`
	DateColumns     []string `json:"date_columns"`
	TimeField       string   `json:"time_field,omitempty"` // Field name that represents time (auto-set for wide tables)
}

// IsAvailable checks if schema analysis is available
func (a *SchemaAnalyzer) IsAvailable(ctx context.Context) bool {
	if !a.enabled {
		return false
	}
	client := aiclient.GetGlobalClient()
	if client == nil {
		return false
	}
	return client.IsAvailable(ctx, aitopics.TopicGithubSchemaAnalyze)
}

// AnalyzeSchema analyzes file samples to determine schema structure
func (a *SchemaAnalyzer) AnalyzeSchema(
	ctx context.Context,
	input *SchemaAnalysisInput,
) (*SchemaAnalysisOutput, error) {
	client := aiclient.GetGlobalClient()
	if client == nil {
		return nil, aiclient.ErrAgentUnavailable
	}

	// Prepare input for AI Crew
	files := make([]aitopics.FileContent, 0, len(input.FileSamples))
	for _, sample := range input.FileSamples {
		// Convert sample to content string (headers + sample rows)
		content := a.sampleToContent(sample)
		files = append(files, aitopics.FileContent{
			Path:     sample.FilePath,
			Name:     sample.FilePath,
			FileType: sample.FileType,
			Content:  content,
		})
	}

	// Convert existing schemas to AI format
	existingSchemas := make([]map[string]interface{}, 0, len(input.ExistingSchemas))
	for _, s := range input.ExistingSchemas {
		existingSchemas = append(existingSchemas, map[string]interface{}{
			"schema_id":   s.SchemaID,
			"schema_hash": s.SchemaHash,
		})
	}

	// Build AI request
	aiInput := aitopics.SchemaAnalyzeInput{
		ConfigID:        input.ConfigID,
		ConfigName:      input.ConfigName,
		Files:           files,
		ExistingSchemas: existingSchemas,
	}

	// Invoke AI
	log.Infof("SchemaAnalyzer: invoking AI to analyze schema for config %d", input.ConfigID)
	resp, err := client.InvokeSync(ctx, aitopics.TopicGithubSchemaAnalyze, aiInput)
	if err != nil {
		return nil, fmt.Errorf("AI invocation failed: %w", err)
	}

	if !resp.IsSuccess() {
		return &SchemaAnalysisOutput{
			Success: false,
			Error:   resp.Message,
		}, nil
	}

	// Parse response
	var output aitopics.SchemaAnalyzeOutput
	if err := resp.UnmarshalPayload(&output); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// Convert to internal format
	result := &SchemaAnalysisOutput{
		Success:       output.Success,
		Error:         output.Error,
		SchemaHash:    output.SchemaHash,
		SchemaMatched: output.SchemaMatched,
	}

	if output.MatchedSchemaID != nil {
		result.MatchedSchemaID = output.MatchedSchemaID
	}

	if output.Schema != nil {
		result.Schema = &SchemaDefinition{
			Name:            output.Schema.Name,
			DimensionFields: output.Schema.DimensionFields,
			MetricFields:    output.Schema.MetricFields,
			IsWideTable:     output.Schema.IsWideTable,
			DateColumns:     output.Schema.DateColumns,
		}
	}

	log.Infof("SchemaAnalyzer: analysis complete - matched=%v, hash=%s", result.SchemaMatched, result.SchemaHash[:8])
	return result, nil
}

// sampleToContent converts a file sample to a content string for AI
func (a *SchemaAnalyzer) sampleToContent(sample *FileSample) string {
	var sb strings.Builder

	switch sample.FileType {
	case "csv":
		// Write headers
		sb.WriteString(strings.Join(sample.Headers, ","))
		sb.WriteString("\n")
		// Write sample rows (max 5)
		maxRows := 5
		if len(sample.SampleRows) < maxRows {
			maxRows = len(sample.SampleRows)
		}
		for i := 0; i < maxRows; i++ {
			row := sample.SampleRows[i]
			values := make([]string, 0, len(sample.Headers))
			for _, h := range sample.Headers {
				if v, ok := row[h]; ok {
					values = append(values, fmt.Sprintf("%v", v))
				} else {
					values = append(values, "")
				}
			}
			sb.WriteString(strings.Join(values, ","))
			sb.WriteString("\n")
		}
	case "json":
		// Just write the sample rows as JSON
		maxRows := 5
		if len(sample.SampleRows) < maxRows {
			maxRows = len(sample.SampleRows)
		}
		data, _ := json.MarshalIndent(sample.SampleRows[:maxRows], "", "  ")
		sb.Write(data)
	case "markdown":
		// Write as markdown table
		sb.WriteString("| ")
		sb.WriteString(strings.Join(sample.Headers, " | "))
		sb.WriteString(" |\n")
		sb.WriteString("|")
		for range sample.Headers {
			sb.WriteString(" --- |")
		}
		sb.WriteString("\n")
		maxRows := 5
		if len(sample.SampleRows) < maxRows {
			maxRows = len(sample.SampleRows)
		}
		for i := 0; i < maxRows; i++ {
			row := sample.SampleRows[i]
			sb.WriteString("| ")
			values := make([]string, 0, len(sample.Headers))
			for _, h := range sample.Headers {
				if v, ok := row[h]; ok {
					values = append(values, fmt.Sprintf("%v", v))
				} else {
					values = append(values, "")
				}
			}
			sb.WriteString(strings.Join(values, " | "))
			sb.WriteString(" |\n")
		}
	default:
		// Default to CSV format
		sb.WriteString(strings.Join(sample.Headers, ","))
		sb.WriteString("\n")
	}

	return sb.String()
}

// PrepareFileSample prepares a file sample for schema analysis
func (a *SchemaAnalyzer) PrepareFileSample(file *PVCFile) (*FileSample, error) {
	// Parse file to get headers and sample data
	records, err := parseFileContent(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no records found in file")
	}

	// Extract headers from first record
	headers := make([]string, 0)
	for key := range records[0] {
		headers = append(headers, key)
	}
	sort.Strings(headers)

	// Infer column types
	columnTypes := inferColumnTypes(headers, records)

	// Limit sample rows
	sampleRows := records
	if len(sampleRows) > 10 {
		sampleRows = sampleRows[:10]
	}

	return &FileSample{
		FilePath:    file.Path,
		FileType:    detectFileType(file.Name),
		Headers:     headers,
		SampleRows:  sampleRows,
		ColumnTypes: columnTypes,
	}, nil
}

// inferColumnTypes infers the type of each column from sample data
func inferColumnTypes(headers []string, records []map[string]interface{}) map[string]string {
	types := make(map[string]string)

	for _, header := range headers {
		isNumeric := true
		isString := false

		for _, record := range records {
			if val, ok := record[header]; ok {
				switch val.(type) {
				case string:
					isString = true
					isNumeric = false
				case float64, int64, int, float32:
					// keep isNumeric = true
				default:
					isString = true
					isNumeric = false
				}
			}
			if isString {
				break
			}
		}

		if isNumeric {
			types[header] = "float"
		} else {
			types[header] = "string"
		}
	}

	return types
}

// ComputeSchemaHash computes a stable hash for a schema definition
// This should match the Python implementation in GitHubSchemaAnalyzerCrew
func ComputeSchemaHash(schema *SchemaDefinition) string {
	// Sort fields for stable hash
	dims := make([]string, len(schema.DimensionFields))
	copy(dims, schema.DimensionFields)
	sort.Strings(dims)

	metrics := make([]string, len(schema.MetricFields))
	copy(metrics, schema.MetricFields)
	sort.Strings(metrics)

	// Build hash input (same as Python implementation)
	hashInput := map[string]interface{}{
		"dimension_fields": dims,
		"metric_fields":    metrics,
		"is_wide_table":    schema.IsWideTable,
	}

	// Serialize with sorted keys
	data, _ := json.Marshal(hashInput)
	hash := sha256.Sum256(data)

	// Return first 32 hex characters (16 bytes) - same as Python md5 implementation
	return hex.EncodeToString(hash[:16])
}

// ConvertSchemaToDBModel converts a SchemaDefinition to database model
func ConvertSchemaToDBModel(schema *SchemaDefinition, configID int64) *model.GithubWorkflowMetricSchemas {
	// For wide tables, remove "date" from dimension_fields since it's now a time field
	filteredDimensions := schema.DimensionFields
	if schema.IsWideTable {
		filteredDimensions = make([]string, 0, len(schema.DimensionFields))
		for _, dim := range schema.DimensionFields {
			if dim != "date" {
				filteredDimensions = append(filteredDimensions, dim)
			}
		}
	}

	dimensionFields, _ := json.Marshal(filteredDimensions)
	metricFields, _ := json.Marshal(schema.MetricFields)
	dateColumns, _ := json.Marshal(schema.DateColumns)

	// Build fields array for backward compatibility
	fields := make([]map[string]interface{}, 0)
	for _, dim := range filteredDimensions {
		fields = append(fields, map[string]interface{}{
			"name":         dim,
			"type":         "string",
			"is_dimension": true,
		})
	}
	for _, metric := range schema.MetricFields {
		fields = append(fields, map[string]interface{}{
			"name":      metric,
			"type":      "float",
			"is_metric": true,
		})
	}
	fieldsJSON, _ := json.Marshal(fields)

	// Set TimeField for wide tables
	timeField := schema.TimeField
	if schema.IsWideTable && timeField == "" {
		timeField = "date" // Wide tables use date columns as time field
	}

	return &model.GithubWorkflowMetricSchemas{
		ConfigID:        configID,
		Name:            schema.Name,
		Fields:          model.ExtJSON(fieldsJSON),
		DimensionFields: model.ExtJSON(dimensionFields),
		MetricFields:    model.ExtJSON(metricFields),
		IsWideTable:     schema.IsWideTable,
		DateColumns:     model.ExtJSON(dateColumns),
		TimeField:       timeField,
		SchemaHash:      ComputeSchemaHash(schema),
		IsActive:        true,
		GeneratedBy:     database.SchemaGeneratedByAI,
	}
}
