/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestIsValidDatasetType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid type sft",
			input:    "sft",
			expected: true,
		},
		{
			name:     "valid type dpo",
			input:    "dpo",
			expected: true,
		},
		{
			name:     "valid type pretrain",
			input:    "pretrain",
			expected: true,
		},
		{
			name:     "valid type rlhf",
			input:    "rlhf",
			expected: true,
		},
		{
			name:     "valid type inference",
			input:    "inference",
			expected: true,
		},
		{
			name:     "valid type evaluation",
			input:    "evaluation",
			expected: true,
		},
		{
			name:     "valid type other",
			input:    "other",
			expected: true,
		},
		{
			name:     "invalid type",
			input:    "invalid",
			expected: false,
		},
		{
			name:     "empty type",
			input:    "",
			expected: false,
		},
		{
			name:     "case sensitive - uppercase",
			input:    "SFT",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDatasetType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDatasetContentType(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected string
	}{
		{
			name:     "json file",
			filePath: "data.json",
			expected: "application/json",
		},
		{
			name:     "jsonl file",
			filePath: "data.jsonl",
			expected: "application/jsonl",
		},
		{
			name:     "txt file",
			filePath: "readme.txt",
			expected: "text/plain; charset=utf-8",
		},
		// Note: CSV content type varies by platform (Windows returns "application/vnd.ms-excel")
		// Skipping CSV test for cross-platform compatibility
		{
			name:     "yaml file",
			filePath: "config.yaml",
			expected: "application/yaml",
		},
		{
			name:     "yml file",
			filePath: "config.yml",
			expected: "application/yaml",
		},
		{
			name:     "unknown extension",
			filePath: "data.xyz",
			expected: "text/plain",
		},
		{
			name:     "no extension",
			filePath: "README",
			expected: "text/plain",
		},
		{
			name:     "uppercase extension",
			filePath: "data.JSON",
			expected: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDatasetContentType(tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToDatasetResponse(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name     string
		dataset  *dbclient.Dataset
		validate func(t *testing.T, resp DatasetResponse)
	}{
		{
			name: "basic dataset conversion",
			dataset: &dbclient.Dataset{
				DatasetId:    "dataset-123",
				DisplayName:  "Test Dataset",
				Description:  "Test Description",
				DatasetType:  "sft",
				Status:       "Ready",
				S3Path:       "datasets/dataset-123/",
				TotalSize:    1024 * 1024, // 1MB
				FileCount:    5,
				Workspace:    "ws-1",
				UserId:       "user-1",
				UserName:     "Test User",
				CreationTime: pq.NullTime{Time: now, Valid: true},
				UpdateTime:   pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, resp DatasetResponse) {
				assert.Equal(t, "dataset-123", resp.DatasetId)
				assert.Equal(t, "Test Dataset", resp.DisplayName)
				assert.Equal(t, "Test Description", resp.Description)
				assert.Equal(t, "sft", resp.DatasetType)
				assert.Equal(t, "Ready", resp.Status)
				assert.Equal(t, "datasets/dataset-123/", resp.S3Path)
				assert.Equal(t, int64(1024*1024), resp.TotalSize)
				assert.Equal(t, "1.00 MB", resp.TotalSizeStr)
				assert.Equal(t, 5, resp.FileCount)
				assert.Equal(t, "ws-1", resp.Workspace)
				assert.Equal(t, "user-1", resp.UserId)
				assert.Equal(t, "Test User", resp.UserName)
				assert.NotNil(t, resp.CreationTime)
				assert.NotNil(t, resp.UpdateTime)
			},
		},
		{
			name: "dataset with local paths",
			dataset: &dbclient.Dataset{
				DatasetId:   "dataset-456",
				DisplayName: "Dataset with LocalPaths",
				DatasetType: "dpo",
				Status:      "Ready",
				TotalSize:   1024,
				LocalPaths:  `[{"workspace":"ws-1","path":"/pfs/datasets/test","status":"Ready"},{"workspace":"ws-2","path":"/pfs/datasets/test","status":"Downloading"}]`,
			},
			validate: func(t *testing.T, resp DatasetResponse) {
				assert.Equal(t, "dataset-456", resp.DatasetId)
				assert.Len(t, resp.LocalPaths, 2)
				assert.Equal(t, "ws-1", resp.LocalPaths[0].Workspace)
				assert.Equal(t, "Ready", resp.LocalPaths[0].Status)
				assert.Equal(t, "ws-2", resp.LocalPaths[1].Workspace)
				assert.Equal(t, "Downloading", resp.LocalPaths[1].Status)
				assert.Equal(t, "1/2 workspaces completed", resp.StatusMessage)
			},
		},
		{
			name: "dataset with invalid local paths JSON",
			dataset: &dbclient.Dataset{
				DatasetId:   "dataset-789",
				DisplayName: "Dataset with Invalid JSON",
				DatasetType: "sft",
				Status:      "Pending",
				TotalSize:   512,
				LocalPaths:  "invalid json",
			},
			validate: func(t *testing.T, resp DatasetResponse) {
				assert.Equal(t, "dataset-789", resp.DatasetId)
				assert.Nil(t, resp.LocalPaths)
				assert.Empty(t, resp.StatusMessage)
			},
		},
		{
			name: "dataset with empty local paths",
			dataset: &dbclient.Dataset{
				DatasetId:   "dataset-empty",
				DisplayName: "Dataset with Empty LocalPaths",
				DatasetType: "pretrain",
				Status:      "Pending",
				TotalSize:   2048,
				LocalPaths:  "[]",
			},
			validate: func(t *testing.T, resp DatasetResponse) {
				assert.Equal(t, "dataset-empty", resp.DatasetId)
				assert.Empty(t, resp.LocalPaths)
				assert.Empty(t, resp.StatusMessage)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToDatasetResponse(tt.dataset)
			tt.validate(t, result)
		})
	}
}

func TestGetDatasetTypeDescriptions(t *testing.T) {
	descriptions := GetDatasetTypeDescriptions()

	// Verify all dataset types have descriptions
	assert.Len(t, descriptions, 7)

	// Verify each type has required fields
	for _, typeInfo := range descriptions {
		assert.NotEmpty(t, typeInfo.Name, "DatasetTypeInfo should have a name")
		assert.NotEmpty(t, typeInfo.Description, "DatasetTypeInfo should have a description")
		assert.NotNil(t, typeInfo.Schema, "DatasetTypeInfo should have a schema")
	}

	// Verify all valid types are covered
	coveredTypes := make(map[string]bool)
	for _, typeInfo := range descriptions {
		coveredTypes[typeInfo.Name] = true
	}

	for typeName := range DatasetTypes {
		assert.True(t, coveredTypes[string(typeName)], "Type %s should be in DatasetTypeDescriptions", typeName)
	}
}

func TestDatasetTypes(t *testing.T) {
	// Test that all expected types exist
	expectedTypes := []string{"sft", "dpo", "pretrain", "rlhf", "inference", "evaluation", "other"}
	for _, typeName := range expectedTypes {
		dt, ok := DatasetTypes[DatasetType(typeName)]
		assert.True(t, ok, "DatasetTypes should contain %s", typeName)
		assert.NotEmpty(t, dt.Description, "DatasetType.Description should not be empty")
		assert.NotNil(t, dt.Schema, "DatasetType.Schema should not be nil")
	}
}

func TestDatasetConstants(t *testing.T) {
	// Test S3 prefix constant
	assert.Equal(t, "datasets", DatasetS3Prefix)

	// Test S3 secret constant
	assert.Equal(t, "primus-safe-s3", DatasetS3Secret)
}

func TestMaxPreviewSize(t *testing.T) {
	// Test that MaxPreviewSize is 100KB
	assert.Equal(t, int64(100*1024), int64(MaxPreviewSize))
}

func TestIsDatasetEnabled(t *testing.T) {
	// Test with nil s3Client
	h := &Handler{
		s3Client: nil,
	}
	assert.False(t, h.IsDatasetEnabled())
}

