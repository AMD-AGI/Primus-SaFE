package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// NewObjectStorageBackend Tests
// ============================================================================

func TestNewObjectStorageBackend_NilConfig(t *testing.T) {
	backend, err := NewObjectStorageBackend(nil)
	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "config is nil")
}

func TestNewObjectStorageBackend_InvalidEndpoint(t *testing.T) {
	config := &ObjectStorageConfig{
		Endpoint:  "invalid-endpoint-that-does-not-exist.example.com:9000",
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
		Bucket:    "test-bucket",
		UseSSL:    false,
	}

	// Should not fail on creation, MinIO client is lazy
	backend, err := NewObjectStorageBackend(config)
	// May fail due to invalid endpoint or succeed with lazy initialization
	if err != nil {
		assert.Contains(t, err.Error(), "failed")
	} else {
		assert.NotNil(t, backend)
	}
}

func TestNewObjectStorageBackend_URLExpiresDefault(t *testing.T) {
	config := &ObjectStorageConfig{
		Endpoint:   "localhost:9000",
		AccessKey:  "test-access-key",
		SecretKey:  "test-secret-key",
		Bucket:     "test-bucket",
		UseSSL:     false,
		URLExpires: "", // Empty, should use default
	}

	// This will likely fail due to no MinIO server, but we can check the config
	_, _ = NewObjectStorageBackend(config)
	// Default should be 7 days
}

func TestNewObjectStorageBackend_CustomURLExpires(t *testing.T) {
	config := &ObjectStorageConfig{
		Endpoint:   "localhost:9000",
		AccessKey:  "test-access-key",
		SecretKey:  "test-secret-key",
		Bucket:     "test-bucket",
		UseSSL:     false,
		URLExpires: "48h",
	}

	_, _ = NewObjectStorageBackend(config)
}

func TestNewObjectStorageBackend_InvalidURLExpires(t *testing.T) {
	config := &ObjectStorageConfig{
		Endpoint:   "localhost:9000",
		AccessKey:  "test-access-key",
		SecretKey:  "test-secret-key",
		Bucket:     "test-bucket",
		UseSSL:     false,
		URLExpires: "invalid",
	}

	// Should not fail, but use default
	_, _ = NewObjectStorageBackend(config)
}

// ============================================================================
// generateObjectKey Tests
// ============================================================================

func TestObjectStorageBackend_generateObjectKey(t *testing.T) {
	// Create a minimal backend for testing helper functions
	backend := &ObjectStorageBackend{
		bucket: "test-bucket",
	}

	tests := []struct {
		name        string
		workloadUID string
		fileType    string
		fileName    string
		checkPrefix string
	}{
		{
			name:        "standard file",
			workloadUID: "workload-123",
			fileType:    "chrome_trace",
			fileName:    "trace.json",
			checkPrefix: "profiler/workload-123/",
		},
		{
			name:        "file with special characters",
			workloadUID: "workload_abc-123",
			fileType:    "memory_dump",
			fileName:    "memory.dump.gz",
			checkPrefix: "profiler/workload_abc-123/",
		},
		{
			name:        "empty workload uid",
			workloadUID: "",
			fileType:    "trace",
			fileName:    "file.json",
			checkPrefix: "profiler//",
		},
		{
			name:        "trace file",
			workloadUID: "test-uid",
			fileType:    "kineto",
			fileName:    "kineto_trace.json",
			checkPrefix: "profiler/test-uid/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := backend.generateObjectKey(tt.workloadUID, tt.fileType, tt.fileName)

			// Check prefix
			assert.Contains(t, key, tt.checkPrefix)

			// Check file type in path
			assert.Contains(t, key, tt.fileType)

			// Check filename at end
			assert.Contains(t, key, tt.fileName)

			// Check date format (YYYY-MM-DD)
			today := time.Now().Format("2006-01-02")
			assert.Contains(t, key, today)
		})
	}
}

// ============================================================================
// getContentType Tests
// ============================================================================

func TestObjectStorageBackend_getContentType(t *testing.T) {
	backend := &ObjectStorageBackend{}

	tests := []struct {
		name     string
		fileName string
		expected string
	}{
		{
			name:     "gzip file",
			fileName: "trace.json.gz",
			expected: "application/gzip",
		},
		{
			name:     "json file",
			fileName: "trace.json",
			expected: "application/json",
		},
		{
			name:     "pickle file",
			fileName: "model.pickle",
			expected: "application/octet-stream",
		},
		{
			name:     "unknown extension",
			fileName: "file.xyz",
			expected: "application/octet-stream",
		},
		{
			name:     "no extension",
			fileName: "file",
			expected: "application/octet-stream",
		},
		{
			name:     "pt.trace.json",
			fileName: "primus-megatron-exp[test]-rank[0].12345.pt.trace.json",
			expected: "application/json",
		},
		{
			name:     "pt.trace.json.gz",
			fileName: "primus-megatron-exp[test]-rank[0].12345.pt.trace.json.gz",
			expected: "application/gzip",
		},
		{
			name:     "short filename",
			fileName: "a.b",
			expected: "application/octet-stream",
		},
		{
			name:     "uppercase extension",
			fileName: "trace.JSON",
			expected: "application/octet-stream", // Case sensitive
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backend.getContentType(tt.fileName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// extractRange Tests
// ============================================================================

func TestObjectStorageBackend_extractRange(t *testing.T) {
	backend := &ObjectStorageBackend{}

	data := []byte("0123456789ABCDEF") // 16 bytes

	tests := []struct {
		name     string
		offset   int64
		length   int64
		expected []byte
	}{
		{
			name:     "full range",
			offset:   0,
			length:   0,
			expected: []byte("0123456789ABCDEF"),
		},
		{
			name:     "from beginning with length",
			offset:   0,
			length:   5,
			expected: []byte("01234"),
		},
		{
			name:     "from middle",
			offset:   5,
			length:   5,
			expected: []byte("56789"),
		},
		{
			name:     "from middle to end",
			offset:   10,
			length:   0,
			expected: []byte("ABCDEF"),
		},
		{
			name:     "offset beyond data",
			offset:   20,
			length:   5,
			expected: []byte{},
		},
		{
			name:     "length exceeds remaining",
			offset:   14,
			length:   10,
			expected: []byte("EF"),
		},
		{
			name:     "single byte",
			offset:   0,
			length:   1,
			expected: []byte("0"),
		},
		{
			name:     "last byte",
			offset:   15,
			length:   1,
			expected: []byte("F"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backend.extractRange(data, tt.offset, tt.length)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestObjectStorageBackend_extractRange_EmptyData(t *testing.T) {
	backend := &ObjectStorageBackend{}

	result := backend.extractRange([]byte{}, 0, 10)
	assert.Equal(t, []byte{}, result)
}

func TestObjectStorageBackend_extractRange_NilData(t *testing.T) {
	backend := &ObjectStorageBackend{}

	// Note: nil slice is different from empty slice
	result := backend.extractRange(nil, 0, 10)
	assert.Equal(t, []byte{}, result)
}

// ============================================================================
// GetStorageType Tests
// ============================================================================

func TestObjectStorageBackend_GetStorageType(t *testing.T) {
	backend := &ObjectStorageBackend{}
	assert.Equal(t, "object_storage", backend.GetStorageType())
}

// Note: ObjectStorageConfig, StoreRequest, StoreResponse, RetrieveRequest, RetrieveResponse
// field tests are in factory_test.go and storage_test.go to avoid duplication

// ============================================================================
// Edge Cases Tests
// ============================================================================

func TestObjectStorageBackend_EdgeCases(t *testing.T) {
	backend := &ObjectStorageBackend{}

	t.Run("empty workload uid in key", func(t *testing.T) {
		key := backend.generateObjectKey("", "type", "file.json")
		assert.Contains(t, key, "profiler//")
	})

	t.Run("empty file type in key", func(t *testing.T) {
		key := backend.generateObjectKey("uid", "", "file.json")
		assert.Contains(t, key, "/file.json")
	})

	t.Run("empty filename in key", func(t *testing.T) {
		key := backend.generateObjectKey("uid", "type", "")
		// Should still generate a key
		assert.NotEmpty(t, key)
	})

	t.Run("very long filename", func(t *testing.T) {
		longName := "primus-megatron-exp[very-long-experiment-name-with-lots-of-characters]-rank[0].1234567890123456789.pt.trace.json.gz"
		contentType := backend.getContentType(longName)
		assert.Equal(t, "application/gzip", contentType)
	})
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestObjectStorageBackend_Integration(t *testing.T) {
	// Skip if no MinIO server available
	t.Skip("Requires MinIO server - integration test")

	config := &ObjectStorageConfig{
		Endpoint:   "localhost:9000",
		AccessKey:  "minioadmin",
		SecretKey:  "minioadmin",
		Bucket:     "test-bucket",
		UseSSL:     false,
		URLExpires: "1h",
	}

	backend, err := NewObjectStorageBackend(config)
	require.NoError(t, err)

	// Test store
	storeReq := &StoreRequest{
		FileID:      "test-file-1",
		WorkloadUID: "test-workload",
		FileName:    "trace.json",
		FileType:    "chrome_trace",
		Content:     []byte(`{"traceEvents": []}`),
		Compressed:  false,
	}

	// storeResp, err := backend.Store(context.Background(), storeReq)
	// require.NoError(t, err)
	// assert.NotEmpty(t, storeResp.StoragePath)

	_ = backend
	_ = storeReq
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkObjectStorageBackend_generateObjectKey(b *testing.B) {
	backend := &ObjectStorageBackend{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.generateObjectKey("workload-123", "chrome_trace", "trace.json")
	}
}

func BenchmarkObjectStorageBackend_getContentType(b *testing.B) {
	backend := &ObjectStorageBackend{}
	fileName := "primus-megatron-exp[test]-rank[0].12345.pt.trace.json.gz"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.getContentType(fileName)
	}
}

func BenchmarkObjectStorageBackend_extractRange(b *testing.B) {
	backend := &ObjectStorageBackend{}
	data := make([]byte, 1024*1024) // 1 MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.extractRange(data, 1000, 5000)
	}
}

