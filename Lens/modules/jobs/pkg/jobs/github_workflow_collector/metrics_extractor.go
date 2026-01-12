// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package github_workflow_collector

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// MetricsExtractor extracts metrics from files based on schema definition
// This runs in Go without LLM calls, saving tokens
type MetricsExtractor struct{}

// NewMetricsExtractor creates a new MetricsExtractor instance
func NewMetricsExtractor() *MetricsExtractor {
	return &MetricsExtractor{}
}

// MetricRecord represents a single extracted metric record
type MetricRecord struct {
	SourceFile string
	Dimensions map[string]string
	Metrics    map[string]float64
	Timestamp  time.Time
}

// ExtractMetrics parses files and extracts metrics according to schema
func (e *MetricsExtractor) ExtractMetrics(
	files []*PVCFile,
	schema *SchemaDefinition,
) ([]*MetricRecord, error) {
	var allMetrics []*MetricRecord

	for _, file := range files {
		metrics, err := e.extractFromFile(file, schema)
		if err != nil {
			log.Warnf("MetricsExtractor: failed to extract from %s: %v", file.Path, err)
			continue
		}
		allMetrics = append(allMetrics, metrics...)
	}

	log.Infof("MetricsExtractor: extracted %d metrics from %d files", len(allMetrics), len(files))
	return allMetrics, nil
}

// extractFromFile extracts metrics from a single file
func (e *MetricsExtractor) extractFromFile(
	file *PVCFile,
	schema *SchemaDefinition,
) ([]*MetricRecord, error) {
	// Parse file based on type
	rows, err := parseFileContent(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// Handle wide table conversion
	if schema.IsWideTable {
		rows = e.convertWideToLong(rows, schema)
	}

	// Extract metrics
	var metrics []*MetricRecord
	for _, row := range rows {
		record := &MetricRecord{
			SourceFile: file.Path,
			Dimensions: make(map[string]string),
			Metrics:    make(map[string]float64),
		}

		// Extract dimensions
		for _, dim := range schema.DimensionFields {
			if val, ok := row[dim]; ok {
				record.Dimensions[dim] = fmt.Sprintf("%v", val)
			}
		}

		// Extract metrics
		for _, metric := range schema.MetricFields {
			if val, ok := row[metric]; ok {
				if floatVal, err := toFloat64(val); err == nil {
					record.Metrics[metric] = floatVal
				}
			}
		}

		// Only add if we have at least one metric
		if len(record.Metrics) > 0 {
			metrics = append(metrics, record)
		}
	}

	return metrics, nil
}

// convertWideToLong converts wide table format to long format
// Wide: Op, Backend, 2025-12-28, 2025-12-29, 2025-12-30
// Long: Op, Backend, date, value
func (e *MetricsExtractor) convertWideToLong(
	rows []map[string]interface{},
	schema *SchemaDefinition,
) []map[string]interface{} {
	if len(schema.DateColumns) == 0 {
		// No date columns detected, try to detect from row keys
		if len(rows) > 0 {
			for key := range rows[0] {
				if isDateColumn(key) {
					schema.DateColumns = append(schema.DateColumns, key)
				}
			}
		}
	}

	if len(schema.DateColumns) == 0 {
		return rows
	}

	var longRows []map[string]interface{}

	for _, row := range rows {
		// For each date column, create a new row
		for _, dateCol := range schema.DateColumns {
			if val, ok := row[dateCol]; ok {
				// Skip empty or zero values
				if floatVal, err := toFloat64(val); err != nil || floatVal == 0 {
					continue
				}

				newRow := make(map[string]interface{})

				// Copy dimensions
				for _, dim := range schema.DimensionFields {
					if v, exists := row[dim]; exists {
						newRow[dim] = v
					}
				}

				// Add date dimension and value metric
				newRow["date"] = dateCol
				newRow["value"] = val

				longRows = append(longRows, newRow)
			}
		}
	}

	return longRows
}

// toFloat64 converts an interface value to float64
func toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}

// isDateColumn checks if a column name looks like a date
func isDateColumn(name string) bool {
	datePatterns := []*regexp.Regexp{
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),      // 2025-12-30
		regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`),      // 12/30/2025
		regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`),      // 30-12-2025
		regexp.MustCompile(`^\d{4}/\d{2}/\d{2}$`),      // 2025/12/30
		regexp.MustCompile(`^\d{8}$`),                  // 20251230
	}

	for _, pattern := range datePatterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}

// ConvertToDBMetrics converts MetricRecords to database model format
func (e *MetricsExtractor) ConvertToDBMetrics(
	configID int64,
	runID int64,
	schemaID int64,
	timestamp time.Time,
	records []*MetricRecord,
) []*model.GithubWorkflowMetrics {
	result := make([]*model.GithubWorkflowMetrics, 0, len(records))

	for _, record := range records {
		// Convert dimensions map to ExtType
		dims := make(model.ExtType)
		for k, v := range record.Dimensions {
			dims[k] = v
		}

		// Convert metrics map to ExtType
		metrics := make(model.ExtType)
		for k, v := range record.Metrics {
			metrics[k] = v
		}

		// Use record timestamp if available, otherwise use provided timestamp
		ts := timestamp
		if !record.Timestamp.IsZero() {
			ts = record.Timestamp
		}

		dbMetric := &model.GithubWorkflowMetrics{
			ConfigID:   configID,
			RunID:      runID,
			SchemaID:   schemaID,
			Timestamp:  ts,
			SourceFile: record.SourceFile,
			Dimensions: dims,
			Metrics:    metrics,
		}

		result = append(result, dbMetric)
	}

	return result
}

// ExtractMetricsFromDBSchema extracts metrics using a database schema model
func (e *MetricsExtractor) ExtractMetricsFromDBSchema(
	files []*PVCFile,
	dbSchema *model.GithubWorkflowMetricSchemas,
) ([]*MetricRecord, error) {
	// Convert DB schema to SchemaDefinition
	schema, err := ConvertDBSchemaToDefinition(dbSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema: %w", err)
	}

	return e.ExtractMetrics(files, schema)
}

// ConvertDBSchemaToDefinition converts a database schema model to SchemaDefinition
func ConvertDBSchemaToDefinition(dbSchema *model.GithubWorkflowMetricSchemas) (*SchemaDefinition, error) {
	var dimensionFields []string
	if err := dbSchema.DimensionFields.UnmarshalTo(&dimensionFields); err != nil {
		return nil, fmt.Errorf("failed to parse dimension_fields: %w", err)
	}

	var metricFields []string
	if err := dbSchema.MetricFields.UnmarshalTo(&metricFields); err != nil {
		return nil, fmt.Errorf("failed to parse metric_fields: %w", err)
	}

	var dateColumns []string
	if err := dbSchema.DateColumns.UnmarshalTo(&dateColumns); err != nil {
		// DateColumns might not exist in old schemas, ignore error
		dateColumns = []string{}
	}

	return &SchemaDefinition{
		Name:            dbSchema.Name,
		DimensionFields: dimensionFields,
		MetricFields:    metricFields,
		IsWideTable:     dbSchema.IsWideTable,
		DateColumns:     dateColumns,
	}, nil
}
