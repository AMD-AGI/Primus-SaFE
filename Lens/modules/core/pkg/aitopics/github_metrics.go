// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitopics

// ========== github.metrics.extract ==========

// ExtractMetricsInput is the input payload for metrics extraction from files
type ExtractMetricsInput struct {
	// ConfigID is the configuration ID for context
	ConfigID int64 `json:"config_id"`

	// ConfigName is the configuration name for context
	ConfigName string `json:"config_name,omitempty"`

	// Files contains the file contents to extract metrics from
	Files []FileContent `json:"files"`

	// ExistingSchema is the existing schema to use for extraction (optional)
	// If provided, the extractor will use this schema to guide extraction
	// If not provided, the extractor will generate a new schema
	ExistingSchema *MetricSchema `json:"existing_schema,omitempty"`

	// CustomPrompt allows user-defined extraction instructions (optional)
	CustomPrompt string `json:"custom_prompt,omitempty"`

	// Options contains additional extraction options
	Options *ExtractMetricsOptions `json:"options,omitempty"`
}

// FileContent represents a file and its content
type FileContent struct {
	// Path is the file path
	Path string `json:"path"`

	// Name is the file name
	Name string `json:"name"`

	// FileType is the detected file type (json, csv, markdown, etc.)
	FileType string `json:"file_type"`

	// Content is the file content as string
	Content string `json:"content"`

	// SizeBytes is the file size in bytes
	SizeBytes int64 `json:"size_bytes,omitempty"`
}

// ExtractMetricsOptions contains optional settings for extraction
type ExtractMetricsOptions struct {
	// IncludeRawData includes the raw parsed data in the output
	IncludeRawData bool `json:"include_raw_data,omitempty"`

	// IncludeExplanation includes AI explanation of extraction decisions
	IncludeExplanation bool `json:"include_explanation,omitempty"`

	// MaxRecordsPerFile limits the number of records extracted per file
	MaxRecordsPerFile int `json:"max_records_per_file,omitempty"`

	// GenerateSchemaOnly only generates schema without extracting metrics
	GenerateSchemaOnly bool `json:"generate_schema_only,omitempty"`
}

// MetricSchema represents a schema definition for metrics extraction
type MetricSchema struct {
	// Name is the schema name
	Name string `json:"name"`

	// Version is the schema version
	Version int32 `json:"version,omitempty"`

	// Fields contains all field definitions
	Fields []SchemaField `json:"fields"`

	// DimensionFields are field names used as dimensions (for grouping)
	DimensionFields []string `json:"dimension_fields"`

	// MetricFields are field names containing numeric metrics
	MetricFields []string `json:"metric_fields"`
}

// SchemaField represents a single field in the schema
type SchemaField struct {
	// Name is the field name
	Name string `json:"name"`

	// Type is the field data type (string, int, float, bool)
	Type string `json:"type"`

	// Unit is the measurement unit (e.g., "tokens/s", "ms", "GB")
	Unit string `json:"unit,omitempty"`

	// Description describes what this field represents
	Description string `json:"description,omitempty"`

	// IsDimension indicates if this field should be used as a dimension
	IsDimension bool `json:"is_dimension,omitempty"`

	// IsMetric indicates if this field is a numeric metric
	IsMetric bool `json:"is_metric,omitempty"`
}

// ExtractMetricsOutput is the output payload from metrics extraction
type ExtractMetricsOutput struct {
	// Schema is the generated or used schema
	// This is always returned, even if an existing schema was provided
	Schema *MetricSchema `json:"schema"`

	// SchemaGenerated indicates if a new schema was generated
	SchemaGenerated bool `json:"schema_generated"`

	// Metrics contains the extracted metric records
	Metrics []ExtractedMetric `json:"metrics"`

	// FilesProcessed is the number of files successfully processed
	FilesProcessed int `json:"files_processed"`

	// TotalRecords is the total number of metric records extracted
	TotalRecords int `json:"total_records"`

	// Errors contains any extraction errors per file
	Errors []ExtractionError `json:"errors,omitempty"`

	// Explanation provides AI reasoning about the extraction (if requested)
	Explanation string `json:"explanation,omitempty"`
}

// ExtractedMetric represents a single extracted metric record
type ExtractedMetric struct {
	// SourceFile is the file this metric was extracted from
	SourceFile string `json:"source_file"`

	// Dimensions contains dimension field values
	Dimensions map[string]interface{} `json:"dimensions"`

	// Metrics contains metric field values (numeric)
	Metrics map[string]interface{} `json:"metrics"`

	// RawData contains the original parsed data (if requested)
	RawData map[string]interface{} `json:"raw_data,omitempty"`
}

// ExtractionError represents an error during extraction
type ExtractionError struct {
	// FilePath is the file that caused the error
	FilePath string `json:"file_path"`

	// Error is the error message
	Error string `json:"error"`

	// Recoverable indicates if the extraction can continue
	Recoverable bool `json:"recoverable"`
}

