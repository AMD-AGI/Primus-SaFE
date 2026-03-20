/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"encoding/csv"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var dateColumnPattern = regexp.MustCompile(`^\d{4}[-/]?\d{2}[-/]?\d{2}$`)

// ParseCSVToRows parses CSV content into flat JSONB rows.
// Handles both wide tables (date columns) and regular tables.
// Returns: list of row_data maps, source file name.
func ParseCSVToRows(data []byte) ([]map[string]interface{}, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, nil
	}

	headers := records[0]
	cleanHeaders := make([]string, len(headers))
	for i, h := range headers {
		cleanHeaders[i] = strings.TrimSpace(h)
	}

	if isWideTable(cleanHeaders) {
		return parseWideTable(cleanHeaders, records[1:])
	}
	return parseRegularTable(cleanHeaders, records[1:])
}

// isWideTable detects if CSV has date columns (wide table format).
func isWideTable(headers []string) bool {
	for _, h := range headers {
		if dateColumnPattern.MatchString(h) {
			return true
		}
	}
	return false
}

// parseWideTable converts wide table (date as column names) to long format rows.
// Input:  #, Op, GPU, Framework, Stage, 2026-02-05, 2026-02-06
//         1, Attention, MI325, PyTorch, Fwd, 255.36, 260.12
// Output: {Op: Attention, GPU: MI325, Framework: PyTorch, Stage: Fwd, date: 2026-02-05, value: 255.36}
//         {Op: Attention, GPU: MI325, Framework: PyTorch, Stage: Fwd, date: 2026-02-06, value: 260.12}
func parseWideTable(headers []string, rows [][]string) ([]map[string]interface{}, error) {
	dimCols := []int{}
	dateCols := []int{}

	for i, h := range headers {
		if h == "#" || h == "" {
			continue
		}
		if dateColumnPattern.MatchString(h) {
			dateCols = append(dateCols, i)
		} else {
			dimCols = append(dimCols, i)
		}
	}

	var result []map[string]interface{}
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if row[0] == "#" || (len(row) > 1 && row[1] == headers[1]) {
			continue
		}

		dims := map[string]interface{}{}
		for _, ci := range dimCols {
			if ci < len(row) {
				dims[headers[ci]] = parseValue(row[ci])
			}
		}

		for _, ci := range dateCols {
			if ci < len(row) && strings.TrimSpace(row[ci]) != "" {
				rowData := make(map[string]interface{})
				for k, v := range dims {
					rowData[k] = v
				}
				rowData["date"] = headers[ci]
				rowData["value"] = parseFloat(row[ci])
				result = append(result, rowData)
			}
		}
	}
	return result, nil
}

// parseRegularTable parses standard CSV into flat rows.
func parseRegularTable(headers []string, rows [][]string) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if len(row) > 0 && row[0] == headers[0] {
			continue
		}

		rowData := make(map[string]interface{})
		for i, h := range headers {
			if h == "" || h == "#" {
				continue
			}
			if i < len(row) {
				rowData[h] = parseValue(row[i])
			}
		}
		if len(rowData) > 0 {
			result = append(result, rowData)
		}
	}
	return result, nil
}

func parseValue(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

func parseFloat(s string) interface{} {
	s = strings.TrimSpace(s)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// ParseJSONToRows parses JSON content (object or array) into flat rows.
func ParseJSONToRows(data []byte) ([]map[string]interface{}, error) {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	switch v := raw.(type) {
	case []interface{}:
		var rows []map[string]interface{}
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				rows = append(rows, m)
			}
		}
		return rows, nil
	case map[string]interface{}:
		return []map[string]interface{}{v}, nil
	}
	return nil, nil
}

// ParseFileToRows auto-detects file format (CSV or JSON) and returns rows.
func ParseFileToRows(data []byte, fileName string) ([]map[string]interface{}, error) {
	lower := strings.ToLower(fileName)
	if strings.HasSuffix(lower, ".csv") {
		return ParseCSVToRows(data)
	}
	if strings.HasSuffix(lower, ".json") {
		return ParseJSONToRows(data)
	}
	if rows, err := ParseCSVToRows(data); err == nil && len(rows) > 0 {
		return rows, nil
	}
	return ParseJSONToRows(data)
}
