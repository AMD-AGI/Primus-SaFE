// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage

import (
	"context"
	"testing"
)


// TestNewStorageBackend_Validation tests NewStorageBackend validation
func TestNewStorageBackend_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      *StorageConfig
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "storage config is nil",
		},
		{
			name: "unknown strategy",
			config: &StorageConfig{
				Strategy: "unknown",
			},
			wantErr:     true,
			errContains: "unknown storage strategy",
		},
		{
			name: "object storage without config",
			config: &StorageConfig{
				Strategy: "object_storage",
				Object:   nil,
			},
			wantErr:     true,
			errContains: "object storage config is missing",
		},
		{
			name: "database without db connection",
			config: &StorageConfig{
				Strategy: "database",
				Database: &DatabaseConfig{
					Compression: true,
				},
			},
			wantErr:     true,
			errContains: "database connection is required",
		},
		{
			name: "database without config",
			config: &StorageConfig{
				Strategy: "database",
				Database: nil,
			},
			wantErr:     true,
			errContains: "database connection is required", // db is nil, so this error comes first
		},
		{
			name: "auto without config",
			config: &StorageConfig{
				Strategy: "auto",
				Auto:     nil,
			},
			wantErr:     true,
			errContains: "auto selection config is missing",
		},
		{
			name: "auto disabled",
			config: &StorageConfig{
				Strategy: "auto",
				Auto: &AutoSelectConfig{
					Enabled: false,
				},
			},
			wantErr:     true,
			errContains: "auto selection config is missing or disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewStorageBackend(nil, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStorageBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("Error message %q doesn't contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestAutoSelectBackend_SelectBackend tests backend selection logic
func TestAutoSelectBackend_SelectBackend(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	tests := []struct {
		name            string
		backend         *AutoSelectBackend
		fileSize        int64
		expectedBackend string
	}{
		{
			name: "small file uses database",
			backend: &AutoSelectBackend{
				objectBackend:   objBackend,
				databaseBackend: dbBackend,
				sizeThreshold:   50 * 1024 * 1024, // 50MB
			},
			fileSize:        10 * 1024 * 1024, // 10MB
			expectedBackend: "database",
		},
		{
			name: "large file uses object storage",
			backend: &AutoSelectBackend{
				objectBackend:   objBackend,
				databaseBackend: dbBackend,
				sizeThreshold:   50 * 1024 * 1024, // 50MB
			},
			fileSize:        100 * 1024 * 1024, // 100MB
			expectedBackend: "object_storage",
		},
		{
			name: "exactly at threshold uses object storage",
			backend: &AutoSelectBackend{
				objectBackend:   objBackend,
				databaseBackend: dbBackend,
				sizeThreshold:   50 * 1024 * 1024, // 50MB
			},
			fileSize:        50 * 1024 * 1024, // 50MB
			expectedBackend: "object_storage",
		},
		{
			name: "no database backend uses object storage",
			backend: &AutoSelectBackend{
				objectBackend:   objBackend,
				databaseBackend: nil,
				sizeThreshold:   50 * 1024 * 1024,
			},
			fileSize:        10 * 1024 * 1024,
			expectedBackend: "object_storage",
		},
		{
			name: "no object backend uses database for large files",
			backend: &AutoSelectBackend{
				objectBackend:   nil,
				databaseBackend: dbBackend,
				sizeThreshold:   50 * 1024 * 1024,
			},
			fileSize:        100 * 1024 * 1024,
			expectedBackend: "database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected := tt.backend.selectBackend(tt.fileSize)
			if selected == nil {
				t.Fatal("selectBackend returned nil")
			}
			if selected.GetStorageType() != tt.expectedBackend {
				t.Errorf("Expected %s backend, got %s", tt.expectedBackend, selected.GetStorageType())
			}
		})
	}
}

// TestAutoSelectBackend_Store tests the Store method
func TestAutoSelectBackend_Store(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   100, // 100 bytes for testing
	}

	ctx := context.Background()

	tests := []struct {
		name            string
		content         []byte
		expectedBackend string
	}{
		{
			name:            "small file goes to database",
			content:         []byte("small content"),
			expectedBackend: "database",
		},
		{
			name:            "large file goes to object storage",
			content:         make([]byte, 200),
			expectedBackend: "object_storage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &StoreRequest{
				FileID:  tt.name,
				Content: tt.content,
			}
			resp, err := backend.Store(ctx, req)
			if err != nil {
				t.Fatalf("Store failed: %v", err)
			}
			if resp.StorageType != tt.expectedBackend {
				t.Errorf("Expected storage type %s, got %s", tt.expectedBackend, resp.StorageType)
			}
		})
	}
}

// TestAutoSelectBackend_Retrieve tests the Retrieve method
func TestAutoSelectBackend_Retrieve(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   100,
	}

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

	// Retrieve it
	retrieveReq := &RetrieveRequest{
		FileID: "test-file",
	}
	resp, err := backend.Retrieve(ctx, retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
}

// TestAutoSelectBackend_Retrieve_NotFound tests Retrieve when file doesn't exist
func TestAutoSelectBackend_Retrieve_NotFound(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   100,
	}

	ctx := context.Background()

	retrieveReq := &RetrieveRequest{
		FileID: "non-existent-file",
	}
	_, err := backend.Retrieve(ctx, retrieveReq)
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// TestAutoSelectBackend_Delete tests the Delete method
func TestAutoSelectBackend_Delete(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   100,
	}

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

	// Delete it
	err = backend.Delete(ctx, "test-file")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

// TestAutoSelectBackend_Exists tests the Exists method
func TestAutoSelectBackend_Exists(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   100,
	}

	ctx := context.Background()

	// Initially should not exist
	exists, err := backend.Exists(ctx, "test-file")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Expected file to not exist initially")
	}

	// Store a file
	req := &StoreRequest{
		FileID:  "test-file",
		Content: []byte("test content"),
	}
	_, err = backend.Store(ctx, req)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Now should exist
	exists, err = backend.Exists(ctx, "test-file")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist after store")
	}
}

// TestAutoSelectBackend_ExistsByWorkloadAndFilename tests ExistsByWorkloadAndFilename
func TestAutoSelectBackend_ExistsByWorkloadAndFilename(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   100,
	}

	ctx := context.Background()

	// Test when not found
	exists, err := backend.ExistsByWorkloadAndFilename(ctx, "workload-123", "test.svg")
	if err != nil {
		t.Fatalf("ExistsByWorkloadAndFilename failed: %v", err)
	}
	if exists {
		t.Error("Expected file to not exist")
	}

	// Test when found
	dbBackend.existsWLResult = true
	exists, err = backend.ExistsByWorkloadAndFilename(ctx, "workload-123", "test.svg")
	if err != nil {
		t.Fatalf("ExistsByWorkloadAndFilename failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist")
	}
}

// TestAutoSelectBackend_GenerateDownloadURL tests GenerateDownloadURL
func TestAutoSelectBackend_GenerateDownloadURL(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   100,
	}

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

	// Generate URL
	url, err := backend.GenerateDownloadURL(ctx, "test-file", 3600)
	if err != nil {
		t.Fatalf("GenerateDownloadURL failed: %v", err)
	}
	if url == "" {
		t.Error("Expected non-empty URL")
	}
}

// TestAutoSelectBackend_GenerateDownloadURL_NotFound tests GenerateDownloadURL for non-existent file
func TestAutoSelectBackend_GenerateDownloadURL_NotFound(t *testing.T) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   100,
	}

	ctx := context.Background()

	_, err := backend.GenerateDownloadURL(ctx, "non-existent", 3600)
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// TestAutoSelectBackend_GetStorageType tests GetStorageType
func TestAutoSelectBackend_GetStorageType(t *testing.T) {
	backend := &AutoSelectBackend{}
	if backend.GetStorageType() != "auto" {
		t.Errorf("Expected 'auto', got '%s'", backend.GetStorageType())
	}
}

// TestNewAutoSelectBackend_Validation tests NewAutoSelectBackend validation
func TestNewAutoSelectBackend_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *StorageConfig
		wantErr bool
	}{
		{
			name: "nil auto config",
			config: &StorageConfig{
				Strategy: "auto",
				Auto:     nil,
			},
			wantErr: true,
		},
		{
			name: "no backends available",
			config: &StorageConfig{
				Strategy: "auto",
				Auto: &AutoSelectConfig{
					Enabled:       true,
					SizeThreshold: 100,
				},
				Object:   nil,
				Database: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAutoSelectBackend(nil, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAutoSelectBackend() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkAutoSelectBackend_SelectBackend benchmarks backend selection
func BenchmarkAutoSelectBackend_SelectBackend(b *testing.B) {
	dbBackend := NewMockStorageBackend("database")
	objBackend := NewMockStorageBackend("object_storage")

	backend := &AutoSelectBackend{
		objectBackend:   objBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   50 * 1024 * 1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.selectBackend(int64(i % 100 * 1024 * 1024))
	}
}
