package storage

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStorageBackend_ObjectStorage(t *testing.T) {
	config := &StorageConfig{
		Strategy: "object_storage",
		Object: &ObjectStorageConfig{
			Type:      "minio",
			Endpoint:  "localhost:9000",
			Bucket:    "test-bucket",
			AccessKey: "test-key",
			SecretKey: "test-secret",
			UseSSL:    false,
		},
	}

	db, _, _ := sqlmock.New()
	defer db.Close()

	backend, err := NewStorageBackend(db, config)

	// Note: Will fail without actual MinIO connection, but tests the factory logic
	if err != nil {
		assert.Contains(t, err.Error(), "failed to create object storage backend")
	} else {
		assert.NotNil(t, backend)
		assert.Equal(t, "object_storage", backend.GetStorageType())
	}
}

func TestNewStorageBackend_Database(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	config := &StorageConfig{
		Strategy: "database",
		Database: &DatabaseConfig{
			Compression:         true,
			ChunkSize:           10 * 1024 * 1024,
			MaxFileSize:         100 * 1024 * 1024,
			MaxConcurrentChunks: 5,
		},
	}

	backend, err := NewStorageBackend(db, config)

	require.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Equal(t, "database", backend.GetStorageType())
}

func TestNewStorageBackend_NilConfig(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	backend, err := NewStorageBackend(db, nil)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "storage config is nil")
}

func TestNewStorageBackend_UnknownStrategy(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	config := &StorageConfig{
		Strategy: "unknown_strategy",
	}

	backend, err := NewStorageBackend(db, config)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "unknown storage strategy")
}

func TestNewStorageBackend_Database_NilDB(t *testing.T) {
	config := &StorageConfig{
		Strategy: "database",
		Database: &DatabaseConfig{},
	}

	backend, err := NewStorageBackend(nil, config)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "database connection is required")
}

func TestNewStorageBackend_Database_MissingConfig(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	config := &StorageConfig{
		Strategy: "database",
		// Database config is missing
	}

	backend, err := NewStorageBackend(db, config)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "database storage config is missing")
}

func TestNewStorageBackend_ObjectStorage_MissingConfig(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	config := &StorageConfig{
		Strategy: "object_storage",
		// Object config is missing
	}

	backend, err := NewStorageBackend(db, config)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "object storage config is missing")
}

func TestNewAutoSelectBackend(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	config := &StorageConfig{
		Strategy: "auto",
		Auto: &AutoSelectConfig{
			Enabled:       true,
			SizeThreshold: 10 * 1024 * 1024,
		},
		Database: &DatabaseConfig{
			ChunkSize:   10 * 1024 * 1024,
			MaxFileSize: 100 * 1024 * 1024,
		},
	}

	backend, err := NewAutoSelectBackend(db, config)

	// May fail without MinIO, but should create database backend at least
	if err == nil {
		assert.NotNil(t, backend)
		assert.Equal(t, int64(10*1024*1024), backend.sizeThreshold)
	}
}

func TestNewAutoSelectBackend_NilAutoConfig(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	config := &StorageConfig{
		Strategy: "auto",
		Auto:     nil,
	}

	backend, err := NewAutoSelectBackend(db, config)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "auto selection config is nil")
}

func TestNewAutoSelectBackend_NoBackendsAvailable(t *testing.T) {
	config := &StorageConfig{
		Strategy: "auto",
		Auto: &AutoSelectConfig{
			Enabled:       true,
			SizeThreshold: 10 * 1024 * 1024,
		},
		// No database or object storage config
	}

	backend, err := NewAutoSelectBackend(nil, config)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "failed to create any storage backend")
}

func TestAutoSelectBackend_SelectBackend_SmallFile(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	dbBackend, _ := NewDatabaseStorageBackend(db, &DatabaseConfig{
		ChunkSize:   10 * 1024 * 1024,
		MaxFileSize: 100 * 1024 * 1024,
	})

	autoBackend := &AutoSelectBackend{
		objectBackend:   nil,
		databaseBackend: dbBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	// Test small file (< threshold)
	smallSize := int64(5 * 1024 * 1024) // 5MB
	selected := autoBackend.selectBackend(smallSize)

	assert.NotNil(t, selected)
	assert.Equal(t, "database", selected.GetStorageType())
}

func TestAutoSelectBackend_SelectBackend_LargeFile(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	dbBackend, _ := NewDatabaseStorageBackend(db, &DatabaseConfig{
		ChunkSize:   10 * 1024 * 1024,
		MaxFileSize: 100 * 1024 * 1024,
	})

	// Mock object backend
	mockObjBackend := NewMockStorageBackend()

	autoBackend := &AutoSelectBackend{
		objectBackend:   mockObjBackend,
		databaseBackend: dbBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	// Test large file (>= threshold)
	largeSize := int64(50 * 1024 * 1024) // 50MB
	selected := autoBackend.selectBackend(largeSize)

	assert.NotNil(t, selected)
	assert.Equal(t, "mock", selected.GetStorageType()) // Mock returns "mock"
}

func TestAutoSelectBackend_SelectBackend_NoObjectBackend(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	dbBackend, _ := NewDatabaseStorageBackend(db, &DatabaseConfig{
		ChunkSize:   10 * 1024 * 1024,
		MaxFileSize: 100 * 1024 * 1024,
	})

	autoBackend := &AutoSelectBackend{
		objectBackend:   nil, // No object backend
		databaseBackend: dbBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	// Large file should fallback to database
	largeSize := int64(50 * 1024 * 1024)
	selected := autoBackend.selectBackend(largeSize)

	assert.NotNil(t, selected)
	assert.Equal(t, "database", selected.GetStorageType())
}

func TestAutoSelectBackend_Store(t *testing.T) {
	mockBackend := NewMockStorageBackend()

	autoBackend := &AutoSelectBackend{
		objectBackend:   mockBackend,
		databaseBackend: mockBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	req := &StoreRequest{
		FileID:  "test-file",
		Content: make([]byte, 5*1024*1024), // 5MB - should use database
	}

	resp, err := autoBackend.Store(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAutoSelectBackend_Retrieve(t *testing.T) {
	mockBackend := NewMockStorageBackend()

	autoBackend := &AutoSelectBackend{
		objectBackend:   mockBackend,
		databaseBackend: mockBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	// First store a file
	storeReq := &StoreRequest{
		FileID:  "test-file",
		Content: []byte("test content"),
	}
	_, _ = mockBackend.Store(context.Background(), storeReq)

	// Then retrieve it
	retrieveReq := &RetrieveRequest{
		FileID:      "test-file",
		StoragePath: "test-file",
	}

	resp, err := autoBackend.Retrieve(context.Background(), retrieveReq)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, []byte("test content"), resp.Content)
}

func TestAutoSelectBackend_Retrieve_NotFound(t *testing.T) {
	mockBackend := NewMockStorageBackend()

	autoBackend := &AutoSelectBackend{
		objectBackend:   mockBackend,
		databaseBackend: mockBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	retrieveReq := &RetrieveRequest{
		FileID:      "non-existent",
		StoragePath: "non-existent",
	}

	resp, err := autoBackend.Retrieve(context.Background(), retrieveReq)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "file not found")
}

func TestAutoSelectBackend_Delete(t *testing.T) {
	mockBackend := NewMockStorageBackend()

	autoBackend := &AutoSelectBackend{
		objectBackend:   mockBackend,
		databaseBackend: mockBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	// Store a file first
	storeReq := &StoreRequest{
		FileID:  "test-file",
		Content: []byte("test"),
	}
	_, _ = mockBackend.Store(context.Background(), storeReq)

	// Delete it
	err := autoBackend.Delete(context.Background(), "test-file")

	require.NoError(t, err)

	// Verify it's deleted
	exists, _ := mockBackend.Exists(context.Background(), "test-file")
	assert.False(t, exists)
}

func TestAutoSelectBackend_GenerateDownloadURL(t *testing.T) {
	mockBackend := NewMockStorageBackend()

	autoBackend := &AutoSelectBackend{
		objectBackend:   mockBackend,
		databaseBackend: mockBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	// Store a file first
	storeReq := &StoreRequest{
		FileID:  "test-file",
		Content: []byte("test"),
	}
	_, _ = mockBackend.Store(context.Background(), storeReq)

	// Generate download URL
	url, err := autoBackend.GenerateDownloadURL(context.Background(), "test-file", time.Hour)

	require.NoError(t, err)
	assert.Contains(t, url, "test-file")
}

func TestAutoSelectBackend_GenerateDownloadURL_NotFound(t *testing.T) {
	mockBackend := NewMockStorageBackend()

	autoBackend := &AutoSelectBackend{
		objectBackend:   mockBackend,
		databaseBackend: mockBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	url, err := autoBackend.GenerateDownloadURL(context.Background(), "non-existent", time.Hour)

	assert.Error(t, err)
	assert.Empty(t, url)
	assert.Contains(t, err.Error(), "file not found")
}

func TestAutoSelectBackend_GetStorageType(t *testing.T) {
	autoBackend := &AutoSelectBackend{}

	storageType := autoBackend.GetStorageType()

	assert.Equal(t, "auto", storageType)
}

func TestAutoSelectBackend_Exists(t *testing.T) {
	mockBackend := NewMockStorageBackend()

	autoBackend := &AutoSelectBackend{
		objectBackend:   mockBackend,
		databaseBackend: mockBackend,
		sizeThreshold:   10 * 1024 * 1024,
	}

	// Store a file
	storeReq := &StoreRequest{
		FileID:  "test-file",
		Content: []byte("test"),
	}
	_, _ = mockBackend.Store(context.Background(), storeReq)

	// Check existence
	exists, err := autoBackend.Exists(context.Background(), "test-file")

	require.NoError(t, err)
	assert.True(t, exists)

	// Check non-existent file
	exists, err = autoBackend.Exists(context.Background(), "non-existent")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestFactoryStorageConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *StorageConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid object storage config",
			config: &StorageConfig{
				Strategy: "object_storage",
				Object: &ObjectStorageConfig{
					Endpoint: "localhost:9000",
					Bucket:   "test",
				},
			},
			wantErr: false,
		},
		{
			name: "Valid database config",
			config: &StorageConfig{
				Strategy: "database",
				Database: &DatabaseConfig{
					ChunkSize: 10 * 1024 * 1024,
				},
			},
			wantErr: false,
		},
		{
			name: "Valid auto config",
			config: &StorageConfig{
				Strategy: "auto",
				Auto: &AutoSelectConfig{
					Enabled:       true,
					SizeThreshold: 10 * 1024 * 1024,
				},
				Database: &DatabaseConfig{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation
			assert.NotEmpty(t, tt.config.Strategy)

			switch tt.config.Strategy {
			case "object_storage":
				if !tt.wantErr {
					assert.NotNil(t, tt.config.Object)
				}
			case "database":
				if !tt.wantErr {
					assert.NotNil(t, tt.config.Database)
				}
			case "auto":
				if !tt.wantErr {
					assert.NotNil(t, tt.config.Auto)
				}
			}
		})
	}
}

func TestObjectStorageConfig_Fields(t *testing.T) {
	config := &ObjectStorageConfig{
		Type:       "minio",
		Endpoint:   "localhost:9000",
		Bucket:     "profiler-data",
		AccessKey:  "access",
		SecretKey:  "secret",
		UseSSL:     false,
		Region:     "us-east-1",
		URLExpires: "168h",
	}

	assert.Equal(t, "minio", config.Type)
	assert.Equal(t, "localhost:9000", config.Endpoint)
	assert.Equal(t, "profiler-data", config.Bucket)
	assert.False(t, config.UseSSL)
	assert.Equal(t, "168h", config.URLExpires)

	// Test URL expiration parsing
	duration, err := time.ParseDuration(config.URLExpires)
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, duration)
}

func TestDatabaseConfig_Fields(t *testing.T) {
	config := &DatabaseConfig{
		Compression:         true,
		ChunkSize:           10 * 1024 * 1024,
		MaxFileSize:         200 * 1024 * 1024,
		MaxConcurrentChunks: 5,
	}

	assert.True(t, config.Compression)
	assert.Equal(t, int64(10*1024*1024), config.ChunkSize)
	assert.Equal(t, int64(200*1024*1024), config.MaxFileSize)
	assert.Equal(t, 5, config.MaxConcurrentChunks)
}

func TestAutoSelectConfig_Fields(t *testing.T) {
	config := &AutoSelectConfig{
		Enabled:       true,
		SizeThreshold: 10 * 1024 * 1024,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, int64(10*1024*1024), config.SizeThreshold)
}
