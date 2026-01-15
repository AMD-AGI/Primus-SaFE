/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dataset_handlers

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestIsValidDatasetType(t *testing.T) {
	tests := []struct {
		name        string
		datasetType string
		expected    bool
	}{
		{
			name:        "valid sft type",
			datasetType: DatasetTypeSFT,
			expected:    true,
		},
		{
			name:        "valid dpo type",
			datasetType: DatasetTypeDPO,
			expected:    true,
		},
		{
			name:        "valid pretrain type",
			datasetType: DatasetTypePretrain,
			expected:    true,
		},
		{
			name:        "valid rlhf type",
			datasetType: DatasetTypeRLHF,
			expected:    true,
		},
		{
			name:        "valid inference type",
			datasetType: DatasetTypeInference,
			expected:    true,
		},
		{
			name:        "valid evaluation type",
			datasetType: DatasetTypeEvaluation,
			expected:    true,
		},
		{
			name:        "valid other type",
			datasetType: DatasetTypeOther,
			expected:    true,
		},
		{
			name:        "invalid type",
			datasetType: "invalid",
			expected:    false,
		},
		{
			name:        "empty type",
			datasetType: "",
			expected:    false,
		},
		{
			name:        "case sensitive - uppercase",
			datasetType: "SFT",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDatasetType(tt.datasetType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetContentType(t *testing.T) {
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
			expected: "text/plain",
		},
		{
			name:     "csv file",
			filePath: "data.csv",
			expected: "text/csv",
		},
		{
			name:     "markdown file",
			filePath: "README.md",
			expected: "text/markdown",
		},
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
			filePath: "Makefile",
			expected: "text/plain",
		},
		{
			name:     "uppercase extension",
			filePath: "data.JSON",
			expected: "application/json",
		},
		{
			name:     "file with path",
			filePath: "/path/to/data.json",
			expected: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getContentType(tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected string
	}{
		{
			name:     "bytes",
			size:     512,
			expected: "512 B",
		},
		{
			name:     "kilobytes",
			size:     1024,
			expected: "1.00 KB",
		},
		{
			name:     "kilobytes with decimals",
			size:     2560,
			expected: "2.50 KB",
		},
		{
			name:     "megabytes",
			size:     1024 * 1024,
			expected: "1.00 MB",
		},
		{
			name:     "megabytes with decimals",
			size:     1536 * 1024,
			expected: "1.50 MB",
		},
		{
			name:     "gigabytes",
			size:     1024 * 1024 * 1024,
			expected: "1.00 GB",
		},
		{
			name:     "terabytes",
			size:     1024 * 1024 * 1024 * 1024,
			expected: "1.00 TB",
		},
		{
			name:     "zero bytes",
			size:     0,
			expected: "0 B",
		},
		{
			name:     "large file 50GB",
			size:     50 * 1024 * 1024 * 1024,
			expected: "50.00 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatFileSize(tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetNfsPathFromWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		workspace *v1.Workspace
		expected  string
	}{
		{
			name: "workspace with PFS volume",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{
							Type:      v1.PFS,
							MountPath: "/pfs/data",
						},
					},
				},
			},
			expected: "/pfs/data",
		},
		{
			name: "workspace with multiple volumes - PFS prioritized",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{
							Type:      v1.HOSTPATH,
							MountPath: "/hostpath/data",
						},
						{
							Type:      v1.PFS,
							MountPath: "/pfs/data",
						},
					},
				},
			},
			expected: "/pfs/data",
		},
		{
			name: "workspace with only hostpath volume",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{
						{
							Type:      v1.HOSTPATH,
							MountPath: "/hostpath/data",
						},
					},
				},
			},
			expected: "/hostpath/data",
		},
		{
			name: "workspace with no volumes",
			workspace: &v1.Workspace{
				Spec: v1.WorkspaceSpec{
					Volumes: []v1.WorkspaceVolume{},
				},
			},
			expected: "",
		},
		{
			name:      "nil workspace volumes",
			workspace: &v1.Workspace{},
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getNfsPathFromWorkspace(tt.workspace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUniqueDownloadPaths(t *testing.T) {
	tests := []struct {
		name        string
		workspaces  []v1.Workspace
		expectedLen int
	}{
		{
			name: "single workspace",
			workspaces: []v1.Workspace{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/ws1"},
						},
					},
				},
			},
			expectedLen: 1,
		},
		{
			name: "multiple workspaces with different paths",
			workspaces: []v1.Workspace{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/ws1"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-2"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/ws2"},
						},
					},
				},
			},
			expectedLen: 2,
		},
		{
			name: "multiple workspaces with same path - deduplicated",
			workspaces: []v1.Workspace{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/shared"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-2"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/shared"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-3"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/shared"},
						},
					},
				},
			},
			expectedLen: 1,
		},
		{
			name: "workspaces with no volumes are skipped",
			workspaces: []v1.Workspace{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
					Spec: v1.WorkspaceSpec{
						Volumes: []v1.WorkspaceVolume{
							{Type: v1.PFS, MountPath: "/pfs/ws1"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ws-2"},
					Spec:       v1.WorkspaceSpec{},
				},
			},
			expectedLen: 1,
		},
		{
			name:        "empty workspaces",
			workspaces:  []v1.Workspace{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUniqueDownloadPaths(tt.workspaces)
			assert.Equal(t, tt.expectedLen, len(result))
		})
	}
}

func TestConvertToDatasetResponse(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		dataset  *dbclient.Dataset
		validate func(*testing.T, DatasetResponse)
	}{
		{
			name: "basic dataset conversion",
			dataset: &dbclient.Dataset{
				DatasetId:    "dataset-abc123",
				DisplayName:  "Test Dataset",
				Description:  "Test description",
				DatasetType:  DatasetTypeSFT,
				Status:       "Ready",
				S3Path:       "datasets/dataset-abc123/",
				TotalSize:    1024 * 1024, // 1MB
				FileCount:    5,
				Message:      "",
				Workspace:    "ws-1",
				UserId:       "user-123",
				UserName:     "Test User",
				CreationTime: pq.NullTime{Time: now, Valid: true},
				UpdateTime:   pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, resp DatasetResponse) {
				assert.Equal(t, "dataset-abc123", resp.DatasetId)
				assert.Equal(t, "Test Dataset", resp.DisplayName)
				assert.Equal(t, "Test description", resp.Description)
				assert.Equal(t, DatasetTypeSFT, resp.DatasetType)
				assert.Equal(t, "Ready", resp.Status)
				assert.Equal(t, "1.00 MB", resp.TotalSizeStr)
				assert.Equal(t, 5, resp.FileCount)
				assert.Equal(t, "ws-1", resp.Workspace)
				assert.Equal(t, "user-123", resp.UserId)
				assert.NotNil(t, resp.CreationTime)
				assert.NotNil(t, resp.UpdateTime)
			},
		},
		{
			name: "dataset with local paths",
			dataset: &dbclient.Dataset{
				DatasetId:   "dataset-xyz789",
				DisplayName: "Multi-workspace Dataset",
				DatasetType: DatasetTypeDPO,
				Status:      "Ready",
				S3Path:      "datasets/dataset-xyz789/",
				TotalSize:   2048,
				FileCount:   1,
				Workspace:   "",
				UserId:      "user-456",
				UserName:    "Another User",
				LocalPaths:  `[{"workspace":"ws-1","path":"/pfs/ws1/datasets/test","status":"Ready"},{"workspace":"ws-2","path":"/pfs/ws2/datasets/test","status":"Downloading"}]`,
			},
			validate: func(t *testing.T, resp DatasetResponse) {
				assert.Equal(t, "dataset-xyz789", resp.DatasetId)
				assert.Equal(t, "", resp.Workspace) // Public dataset
				assert.Len(t, resp.LocalPaths, 2)
				assert.Equal(t, "1/2 workspaces completed", resp.StatusMessage)
				assert.Equal(t, "ws-1", resp.LocalPaths[0].Workspace)
				assert.Equal(t, "Ready", resp.LocalPaths[0].Status)
				assert.Equal(t, "ws-2", resp.LocalPaths[1].Workspace)
				assert.Equal(t, "Downloading", resp.LocalPaths[1].Status)
			},
		},
		{
			name: "dataset with invalid local paths JSON",
			dataset: &dbclient.Dataset{
				DatasetId:   "dataset-invalid",
				DisplayName: "Invalid LocalPaths",
				DatasetType: DatasetTypeSFT,
				Status:      "Ready",
				TotalSize:   100,
				FileCount:   1,
				LocalPaths:  "invalid json",
			},
			validate: func(t *testing.T, resp DatasetResponse) {
				assert.Equal(t, "dataset-invalid", resp.DatasetId)
				assert.Nil(t, resp.LocalPaths)
				assert.Empty(t, resp.StatusMessage)
			},
		},
		{
			name: "dataset with null times",
			dataset: &dbclient.Dataset{
				DatasetId:    "dataset-nulltime",
				DisplayName:  "No Times",
				DatasetType:  DatasetTypeSFT,
				Status:       "Ready",
				TotalSize:    100,
				FileCount:    1,
				CreationTime: pq.NullTime{Valid: false},
				UpdateTime:   pq.NullTime{Valid: false},
			},
			validate: func(t *testing.T, resp DatasetResponse) {
				assert.Equal(t, "dataset-nulltime", resp.DatasetId)
				assert.Nil(t, resp.CreationTime)
				assert.Nil(t, resp.UpdateTime)
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

func TestDatasetTypeDescriptions(t *testing.T) {
	// Verify all dataset types have descriptions
	assert.Len(t, DatasetTypeDescriptions, 7)

	// Verify each type has required fields
	for _, typeInfo := range DatasetTypeDescriptions {
		assert.NotEmpty(t, typeInfo.Name, "DatasetTypeInfo should have a name")
		assert.NotEmpty(t, typeInfo.Description, "DatasetTypeInfo should have a description")
		assert.NotNil(t, typeInfo.Schema, "DatasetTypeInfo should have a schema")
	}

	// Verify all valid types are covered
	coveredTypes := make(map[string]bool)
	for _, typeInfo := range DatasetTypeDescriptions {
		coveredTypes[typeInfo.Name] = true
	}

	for typeName := range ValidDatasetTypes {
		assert.True(t, coveredTypes[typeName], "Type %s should be in DatasetTypeDescriptions", typeName)
	}
}
