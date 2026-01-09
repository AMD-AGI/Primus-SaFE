// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestStoreRequest_Fields tests StoreRequest struct fields
func TestStoreRequest_Fields(t *testing.T) {
	content := []byte("test content")
	req := StoreRequest{
		FileID:      "file-123",
		WorkloadUID: "workload-456",
		FileName:    "test.svg",
		FileType:    "flamegraph",
		Content:     content,
		Compressed:  false,
		Metadata: map[string]string{
			"pod_name":      "my-pod",
			"pod_namespace": "default",
		},
	}

	if req.FileID != "file-123" {
		t.Errorf("Expected FileID 'file-123', got %s", req.FileID)
	}
	if req.WorkloadUID != "workload-456" {
		t.Errorf("Expected WorkloadUID 'workload-456', got %s", req.WorkloadUID)
	}
	if req.FileName != "test.svg" {
		t.Errorf("Expected FileName 'test.svg', got %s", req.FileName)
	}
	if req.FileType != "flamegraph" {
		t.Errorf("Expected FileType 'flamegraph', got %s", req.FileType)
	}
	if !bytes.Equal(req.Content, content) {
		t.Error("Content mismatch")
	}
	if req.Compressed {
		t.Error("Expected Compressed to be false")
	}
	if req.Metadata["pod_name"] != "my-pod" {
		t.Errorf("Expected pod_name 'my-pod', got %s", req.Metadata["pod_name"])
	}
}

// TestStoreResponse_Fields tests StoreResponse struct fields
func TestStoreResponse_Fields(t *testing.T) {
	resp := StoreResponse{
		FileID:      "file-123",
		StoragePath: "/path/to/file",
		StorageType: "database",
		Size:        1024,
		MD5:         "d41d8cd98f00b204e9800998ecf8427e",
		Metadata: map[string]interface{}{
			"compressed": true,
			"chunks":     3,
		},
	}

	if resp.FileID != "file-123" {
		t.Errorf("Expected FileID 'file-123', got %s", resp.FileID)
	}
	if resp.StoragePath != "/path/to/file" {
		t.Errorf("Expected StoragePath '/path/to/file', got %s", resp.StoragePath)
	}
	if resp.StorageType != "database" {
		t.Errorf("Expected StorageType 'database', got %s", resp.StorageType)
	}
	if resp.Size != 1024 {
		t.Errorf("Expected Size 1024, got %d", resp.Size)
	}
	if resp.Metadata["compressed"] != true {
		t.Error("Expected metadata compressed to be true")
	}
	if resp.Metadata["chunks"] != 3 {
		t.Errorf("Expected metadata chunks to be 3, got %v", resp.Metadata["chunks"])
	}
}

// TestRetrieveRequest_Fields tests RetrieveRequest struct fields
func TestRetrieveRequest_Fields(t *testing.T) {
	tests := []struct {
		name   string
		req    RetrieveRequest
		verify func(t *testing.T, req RetrieveRequest)
	}{
		{
			name: "basic request",
			req: RetrieveRequest{
				FileID:      "file-123",
				StoragePath: "123",
			},
			verify: func(t *testing.T, req RetrieveRequest) {
				if req.FileID != "file-123" {
					t.Errorf("Expected FileID 'file-123', got %s", req.FileID)
				}
				if req.StoragePath != "123" {
					t.Errorf("Expected StoragePath '123', got %s", req.StoragePath)
				}
				if req.Offset != 0 {
					t.Errorf("Expected Offset 0, got %d", req.Offset)
				}
				if req.Length != 0 {
					t.Errorf("Expected Length 0, got %d", req.Length)
				}
			},
		},
		{
			name: "partial read request",
			req: RetrieveRequest{
				FileID:      "file-456",
				StoragePath: "456",
				Offset:      100,
				Length:      500,
			},
			verify: func(t *testing.T, req RetrieveRequest) {
				if req.Offset != 100 {
					t.Errorf("Expected Offset 100, got %d", req.Offset)
				}
				if req.Length != 500 {
					t.Errorf("Expected Length 500, got %d", req.Length)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.req)
		})
	}
}

// TestRetrieveResponse_Fields tests RetrieveResponse struct fields
func TestRetrieveResponse_Fields(t *testing.T) {
	content := []byte("retrieved content")
	resp := RetrieveResponse{
		Content:    content,
		Size:       int64(len(content)),
		Compressed: false,
		MD5:        "abc123",
	}

	if !bytes.Equal(resp.Content, content) {
		t.Error("Content mismatch")
	}
	if resp.Size != int64(len(content)) {
		t.Errorf("Expected Size %d, got %d", len(content), resp.Size)
	}
	if resp.Compressed {
		t.Error("Expected Compressed to be false")
	}
	if resp.MD5 != "abc123" {
		t.Errorf("Expected MD5 'abc123', got %s", resp.MD5)
	}
}

// TestStorageConfig_Fields tests StorageConfig struct fields
func TestStorageConfig_Fields(t *testing.T) {
	tests := []struct {
		name   string
		config StorageConfig
		verify func(t *testing.T, cfg StorageConfig)
	}{
		{
			name: "database strategy",
			config: StorageConfig{
				Strategy: "database",
				Database: &DatabaseConfig{
					Compression: true,
					ChunkSize:   10 * 1024 * 1024,
					MaxFileSize: 200 * 1024 * 1024,
				},
			},
			verify: func(t *testing.T, cfg StorageConfig) {
				if cfg.Strategy != "database" {
					t.Errorf("Expected strategy 'database', got %s", cfg.Strategy)
				}
				if cfg.Database == nil {
					t.Fatal("Expected Database config to be set")
				}
				if !cfg.Database.Compression {
					t.Error("Expected Compression to be true")
				}
			},
		},
		{
			name: "object storage strategy",
			config: StorageConfig{
				Strategy: "object_storage",
				Object: &ObjectStorageConfig{
					Type:     "minio",
					Endpoint: "localhost:9000",
					Bucket:   "profiler",
				},
			},
			verify: func(t *testing.T, cfg StorageConfig) {
				if cfg.Strategy != "object_storage" {
					t.Errorf("Expected strategy 'object_storage', got %s", cfg.Strategy)
				}
				if cfg.Object == nil {
					t.Fatal("Expected Object config to be set")
				}
				if cfg.Object.Type != "minio" {
					t.Errorf("Expected Type 'minio', got %s", cfg.Object.Type)
				}
			},
		},
		{
			name: "auto strategy",
			config: StorageConfig{
				Strategy: "auto",
				Auto: &AutoSelectConfig{
					Enabled:       true,
					SizeThreshold: 50 * 1024 * 1024,
				},
			},
			verify: func(t *testing.T, cfg StorageConfig) {
				if cfg.Strategy != "auto" {
					t.Errorf("Expected strategy 'auto', got %s", cfg.Strategy)
				}
				if cfg.Auto == nil {
					t.Fatal("Expected Auto config to be set")
				}
				if !cfg.Auto.Enabled {
					t.Error("Expected Auto.Enabled to be true")
				}
				if cfg.Auto.SizeThreshold != 50*1024*1024 {
					t.Errorf("Expected SizeThreshold 50MB, got %d", cfg.Auto.SizeThreshold)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.config)
		})
	}
}

// TestDatabaseConfig_Defaults tests DatabaseConfig with various configurations
func TestDatabaseConfig_Defaults(t *testing.T) {
	tests := []struct {
		name   string
		config DatabaseConfig
		verify func(t *testing.T, cfg DatabaseConfig)
	}{
		{
			name: "all fields set",
			config: DatabaseConfig{
				Compression:         true,
				ChunkSize:           5 * 1024 * 1024,
				MaxFileSize:         100 * 1024 * 1024,
				MaxConcurrentChunks: 3,
			},
			verify: func(t *testing.T, cfg DatabaseConfig) {
				if !cfg.Compression {
					t.Error("Expected Compression to be true")
				}
				if cfg.ChunkSize != 5*1024*1024 {
					t.Errorf("Expected ChunkSize 5MB, got %d", cfg.ChunkSize)
				}
				if cfg.MaxFileSize != 100*1024*1024 {
					t.Errorf("Expected MaxFileSize 100MB, got %d", cfg.MaxFileSize)
				}
				if cfg.MaxConcurrentChunks != 3 {
					t.Errorf("Expected MaxConcurrentChunks 3, got %d", cfg.MaxConcurrentChunks)
				}
			},
		},
		{
			name:   "zero values",
			config: DatabaseConfig{},
			verify: func(t *testing.T, cfg DatabaseConfig) {
				if cfg.Compression {
					t.Error("Expected Compression to be false (zero value)")
				}
				if cfg.ChunkSize != 0 {
					t.Errorf("Expected ChunkSize 0 (zero value), got %d", cfg.ChunkSize)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.config)
		})
	}
}

// TestObjectStorageConfig_Fields tests ObjectStorageConfig struct fields
func TestObjectStorageConfig_Fields(t *testing.T) {
	cfg := ObjectStorageConfig{
		Type:       "s3",
		Endpoint:   "s3.amazonaws.com",
		Bucket:     "my-bucket",
		AccessKey:  "AKIAIOSFODNN7EXAMPLE",
		SecretKey:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		UseSSL:     true,
		Region:     "us-west-2",
		URLExpires: "168h",
	}

	if cfg.Type != "s3" {
		t.Errorf("Expected Type 's3', got %s", cfg.Type)
	}
	if cfg.Endpoint != "s3.amazonaws.com" {
		t.Errorf("Expected Endpoint 's3.amazonaws.com', got %s", cfg.Endpoint)
	}
	if cfg.Bucket != "my-bucket" {
		t.Errorf("Expected Bucket 'my-bucket', got %s", cfg.Bucket)
	}
	if !cfg.UseSSL {
		t.Error("Expected UseSSL to be true")
	}
	if cfg.Region != "us-west-2" {
		t.Errorf("Expected Region 'us-west-2', got %s", cfg.Region)
	}
	if cfg.URLExpires != "168h" {
		t.Errorf("Expected URLExpires '168h', got %s", cfg.URLExpires)
	}
}

// TestAutoSelectConfig_Fields tests AutoSelectConfig struct fields
func TestAutoSelectConfig_Fields(t *testing.T) {
	tests := []struct {
		name   string
		config AutoSelectConfig
		verify func(t *testing.T, cfg AutoSelectConfig)
	}{
		{
			name: "enabled with threshold",
			config: AutoSelectConfig{
				Enabled:       true,
				SizeThreshold: 50 * 1024 * 1024, // 50MB
			},
			verify: func(t *testing.T, cfg AutoSelectConfig) {
				if !cfg.Enabled {
					t.Error("Expected Enabled to be true")
				}
				if cfg.SizeThreshold != 50*1024*1024 {
					t.Errorf("Expected SizeThreshold 50MB, got %d", cfg.SizeThreshold)
				}
			},
		},
		{
			name: "disabled",
			config: AutoSelectConfig{
				Enabled:       false,
				SizeThreshold: 100 * 1024 * 1024,
			},
			verify: func(t *testing.T, cfg AutoSelectConfig) {
				if cfg.Enabled {
					t.Error("Expected Enabled to be false")
				}
			},
		},
		{
			name:   "zero values",
			config: AutoSelectConfig{},
			verify: func(t *testing.T, cfg AutoSelectConfig) {
				if cfg.Enabled {
					t.Error("Expected Enabled to be false (zero value)")
				}
				if cfg.SizeThreshold != 0 {
					t.Errorf("Expected SizeThreshold 0 (zero value), got %d", cfg.SizeThreshold)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.config)
		})
	}
}

// MockStorageBackend implements StorageBackend for testing
type MockStorageBackend struct {
	storageType    string
	files          map[string]*StoreResponse
	storeErr       error
	retrieveErr    error
	deleteErr      error
	existsResult   bool
	existsErr      error
	existsWLResult bool
	existsWLErr    error
}

// NewMockStorageBackend creates a new mock storage backend
func NewMockStorageBackend(storageType string) *MockStorageBackend {
	return &MockStorageBackend{
		storageType: storageType,
		files:       make(map[string]*StoreResponse),
	}
}

func (m *MockStorageBackend) Store(ctx context.Context, req *StoreRequest) (*StoreResponse, error) {
	if m.storeErr != nil {
		return nil, m.storeErr
	}
	resp := &StoreResponse{
		FileID:      req.FileID,
		StoragePath: req.FileID,
		StorageType: m.storageType,
		Size:        int64(len(req.Content)),
		MD5:         "mock-md5",
	}
	m.files[req.FileID] = resp
	return resp, nil
}

func (m *MockStorageBackend) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResponse, error) {
	if m.retrieveErr != nil {
		return nil, m.retrieveErr
	}
	return &RetrieveResponse{
		Content: []byte("mock content"),
		Size:    12,
		MD5:     "mock-md5",
	}, nil
}

func (m *MockStorageBackend) Delete(ctx context.Context, fileID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.files, fileID)
	return nil
}

func (m *MockStorageBackend) GenerateDownloadURL(ctx context.Context, fileID string, expires time.Duration) (string, error) {
	return "/mock/download/" + fileID, nil
}

func (m *MockStorageBackend) GetStorageType() string {
	return m.storageType
}

func (m *MockStorageBackend) Exists(ctx context.Context, fileID string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	if m.existsResult {
		return true, nil
	}
	_, exists := m.files[fileID]
	return exists, nil
}

func (m *MockStorageBackend) ExistsByWorkloadAndFilename(ctx context.Context, workloadUID string, fileName string) (bool, error) {
	if m.existsWLErr != nil {
		return false, m.existsWLErr
	}
	return m.existsWLResult, nil
}

// TestMockStorageBackend_Store tests mock storage backend store operation
func TestMockStorageBackend_Store(t *testing.T) {
	backend := NewMockStorageBackend("mock")
	ctx := context.Background()

	req := &StoreRequest{
		FileID:      "test-file",
		WorkloadUID: "workload-123",
		FileName:    "test.svg",
		FileType:    "flamegraph",
		Content:     []byte("test content"),
	}

	resp, err := backend.Store(ctx, req)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if resp.FileID != "test-file" {
		t.Errorf("Expected FileID 'test-file', got %s", resp.FileID)
	}
	if resp.StorageType != "mock" {
		t.Errorf("Expected StorageType 'mock', got %s", resp.StorageType)
	}

	// Verify file exists
	exists, err := backend.Exists(ctx, "test-file")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist after store")
	}
}

// TestMockStorageBackend_Delete tests mock storage backend delete operation
func TestMockStorageBackend_Delete(t *testing.T) {
	backend := NewMockStorageBackend("mock")
	ctx := context.Background()

	// Store a file first
	req := &StoreRequest{
		FileID:  "test-file",
		Content: []byte("test content"),
	}
	_, err := backend.Store(ctx, req)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Delete the file
	err = backend.Delete(ctx, "test-file")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file no longer exists
	exists, err := backend.Exists(ctx, "test-file")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Expected file to not exist after delete")
	}
}

// TestMockStorageBackend_GetStorageType tests GetStorageType method
func TestMockStorageBackend_GetStorageType(t *testing.T) {
	tests := []struct {
		name         string
		storageType  string
		expectedType string
	}{
		{"database", "database", "database"},
		{"object_storage", "object_storage", "object_storage"},
		{"auto", "auto", "auto"},
		{"custom", "my-custom-storage", "my-custom-storage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewMockStorageBackend(tt.storageType)
			if backend.GetStorageType() != tt.expectedType {
				t.Errorf("Expected storage type '%s', got '%s'", tt.expectedType, backend.GetStorageType())
			}
		})
	}
}
