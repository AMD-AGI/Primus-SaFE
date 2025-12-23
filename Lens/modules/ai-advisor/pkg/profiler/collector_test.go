package profiler

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/profiler/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCollector(t *testing.T) {
	config := &CollectorConfig{
		AutoCollect: true,
		Interval:    300,
	}

	backend := newMockStorageBackend()
	collector, err := NewCollector(config, backend, "http://node-exporter:8080")

	require.NoError(t, err)
	assert.NotNil(t, collector)
	assert.Equal(t, config, collector.config)
	assert.NotNil(t, collector.nodeClient)
}

func TestCollector_ShouldCollectFile(t *testing.T) {
	config := &CollectorConfig{
		AutoCollect: true,
		Interval:    300,
		Filter: &FilterConfig{
			MinConfidence:  "medium",
			MaxFileSize:    100 * 1024 * 1024, // 100MB
			AllowedTypes:   []string{"chrome_trace", "pytorch_trace", "stack_trace"},
			RequireFramework: true,
		},
	}

	backend := newMockStorageBackend()
	collector, _ := NewCollector(config, backend, "http://node-exporter:8080")

	tests := []struct {
		name       string
		framework  string
		file       *ProfilerFileInfo
		expected   bool
	}{
		{
			name:      "High confidence file - always collect",
			framework: "pytorch",
			file: &ProfilerFileInfo{
				FileName:   "profiler.json",
				FileType:   "chrome_trace",
				FileSize:   10 * 1024 * 1024,
				Confidence: "high",
			},
			expected: true,
		},
		{
			name:      "Medium confidence with PyTorch framework",
			framework: "pytorch",
			file: &ProfilerFileInfo{
				FileName:   "profiler.json",
				FileType:   "chrome_trace",
				FileSize:   10 * 1024 * 1024,
				Confidence: "medium",
			},
			expected: true,
		},
		{
			name:      "Low confidence - skip",
			framework: "pytorch",
			file: &ProfilerFileInfo{
				FileName:   "profiler.json",
				FileType:   "chrome_trace",
				FileSize:   10 * 1024 * 1024,
				Confidence: "low",
			},
			expected: false,
		},
		{
			name:      "Wrong framework - skip",
			framework: "tensorflow",
			file: &ProfilerFileInfo{
				FileName:   "profiler.json",
				FileType:   "chrome_trace",
				FileSize:   10 * 1024 * 1024,
				Confidence: "medium",
			},
			expected: false,
		},
		{
			name:      "File too large - skip",
			framework: "pytorch",
			file: &ProfilerFileInfo{
				FileName:   "large-profiler.json",
				FileType:   "chrome_trace",
				FileSize:   200 * 1024 * 1024, // 200MB > 100MB limit
				Confidence: "high",
			},
			expected: false,
		},
		{
			name:      "Type not allowed - skip",
			framework: "pytorch",
			file: &ProfilerFileInfo{
				FileName:   "memory.pickle",
				FileType:   "memory_dump",
				FileSize:   10 * 1024 * 1024,
				Confidence: "high",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.shouldCollectFile(tt.framework, tt.file)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollector_CheckConfidence(t *testing.T) {
	config := &CollectorConfig{
		Filter: &FilterConfig{
			MinConfidence: "medium",
		},
	}

	backend := newMockStorageBackend()
	collector, _ := NewCollector(config, backend, "http://node-exporter:8080")

	tests := []struct {
		confidence string
		expected   bool
	}{
		{"high", true},
		{"medium", true},
		{"low", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.confidence, func(t *testing.T) {
			result := collector.checkConfidence(tt.confidence)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollector_IsAllowedType(t *testing.T) {
	config := &CollectorConfig{
		Filter: &FilterConfig{
			AllowedTypes: []string{"chrome_trace", "pytorch_trace", "stack_trace"},
		},
	}

	backend := newMockStorageBackend()
	collector, _ := NewCollector(config, backend, "http://node-exporter:8080")

	tests := []struct {
		fileType string
		expected bool
	}{
		{"chrome_trace", true},
		{"pytorch_trace", true},
		{"stack_trace", true},
		{"memory_dump", false},
		{"kineto", false},
	}

	for _, tt := range tests {
		t.Run(tt.fileType, func(t *testing.T) {
			result := collector.isAllowedType(tt.fileType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollector_GetSkipReason(t *testing.T) {
	config := &CollectorConfig{
		AutoCollect: true,
		Filter: &FilterConfig{
			MinConfidence:  "medium",
			MaxFileSize:    100 * 1024 * 1024,
			AllowedTypes:   []string{"chrome_trace"},
			RequireFramework: true,
		},
	}

	backend := newMockStorageBackend()
	collector, _ := NewCollector(config, backend, "http://node-exporter:8080")

	tests := []struct {
		name       string
		framework  string
		file       *ProfilerFileInfo
		contains   string
	}{
		{
			name:      "Low confidence",
			framework: "pytorch",
			file: &ProfilerFileInfo{
				FileType:   "chrome_trace",
				FileSize:   10 * 1024 * 1024,
				Confidence: "low",
			},
			contains: "confidence too low",
		},
		{
			name:      "File too large",
			framework: "pytorch",
			file: &ProfilerFileInfo{
				FileType:   "chrome_trace",
				FileSize:   200 * 1024 * 1024,
				Confidence: "high",
			},
			contains: "file too large",
		},
		{
			name:      "Type not allowed",
			framework: "pytorch",
			file: &ProfilerFileInfo{
				FileType:   "memory_dump",
				FileSize:   10 * 1024 * 1024,
				Confidence: "high",
			},
			contains: "type not allowed",
		},
		{
			name:      "Framework requirement not met",
			framework: "tensorflow",
			file: &ProfilerFileInfo{
				FileType:   "chrome_trace",
				FileSize:   10 * 1024 * 1024,
				Confidence: "medium",
			},
			contains: "framework requirement not met",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := collector.getSkipReason(tt.framework, tt.file)
			assert.Contains(t, reason, tt.contains)
		})
	}
}

func TestGenerateFileID(t *testing.T) {
	workloadUID := "test-workload-123"
	fileName := "profiler.json"

	fileID1 := generateFileID(workloadUID, fileName)
	assert.NotEmpty(t, fileID1)
	assert.Contains(t, fileID1, workloadUID)
	assert.Contains(t, fileID1, fileName)

	// Sleep to ensure different timestamp (Unix timestamp is in seconds)
	time.Sleep(time.Second * 2)

	fileID2 := generateFileID(workloadUID, fileName)
	assert.NotEqual(t, fileID1, fileID2, "FileIDs should be unique due to different timestamps")
}

func TestCollectionRequest_Validation(t *testing.T) {
	req := &CollectionRequest{
		WorkloadUID: "workload-001",
		PodUID:      "pod-123",
		PodName:     "training-pod-0",
		Framework:   "pytorch",
		Files: []*ProfilerFileInfo{
			{
				FileName:   "profiler.json",
				FileType:   "chrome_trace",
				FileSize:   10 * 1024 * 1024,
				Confidence: "high",
			},
		},
	}

	assert.NotEmpty(t, req.WorkloadUID)
	assert.NotEmpty(t, req.PodUID)
	assert.NotEmpty(t, req.Framework)
	assert.NotEmpty(t, req.Files)
}

func TestCollectionResult_Fields(t *testing.T) {
	result := &CollectionResult{
		WorkloadUID:   "workload-001",
		TotalFiles:    10,
		ArchivedFiles: 8,
		SkippedFiles:  2,
		FailedFiles:   0,
		CollectedAt:   time.Now(),
		Files:         []*ArchivedFileInfo{},
		Errors:        []string{},
	}

	assert.NotEmpty(t, result.WorkloadUID)
	assert.Equal(t, 10, result.TotalFiles)
	assert.Equal(t, 8, result.ArchivedFiles)
	assert.Equal(t, 2, result.SkippedFiles)
	assert.Equal(t, 0, result.FailedFiles)
	assert.NotNil(t, result.Files)
	assert.NotNil(t, result.Errors)
}

func TestArchivedFileInfo_Fields(t *testing.T) {
	info := &ArchivedFileInfo{
		FileName:    "profiler.json",
		FilePath:    "/workspace/logs/profiler.json",
		FileType:    "chrome_trace",
		FileSize:    10 * 1024 * 1024,
		StorageType: "object_storage",
		StoragePath: "profiler/workload-001/2024-12-15/chrome_trace/profiler.json",
		DownloadURL: "https://minio.example.com/...",
		CollectedAt: time.Now(),
	}

	assert.NotEmpty(t, info.FileName)
	assert.NotEmpty(t, info.FilePath)
	assert.NotEmpty(t, info.FileType)
	assert.Greater(t, info.FileSize, int64(0))
	assert.NotEmpty(t, info.StorageType)
	assert.NotEmpty(t, info.StoragePath)
	assert.NotEmpty(t, info.DownloadURL)
}

func TestFilterConfig_Defaults(t *testing.T) {
	// Test that NewCollector sets default filter config
	config := &CollectorConfig{
		AutoCollect: true,
		Interval:    300,
		// No Filter set
	}

	backend := newMockStorageBackend()
	collector, err := NewCollector(config, backend, "http://node-exporter:8080")

	require.NoError(t, err)
	assert.NotNil(t, collector.config.Filter)
	assert.Equal(t, "medium", collector.config.Filter.MinConfidence)
	assert.Greater(t, collector.config.Filter.MaxFileSize, int64(0))
	assert.NotEmpty(t, collector.config.Filter.AllowedTypes)
	assert.True(t, collector.config.Filter.RequireFramework)
}

func TestCollectorConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *CollectorConfig
		wantErr bool
	}{
		{
			name: "Valid config",
			config: &CollectorConfig{
				AutoCollect: true,
				Interval:    300,
				Filter: &FilterConfig{
					MinConfidence:  "medium",
					MaxFileSize:    100 * 1024 * 1024,
					AllowedTypes:   []string{"chrome_trace"},
					RequireFramework: true,
				},
			},
			wantErr: false,
		},
		{
			name:    "Nil config",
			config:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newMockStorageBackend()
			_, err := NewCollector(tt.config, backend, "http://node-exporter:8080")

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Mock storage backend for testing (simplified version)
type mockBackend struct {
	storage.StorageBackend
}

func (m *mockBackend) Store(ctx context.Context, req *storage.StoreRequest) (*storage.StoreResponse, error) {
	return &storage.StoreResponse{
		FileID:      req.FileID,
		StoragePath: "mock-path",
		StorageType: "mock",
		Size:        int64(len(req.Content)),
		MD5:         "mock-md5",
	}, nil
}

func (m *mockBackend) GenerateDownloadURL(ctx context.Context, fileID string, expires time.Duration) (string, error) {
	return "http://mock-url/" + fileID, nil
}

func (m *mockBackend) GetStorageType() string {
	return "mock"
}

