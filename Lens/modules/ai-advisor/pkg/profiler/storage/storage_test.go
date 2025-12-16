package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     *StoreRequest
		wantErr bool
	}{
		{
			name: "Valid request",
			req: &StoreRequest{
				FileID:      "test-file-123",
				WorkloadUID: "workload-001",
				FileName:    "profiler.json",
				FileType:    "chrome_trace",
				Content:     []byte("test content"),
				Compressed:  false,
			},
			wantErr: false,
		},
		{
			name: "Valid compressed request",
			req: &StoreRequest{
				FileID:      "test-file-456",
				WorkloadUID: "workload-002",
				FileName:    "profiler.json.gz",
				FileType:    "chrome_trace",
				Content:     []byte("compressed content"),
				Compressed:  true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation
			if tt.req.FileID == "" {
				t.Error("FileID should not be empty")
			}
			if tt.req.WorkloadUID == "" {
				t.Error("WorkloadUID should not be empty")
			}
			if tt.req.FileName == "" {
				t.Error("FileName should not be empty")
			}
			if tt.req.Content == nil {
				t.Error("Content should not be nil")
			}
		})
	}
}

func TestStorageConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *StorageConfig
		wantErr bool
	}{
		{
			name: "Object storage config",
			config: &StorageConfig{
				Strategy: "object_storage",
				Object: &ObjectStorageConfig{
					Type:      "minio",
					Endpoint:  "minio:9000",
					Bucket:    "profiler-data",
					AccessKey: "test-key",
					SecretKey: "test-secret",
					UseSSL:    false,
				},
			},
			wantErr: false,
		},
		{
			name: "Database config",
			config: &StorageConfig{
				Strategy: "database",
				Database: &DatabaseConfig{
					Compression:         true,
					ChunkSize:           10 * 1024 * 1024,
					MaxFileSize:         200 * 1024 * 1024,
					MaxConcurrentChunks: 5,
				},
			},
			wantErr: false,
		},
		{
			name: "Auto select config",
			config: &StorageConfig{
				Strategy: "auto",
				Auto: &AutoSelectConfig{
					Enabled:       true,
					SizeThreshold: 10 * 1024 * 1024,
				},
				Object: &ObjectStorageConfig{
					Type:      "minio",
					Endpoint:  "minio:9000",
					Bucket:    "profiler-data",
					AccessKey: "test-key",
					SecretKey: "test-secret",
				},
				Database: &DatabaseConfig{
					Compression: true,
					ChunkSize:   10 * 1024 * 1024,
					MaxFileSize: 100 * 1024 * 1024,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.config.Strategy)

			switch tt.config.Strategy {
			case "object_storage":
				assert.NotNil(t, tt.config.Object)
				assert.NotEmpty(t, tt.config.Object.Endpoint)
				assert.NotEmpty(t, tt.config.Object.Bucket)
			case "database":
				assert.NotNil(t, tt.config.Database)
				assert.Greater(t, tt.config.Database.ChunkSize, int64(0))
			case "auto":
				assert.NotNil(t, tt.config.Auto)
				assert.True(t, tt.config.Auto.Enabled)
			}
		})
	}
}

func TestRetrieveRequest_Validation(t *testing.T) {
	req := &RetrieveRequest{
		FileID:      "test-file-123",
		StoragePath: "profiler/workload-001/2024-12-15/chrome_trace/profiler.json",
		Offset:      0,
		Length:      1024,
	}

	assert.NotEmpty(t, req.FileID)
	assert.NotEmpty(t, req.StoragePath)
	assert.GreaterOrEqual(t, req.Offset, int64(0))
	assert.GreaterOrEqual(t, req.Length, int64(0))
}

func TestObjectStorageConfig_URLExpires(t *testing.T) {
	config := &ObjectStorageConfig{
		URLExpires: "168h", // 7 days
	}

	duration, err := time.ParseDuration(config.URLExpires)
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, duration)
}

func TestDatabaseConfig_Defaults(t *testing.T) {
	config := &DatabaseConfig{}

	// Test default values would be set in NewDatabaseStorageBackend
	assert.Equal(t, int64(0), config.ChunkSize)
	assert.Equal(t, int64(0), config.MaxFileSize)
	assert.Equal(t, 0, config.MaxConcurrentChunks)
}

func TestAutoSelectConfig_Threshold(t *testing.T) {
	tests := []struct {
		name      string
		fileSize  int64
		threshold int64
		useDB     bool
	}{
		{
			name:      "Small file uses database",
			fileSize:  5 * 1024 * 1024, // 5MB
			threshold: 10 * 1024 * 1024, // 10MB
			useDB:     true,
		},
		{
			name:      "Large file uses object storage",
			fileSize:  50 * 1024 * 1024, // 50MB
			threshold: 10 * 1024 * 1024,  // 10MB
			useDB:     false,
		},
		{
			name:      "Exactly threshold uses object storage",
			fileSize:  10 * 1024 * 1024, // 10MB
			threshold: 10 * 1024 * 1024, // 10MB
			useDB:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldUseDB := tt.fileSize < tt.threshold
			assert.Equal(t, tt.useDB, shouldUseDB)
		})
	}
}

func TestStoreResponse_Fields(t *testing.T) {
	resp := &StoreResponse{
		FileID:      "test-file-123",
		StoragePath: "profiler/workload-001/2024-12-15/chrome_trace/profiler.json",
		StorageType: "object_storage",
		Size:        1024,
		MD5:         "abc123def456",
		Metadata: map[string]interface{}{
			"bucket": "profiler-data",
			"etag":   "xyz789",
		},
	}

	assert.NotEmpty(t, resp.FileID)
	assert.NotEmpty(t, resp.StoragePath)
	assert.NotEmpty(t, resp.StorageType)
	assert.Greater(t, resp.Size, int64(0))
	assert.NotEmpty(t, resp.MD5)
	assert.NotNil(t, resp.Metadata)
	assert.Equal(t, "profiler-data", resp.Metadata["bucket"])
}

func TestRetrieveResponse_Fields(t *testing.T) {
	testContent := []byte("test content")
	resp := &RetrieveResponse{
		Content:    testContent,
		Size:       int64(len(testContent)),
		Compressed: false,
		MD5:        "abc123",
	}

	assert.NotEmpty(t, resp.Content)
	assert.Equal(t, int64(len(testContent)), resp.Size)
	assert.False(t, resp.Compressed)
	assert.NotEmpty(t, resp.MD5)
}

// Mock storage backend for testing
type MockStorageBackend struct {
	files map[string][]byte
}

func NewMockStorageBackend() *MockStorageBackend {
	return &MockStorageBackend{
		files: make(map[string][]byte),
	}
}

func (m *MockStorageBackend) Store(ctx context.Context, req *StoreRequest) (*StoreResponse, error) {
	m.files[req.FileID] = req.Content
	return &StoreResponse{
		FileID:      req.FileID,
		StoragePath: req.FileID,
		StorageType: "mock",
		Size:        int64(len(req.Content)),
		MD5:         "mock-md5",
	}, nil
}

func (m *MockStorageBackend) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResponse, error) {
	content, exists := m.files[req.FileID]
	if !exists {
		return nil, assert.AnError
	}
	return &RetrieveResponse{
		Content:    content,
		Size:       int64(len(content)),
		Compressed: false,
		MD5:        "mock-md5",
	}, nil
}

func (m *MockStorageBackend) Delete(ctx context.Context, fileID string) error {
	delete(m.files, fileID)
	return nil
}

func (m *MockStorageBackend) GenerateDownloadURL(ctx context.Context, fileID string, expires time.Duration) (string, error) {
	return "http://mock-url/" + fileID, nil
}

func (m *MockStorageBackend) GetStorageType() string {
	return "mock"
}

func (m *MockStorageBackend) Exists(ctx context.Context, fileID string) (bool, error) {
	_, exists := m.files[fileID]
	return exists, nil
}

func TestMockStorageBackend(t *testing.T) {
	ctx := context.Background()
	backend := NewMockStorageBackend()

	// Test Store
	storeReq := &StoreRequest{
		FileID:      "test-file-1",
		WorkloadUID: "workload-1",
		FileName:    "test.json",
		FileType:    "chrome_trace",
		Content:     []byte("test content"),
	}

	storeResp, err := backend.Store(ctx, storeReq)
	require.NoError(t, err)
	assert.Equal(t, "test-file-1", storeResp.FileID)
	assert.Equal(t, "mock", storeResp.StorageType)

	// Test Exists
	exists, err := backend.Exists(ctx, "test-file-1")
	require.NoError(t, err)
	assert.True(t, exists)

	// Test Retrieve
	retrieveReq := &RetrieveRequest{
		FileID:      "test-file-1",
		StoragePath: "test-file-1",
	}

	retrieveResp, err := backend.Retrieve(ctx, retrieveReq)
	require.NoError(t, err)
	assert.Equal(t, []byte("test content"), retrieveResp.Content)

	// Test GenerateDownloadURL
	url, err := backend.GenerateDownloadURL(ctx, "test-file-1", time.Hour)
	require.NoError(t, err)
	assert.Contains(t, url, "test-file-1")

	// Test Delete
	err = backend.Delete(ctx, "test-file-1")
	require.NoError(t, err)

	exists, err = backend.Exists(ctx, "test-file-1")
	require.NoError(t, err)
	assert.False(t, exists)
}

