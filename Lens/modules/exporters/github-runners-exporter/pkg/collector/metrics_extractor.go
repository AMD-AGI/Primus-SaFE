// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"encoding/json"
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

	// Check if using new column-based format
	if len(schema.Columns) > 0 {
		return e.extractWithColumnSchema(file, rows, schema)
	}

	// Legacy format handling
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

		// Extract timestamp if present (from wide table conversion)
		if ts, ok := row["__timestamp__"]; ok {
			if timestamp, isTime := ts.(time.Time); isTime {
				record.Timestamp = timestamp
			}
		}

		// Extract dimensions (skip internal markers and "date" for wide tables)
		for _, dim := range schema.DimensionFields {
			// Skip "date" dimension for wide tables - it's now in timestamp
			if schema.IsWideTable && dim == "date" {
				continue
			}
			if val, ok := row[dim]; ok {
				record.Dimensions[dim] = formatDimensionValue(val)
			}
		}

		// Extract metrics
		for _, metric := range schema.MetricFields {
			if val, ok := row[metric]; ok {
				if floatVal, ok := toFloat64WithError(val); ok == nil {
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

// extractWithColumnSchema extracts metrics using new column-based schema format
func (e *MetricsExtractor) extractWithColumnSchema(
	file *PVCFile,
	rows []map[string]interface{},
	schema *SchemaDefinition,
) ([]*MetricRecord, error) {
	var metrics []*MetricRecord

	// Compile date column pattern if present
	var dateColumnRegex *regexp.Regexp
	if schema.DateColumnPattern != "" {
		var err error
		dateColumnRegex, err = regexp.Compile(schema.DateColumnPattern)
		if err != nil {
			log.Warnf("Invalid date_column_pattern: %v", err)
		}
	}

	// Get all keys from first row to identify date columns
	var dateColumns []string
	if schema.IsWideTable && dateColumnRegex != nil && len(rows) > 0 {
		for key := range rows[0] {
			if dateColumnRegex.MatchString(key) {
				dateColumns = append(dateColumns, key)
			}
		}
	}

	for _, row := range rows {
		if schema.IsWideTable && len(dateColumns) > 0 {
			// Wide table: create a record for each date column
			baseDimensions := make(map[string]string)

			// Extract dimensions from configured columns
			for colName, colConfig := range schema.Columns {
				if colConfig.Skip {
					continue
				}
				if colConfig.Type == "dimension" {
					if val, ok := row[colName]; ok {
						baseDimensions[colName] = formatDimensionValue(val)
					}
				}
			}

			// Create a record for each date column
			for _, dateCol := range dateColumns {
				if val, ok := row[dateCol]; ok {
					floatVal, ok := toFloat64WithError(val)
					if ok != nil || floatVal == 0 {
						continue
					}

					record := &MetricRecord{
						SourceFile: file.Path,
						Dimensions: make(map[string]string),
						Metrics:    make(map[string]float64),
					}

					// Copy base dimensions
					for k, v := range baseDimensions {
						record.Dimensions[k] = v
					}

					// Set timestamp from column name
					if schema.DateColumnConfig != nil && schema.DateColumnConfig.TimeSource == "column_name" {
						record.Timestamp = parseDateColumn(dateCol)
					}

					// Set metric value
					metricKey := "value"
					if schema.DateColumnConfig != nil && schema.DateColumnConfig.MetricKey != "" {
						metricKey = schema.DateColumnConfig.MetricKey
					}
					record.Metrics[metricKey] = floatVal

					metrics = append(metrics, record)
				}
			}
		} else {
			// Long table: each row is a single metric record
			record := &MetricRecord{
				SourceFile: file.Path,
				Dimensions: make(map[string]string),
				Metrics:    make(map[string]float64),
			}

			for colName, colConfig := range schema.Columns {
				if colConfig.Skip {
					continue
				}

				val, ok := row[colName]
				if !ok {
					continue
				}

				if colConfig.Type == "dimension" {
					record.Dimensions[colName] = formatDimensionValue(val)
				} else if colConfig.Type == "metric" {
					metricKey := colConfig.MetricKey
					if metricKey == "" {
						metricKey = colName
					}
					if floatVal, ok := toFloat64WithError(val); ok == nil {
						record.Metrics[metricKey] = floatVal
					}
				}
			}

			if len(record.Metrics) > 0 {
				metrics = append(metrics, record)
			}
		}
	}

	return metrics, nil
}

// convertWideToLong converts wide table format to long format
// Wide: Op, Backend, 2025-12-28, 2025-12-29, 2025-12-30
// Long: Op, Backend, value (with timestamp set from date column name)
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
				if floatVal, ok := toFloat64WithError(val); ok != nil || floatVal == 0 {
					continue
				}

				newRow := make(map[string]interface{})

				// Copy dimensions (excluding "date" which is now a time field)
				for _, dim := range schema.DimensionFields {
					// Skip "date" dimension - it's now handled as timestamp
					if dim == "date" {
						continue
					}
					if v, exists := row[dim]; exists {
						newRow[dim] = v
					}
				}

				// Parse date column name to timestamp
				timestamp := parseDateColumn(dateCol)
				newRow["__timestamp__"] = timestamp // Internal marker for timestamp
				newRow["value"] = val

				longRows = append(longRows, newRow)
			}
		}
	}

	return longRows
}

// parseDateColumn parses a date column name into a time.Time
func parseDateColumn(dateCol string) time.Time {
	dateFormats := []string{
		"2006-01-02", // 2025-12-30
		"01/02/2006", // 12/30/2025
		"02-01-2006", // 30-12-2025
		"2006/01/02", // 2025/12/30
		"20060102",   // 20251230
	}

	for _, format := range dateFormats {
		if t, err := time.Parse(format, dateCol); err == nil {
			return t
		}
	}

	// Fallback to current time if parsing fails
	return time.Now()
}

// toFloat64WithError converts an interface value to float64 with error
func toFloat64WithError(val interface{}) (float64, error) {
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

// formatDimensionValue formats a value for dimension storage
// This handles the case where JSON unmarshals numbers as float64,
// which would result in scientific notation for large integers like dates (20260115)
// It also converts YYYYMMDD format integers to YYYY-MM-DD standard date format
func formatDimensionValue(val interface{}) string {
	switch v := val.(type) {
	case float64:
		// Check if the float is actually an integer
		if v == float64(int64(v)) {
			intVal := int64(v)
			// Check if it looks like a YYYYMMDD date (8 digits, valid range)
			if formatted := tryFormatAsDate(intVal); formatted != "" {
				return formatted
			}
			return strconv.FormatInt(intVal, 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		if v == float32(int32(v)) {
			intVal := int64(v)
			if formatted := tryFormatAsDate(intVal); formatted != "" {
				return formatted
			}
			return strconv.FormatInt(intVal, 10)
		}
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case int:
		if formatted := tryFormatAsDate(int64(v)); formatted != "" {
			return formatted
		}
		return fmt.Sprintf("%d", v)
	case int64:
		if formatted := tryFormatAsDate(v); formatted != "" {
			return formatted
		}
		return fmt.Sprintf("%d", v)
	case int32:
		if formatted := tryFormatAsDate(int64(v)); formatted != "" {
			return formatted
		}
		return fmt.Sprintf("%d", v)
	case int8, int16:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case string:
		// Also try to convert string like "20260115" to date format
		if len(v) == 8 {
			if intVal, err := strconv.ParseInt(v, 10, 64); err == nil {
				if formatted := tryFormatAsDate(intVal); formatted != "" {
					return formatted
				}
			}
		}
		return v
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// tryFormatAsDate attempts to format an integer as YYYY-MM-DD date
// Returns empty string if the value doesn't look like a valid YYYYMMDD date
func tryFormatAsDate(val int64) string {
	// Check if it's in YYYYMMDD range (19000101 to 21001231)
	if val < 19000101 || val > 21001231 {
		return ""
	}

	// Extract year, month, day
	year := val / 10000
	month := (val % 10000) / 100
	day := val % 100

	// Validate ranges
	if year < 1900 || year > 2100 {
		return ""
	}
	if month < 1 || month > 12 {
		return ""
	}
	if day < 1 || day > 31 {
		return ""
	}

	// Format as YYYY-MM-DD
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

// isDateColumn checks if a column name looks like a date
func isDateColumn(name string) bool {
	datePatterns := []*regexp.Regexp{
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`), // 2025-12-30
		regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`), // 12/30/2025
		regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`), // 30-12-2025
		regexp.MustCompile(`^\d{4}/\d{2}/\d{2}$`), // 2025/12/30
		regexp.MustCompile(`^\d{8}$`),             // 20251230
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
	schema := &SchemaDefinition{
		Name:              dbSchema.Name,
		Version:           int(dbSchema.Version),
		IsWideTable:       dbSchema.IsWideTable,
		DateColumnPattern: dbSchema.DateColumnPattern,
	}

	// Check if using new column-based format
	if dbSchema.ColumnDefinitions != nil && len(dbSchema.ColumnDefinitions) > 0 {
		var columns map[string]ColumnConfig
		colBytes, _ := json.Marshal(dbSchema.ColumnDefinitions)
		if err := json.Unmarshal(colBytes, &columns); err != nil {
			log.Warnf("Failed to parse columns, falling back to legacy format: %v", err)
		} else {
			schema.Columns = columns
		}

		if dbSchema.DateColumnConfig != nil && len(dbSchema.DateColumnConfig) > 0 {
			var dateColumnConfig DateColumnConfig
			dateColBytes, _ := json.Marshal(dbSchema.DateColumnConfig)
			if err := json.Unmarshal(dateColBytes, &dateColumnConfig); err != nil {
				log.Warnf("Failed to parse date_column_config: %v", err)
			} else {
				schema.DateColumnConfig = &dateColumnConfig
			}
		}

		// If we have columns, return early (new format)
		if schema.Columns != nil {
			return schema, nil
		}
	}

	// Legacy format
	var dimensionFields []string
	if err := dbSchema.DimensionFields.UnmarshalTo(&dimensionFields); err != nil {
		return nil, fmt.Errorf("failed to parse dimension_fields: %w", err)
	}
	schema.DimensionFields = dimensionFields

	var metricFields []string
	if err := dbSchema.MetricFields.UnmarshalTo(&metricFields); err != nil {
		return nil, fmt.Errorf("failed to parse metric_fields: %w", err)
	}
	schema.MetricFields = metricFields

	var dateColumns []string
	if err := dbSchema.DateColumns.UnmarshalTo(&dateColumns); err != nil {
		// DateColumns might not exist in old schemas, ignore error
		dateColumns = []string{}
	}
	schema.DateColumns = dateColumns
	schema.TimeField = dbSchema.TimeField

	return schema, nil
}
