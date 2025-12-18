package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return db, mock
}

func TestNewDatabaseStorageBackend(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		Compression:         true,
		ChunkSize:           10 * 1024 * 1024,
		MaxFileSize:         200 * 1024 * 1024,
		MaxConcurrentChunks: 5,
	}

	backend, err := NewDatabaseStorageBackend(db, config)

	require.NoError(t, err)
	assert.NotNil(t, backend)
	assert.True(t, backend.compression)
	assert.Equal(t, int64(10*1024*1024), backend.chunkSize)
	assert.Equal(t, int64(200*1024*1024), backend.maxFileSize)
	assert.Equal(t, 5, backend.maxConcurrentChunks)
}

func TestNewDatabaseStorageBackend_NilDB(t *testing.T) {
	config := &DatabaseConfig{}

	backend, err := NewDatabaseStorageBackend(nil, config)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "database connection is nil")
}

func TestNewDatabaseStorageBackend_NilConfig(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	backend, err := NewDatabaseStorageBackend(db, nil)

	assert.Error(t, err)
	assert.Nil(t, backend)
	assert.Contains(t, err.Error(), "database config is nil")
}

func TestNewDatabaseStorageBackend_Defaults(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{} // Empty config

	backend, err := NewDatabaseStorageBackend(db, config)

	require.NoError(t, err)
	assert.Equal(t, int64(10*1024*1024), backend.chunkSize)    // Default 10MB
	assert.Equal(t, int64(200*1024*1024), backend.maxFileSize) // Default 200MB
	assert.Equal(t, 5, backend.maxConcurrentChunks)            // Default 5
}

func TestDatabaseStorageBackend_Store_SmallFile(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		Compression: false,
		ChunkSize:   10 * 1024 * 1024,
		MaxFileSize: 100 * 1024 * 1024,
	}

	backend, _ := NewDatabaseStorageBackend(db, config)

	testContent := []byte("small test content")
	req := &StoreRequest{
		FileID:      "test-file-1",
		WorkloadUID: "workload-1",
		FileName:    "test.json",
		FileType:    "chrome_trace",
		Content:     testContent,
		Compressed:  false,
		Metadata: map[string]string{
			"pod_uid":       "pod-123",
			"pod_name":      "test-pod",
			"pod_namespace": "default",
			"file_path":     "/tmp/trace.json",
			"confidence":    "0.9",
		},
	}

	// Mock transaction
	mock.ExpectBegin()

	// Step 1: Mock INSERT INTO profiler_files with RETURNING
	mock.ExpectQuery(`INSERT INTO profiler_files`).
		WithArgs(
			"workload-1",          // workload_uid
			"pod-123",             // pod_uid
			"test-pod",            // pod_name
			"default",             // pod_namespace
			"test.json",           // file_name
			"/tmp/trace.json",     // file_path
			"chrome_trace",        // file_type
			int64(len(testContent)), // file_size
			"0.9",                 // confidence (string)
			"{}",                  // metadata
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	// Step 2: Mock INSERT INTO profiler_file_content
	mock.ExpectExec(`INSERT INTO profiler_file_content`).
		WithArgs(int64(1), testContent, "none", 0, 1, len(testContent), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	resp, err := backend.Store(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-file-1", resp.FileID)
	assert.Equal(t, "database", resp.StorageType)
	assert.Equal(t, int64(len(testContent)), resp.Size)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_Store_WithCompression(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		Compression: true,
		ChunkSize:   10 * 1024 * 1024,
		MaxFileSize: 100 * 1024 * 1024,
	}

	backend, _ := NewDatabaseStorageBackend(db, config)

	testContent := []byte(strings.Repeat("a", 1000)) // Compressible content
	req := &StoreRequest{
		FileID:      "test-file-2",
		WorkloadUID: "workload-1",
		FileName:    "test.json",
		FileType:    "chrome_trace",
		Content:     testContent,
		Compressed:  false,
		Metadata: map[string]string{
			"pod_uid":       "pod-123",
			"pod_name":      "test-pod",
			"pod_namespace": "default",
			"file_path":     "/tmp/trace.json",
			"confidence":    "0.9",
		},
	}

	// Mock transaction
	mock.ExpectBegin()

	// Step 1: Mock INSERT INTO profiler_files
	mock.ExpectQuery(`INSERT INTO profiler_files`).
		WithArgs(
			"workload-1",
			"pod-123",
			"test-pod",
			"default",
			"test.json",
			"/tmp/trace.json",
			"chrome_trace",
			int64(len(testContent)),
			"0.9",
			"{}",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))

	// Step 2: Mock INSERT INTO profiler_file_content (compressed)
	mock.ExpectExec(`INSERT INTO profiler_file_content`).
		WithArgs(int64(2), sqlmock.AnyArg(), "gzip", 0, 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	resp, err := backend.Store(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Metadata)
	assert.Equal(t, true, resp.Metadata["compressed"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_Store_FileTooLarge(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		MaxFileSize: 1024, // Very small limit
	}

	backend, _ := NewDatabaseStorageBackend(db, config)

	largeContent := make([]byte, 2048) // Larger than limit
	req := &StoreRequest{
		FileID:  "large-file",
		Content: largeContent,
	}

	resp, err := backend.Store(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "file too large")
}

func TestDatabaseStorageBackend_Store_ChunkedFile(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		Compression:         false,
		ChunkSize:           100, // Small chunk size for testing
		MaxFileSize:         1000,
		MaxConcurrentChunks: 1, // Sequential for predictable testing
	}

	backend, _ := NewDatabaseStorageBackend(db, config)

	testContent := make([]byte, 250) // Will be split into 3 chunks (100, 100, 50)
	req := &StoreRequest{
		FileID:      "chunked-file",
		WorkloadUID: "workload-1",
		Content:     testContent,
		Metadata: map[string]string{
			"pod_uid":       "pod-123",
			"pod_name":      "test-pod",
			"pod_namespace": "default",
			"file_path":     "/tmp/trace.json",
			"confidence":    "0.9",
		},
	}

	// Mock transaction
	mock.ExpectBegin()

	// Step 1: Mock INSERT INTO profiler_files
	mock.ExpectQuery(`INSERT INTO profiler_files`).
		WithArgs(
			"workload-1",
			"pod-123",
			"test-pod",
			"default",
			sqlmock.AnyArg(), // file_name
			"/tmp/trace.json",
			sqlmock.AnyArg(), // file_type
			int64(len(testContent)),
			"0.9",
			"{}",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))

	// Step 2: Expect 3 INSERT statements for 3 chunks
	for i := 0; i < 3; i++ {
		mock.ExpectExec(`INSERT INTO profiler_file_content`).
			WithArgs(int64(3), sqlmock.AnyArg(), "none", i, 3, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(int64(i+1), 1))
	}

	mock.ExpectCommit()

	resp, err := backend.Store(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 3, resp.Metadata["chunks"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_Retrieve_SingleChunk(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		ChunkSize: 10 * 1024 * 1024,
	}

	backend, _ := NewDatabaseStorageBackend(db, config)

	testContent := []byte("test content")

	// Mock count query (use integer profiler_file_id)
	mock.ExpectQuery(`SELECT COUNT\(\*\), content_encoding FROM profiler_file_content`).
		WithArgs(int64(123)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "content_encoding"}).
			AddRow(1, "none"))

	// Mock single chunk retrieval
	mock.ExpectQuery(`SELECT content FROM profiler_file_content WHERE profiler_file_id`).
		WithArgs(int64(123)).
		WillReturnRows(sqlmock.NewRows([]string{"content"}).
			AddRow(testContent))

	req := &RetrieveRequest{
		FileID:      "123",
		StoragePath: "123", // Integer as string
	}

	resp, err := backend.Retrieve(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, testContent, resp.Content)
	assert.False(t, resp.Compressed)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_Retrieve_MultipleChunks(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		ChunkSize:           100,
		MaxConcurrentChunks: 1, // Sequential for testing
	}

	backend, _ := NewDatabaseStorageBackend(db, config)

	chunk1 := []byte("chunk1")
	chunk2 := []byte("chunk2")
	chunk3 := []byte("chunk3")

	// Mock count query
	mock.ExpectQuery(`SELECT COUNT\(\*\), content_encoding FROM profiler_file_content`).
		WithArgs(int64(456)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "content_encoding"}).
			AddRow(3, "none"))

	// Mock sequential chunk retrieval (3 chunks)
	mock.ExpectQuery(`SELECT content, chunk_index FROM profiler_file_content`).
		WithArgs(int64(456)).
		WillReturnRows(sqlmock.NewRows([]string{"content", "chunk_index"}).
			AddRow(chunk1, 0).
			AddRow(chunk2, 1).
			AddRow(chunk3, 2))

	req := &RetrieveRequest{
		FileID:      "456",
		StoragePath: "456",
	}

	resp, err := backend.Retrieve(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, resp)

	expectedContent := append(append(chunk1, chunk2...), chunk3...)
	assert.Equal(t, expectedContent, resp.Content)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_Retrieve_WithDecompression(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		ChunkSize: 10 * 1024 * 1024,
	}

	backend, _ := NewDatabaseStorageBackend(db, config)

	originalContent := []byte("test content for compression")

	// Compress the content
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	gzWriter.Write(originalContent)
	gzWriter.Close()
	compressedContent := compressed.Bytes()

	// Mock count query
	mock.ExpectQuery(`SELECT COUNT\(\*\), content_encoding FROM profiler_file_content`).
		WithArgs(int64(789)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "content_encoding"}).
			AddRow(1, "gzip"))

	// Mock retrieval
	mock.ExpectQuery(`SELECT content FROM profiler_file_content WHERE profiler_file_id`).
		WithArgs(int64(789)).
		WillReturnRows(sqlmock.NewRows([]string{"content"}).
			AddRow(compressedContent))

	req := &RetrieveRequest{
		FileID:      "789",
		StoragePath: "789",
	}

	resp, err := backend.Retrieve(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, originalContent, resp.Content)
	assert.False(t, resp.Compressed) // Should be decompressed
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{}
	backend, _ := NewDatabaseStorageBackend(db, config)

	// Delete chunks first
	mock.ExpectExec(`DELETE FROM profiler_file_content WHERE profiler_file_id`).
		WithArgs(int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 3)) // 3 chunks deleted

	// Then delete file metadata
	mock.ExpectExec(`DELETE FROM profiler_files WHERE id`).
		WithArgs(int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := backend.Delete(context.Background(), "100")

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_Exists_True(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{}
	backend, _ := NewDatabaseStorageBackend(db, config)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM profiler_file_content WHERE profiler_file_id`).
		WithArgs(int64(200)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	exists, err := backend.Exists(context.Background(), "200")

	require.NoError(t, err)
	assert.True(t, exists)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_Exists_False(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{}
	backend, _ := NewDatabaseStorageBackend(db, config)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM profiler_file_content WHERE profiler_file_id`).
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	exists, err := backend.Exists(context.Background(), "999")

	require.NoError(t, err)
	assert.False(t, exists)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_ExistsByWorkloadAndFilename(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{}
	backend, _ := NewDatabaseStorageBackend(db, config)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM profiler_files WHERE workload_uid`).
		WithArgs("workload-123", "trace.json").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	exists, err := backend.ExistsByWorkloadAndFilename(context.Background(), "workload-123", "trace.json")

	require.NoError(t, err)
	assert.True(t, exists)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDatabaseStorageBackend_GenerateDownloadURL(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{}
	backend, _ := NewDatabaseStorageBackend(db, config)

	url, err := backend.GenerateDownloadURL(context.Background(), "test-file-1", 0)

	require.NoError(t, err)
	assert.Contains(t, url, "/api/v1/profiler/files/test-file-1/download")
}

func TestDatabaseStorageBackend_GetStorageType(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{}
	backend, _ := NewDatabaseStorageBackend(db, config)

	storageType := backend.GetStorageType()

	assert.Equal(t, "database", storageType)
}

func TestSplitIntoChunks(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{
		ChunkSize: 100,
	}

	backend, _ := NewDatabaseStorageBackend(db, config)

	tests := []struct {
		name           string
		contentSize    int
		expectedChunks int
	}{
		{"Small file (no chunking)", 50, 1},
		{"Exact chunk size", 100, 1},
		{"Two chunks", 150, 2},
		{"Three chunks", 250, 3},
		{"Multiple exact chunks", 300, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := make([]byte, tt.contentSize)
			chunks := backend.splitIntoChunks(content)
			assert.Len(t, chunks, tt.expectedChunks)

			// Verify total size
			totalSize := 0
			for _, chunk := range chunks {
				totalSize += len(chunk)
			}
			assert.Equal(t, tt.contentSize, totalSize)
		})
	}
}

func TestCompressDecompress(t *testing.T) {
	// Use larger, more repetitive content that compresses well
	originalContent := []byte(strings.Repeat("This is test content that should be compressed and decompressed correctly. ", 100))

	// Compress
	compressed, err := compressGzip(originalContent)
	require.NoError(t, err)
	assert.NotNil(t, compressed)
	// For large repetitive data, compression should be effective
	assert.Less(t, len(compressed), len(originalContent), "Compressed size should be smaller for repetitive data")

	// Decompress
	decompressed, err := decompressGzip(compressed)
	require.NoError(t, err)
	assert.Equal(t, originalContent, decompressed)
}

func TestCompressGzip_Error(t *testing.T) {
	// This test is hard to trigger actual gzip write error
	// but we test with valid input
	data := []byte("test")
	compressed, err := compressGzip(data)
	require.NoError(t, err)
	assert.NotNil(t, compressed)
}

func TestDecompressGzip_InvalidData(t *testing.T) {
	invalidGzip := []byte("this is not gzip data")

	decompressed, err := decompressGzip(invalidGzip)

	assert.Error(t, err)
	assert.Nil(t, decompressed)
}

func TestExtractRange(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	config := &DatabaseConfig{}
	backend, _ := NewDatabaseStorageBackend(db, config)

	content := []byte("0123456789")

	tests := []struct {
		name     string
		offset   int64
		length   int64
		expected string
	}{
		{"Full content", 0, 0, "0123456789"},
		{"First 5 bytes", 0, 5, "01234"},
		{"Middle bytes", 3, 4, "3456"},
		{"Last bytes", 7, 0, "789"},
		{"Offset beyond size", 20, 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backend.extractRange(content, tt.offset, tt.length)
			assert.Equal(t, []byte(tt.expected), result)
		})
	}
}

func TestMin(t *testing.T) {
	assert.Equal(t, 5, min(5, 10))
	assert.Equal(t, 3, min(10, 3))
	assert.Equal(t, 7, min(7, 7))
}

func BenchmarkDatabaseStorage_Compress(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = compressGzip(data)
	}
}

func BenchmarkDatabaseStorage_Decompress(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB
	compressed, _ := compressGzip(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = decompressGzip(compressed)
	}
}
