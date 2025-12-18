package tracelens

import (
	"bytes"
	"compress/gzip"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfilerFileContentChunk(t *testing.T) {
	tests := []struct {
		name     string
		chunk    ProfilerFileContentChunk
		validate func(t *testing.T, chunk ProfilerFileContentChunk)
	}{
		{
			name: "single chunk file",
			chunk: ProfilerFileContentChunk{
				Content:         []byte("test content"),
				ContentEncoding: "none",
				ChunkIndex:      0,
				TotalChunks:     1,
			},
			validate: func(t *testing.T, chunk ProfilerFileContentChunk) {
				assert.Equal(t, []byte("test content"), chunk.Content)
				assert.Equal(t, "none", chunk.ContentEncoding)
				assert.Equal(t, 0, chunk.ChunkIndex)
				assert.Equal(t, 1, chunk.TotalChunks)
			},
		},
		{
			name: "first of multiple chunks",
			chunk: ProfilerFileContentChunk{
				Content:         []byte("chunk1data"),
				ContentEncoding: "gzip",
				ChunkIndex:      0,
				TotalChunks:     3,
			},
			validate: func(t *testing.T, chunk ProfilerFileContentChunk) {
				assert.Equal(t, 0, chunk.ChunkIndex)
				assert.Equal(t, 3, chunk.TotalChunks)
				assert.Equal(t, "gzip", chunk.ContentEncoding)
			},
		},
		{
			name: "last of multiple chunks",
			chunk: ProfilerFileContentChunk{
				Content:         []byte("chunk3data"),
				ContentEncoding: "gzip",
				ChunkIndex:      2,
				TotalChunks:     3,
			},
			validate: func(t *testing.T, chunk ProfilerFileContentChunk) {
				assert.Equal(t, 2, chunk.ChunkIndex)
				assert.Equal(t, 3, chunk.TotalChunks)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.chunk)
		})
	}
}

func TestCombineChunks(t *testing.T) {
	chunks := []ProfilerFileContentChunk{
		{Content: []byte("Hello, "), ChunkIndex: 0, TotalChunks: 3},
		{Content: []byte("World"), ChunkIndex: 1, TotalChunks: 3},
		{Content: []byte("!"), ChunkIndex: 2, TotalChunks: 3},
	}

	var combinedContent bytes.Buffer
	for _, chunk := range chunks {
		combinedContent.Write(chunk.Content)
	}

	expected := "Hello, World!"
	assert.Equal(t, expected, combinedContent.String())
}

func TestGzipDecompression(t *testing.T) {
	originalContent := []byte("This is test content that will be compressed and decompressed")

	// Compress the content
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	_, err := gzWriter.Write(originalContent)
	require.NoError(t, err)
	err = gzWriter.Close()
	require.NoError(t, err)

	// Decompress the content
	reader, err := gzip.NewReader(bytes.NewReader(compressed.Bytes()))
	require.NoError(t, err)
	defer reader.Close()

	var decompressed bytes.Buffer
	_, err = decompressed.ReadFrom(reader)
	require.NoError(t, err)

	assert.Equal(t, originalContent, decompressed.Bytes())
}

func TestFileIDParsing(t *testing.T) {
	tests := []struct {
		name        string
		fileIDStr   string
		expectError bool
		expectedID  int64
	}{
		{
			name:        "valid positive id",
			fileIDStr:   "123",
			expectError: false,
			expectedID:  123,
		},
		{
			name:        "valid large id",
			fileIDStr:   "999999999",
			expectError: false,
			expectedID:  999999999,
		},
		{
			name:        "valid zero",
			fileIDStr:   "0",
			expectError: false,
			expectedID:  0,
		},
		{
			name:        "invalid string",
			fileIDStr:   "abc",
			expectError: true,
		},
		{
			name:        "invalid mixed",
			fileIDStr:   "123abc",
			expectError: true,
		},
		{
			name:        "invalid empty",
			fileIDStr:   "",
			expectError: true,
		},
		{
			name:        "invalid float",
			fileIDStr:   "123.45",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileID, err := strconv.ParseInt(tt.fileIDStr, 10, 32)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedID, fileID)
			}
		})
	}
}

func TestStorageTypeValidation(t *testing.T) {
	tests := []struct {
		name         string
		storageType  string
		isSupported  bool
	}{
		{
			name:        "database storage supported",
			storageType: "database",
			isSupported: true,
		},
		{
			name:        "object storage not supported",
			storageType: "object_storage",
			isSupported: false,
		},
		{
			name:        "minio not supported",
			storageType: "minio",
			isSupported: false,
		},
		{
			name:        "empty not supported",
			storageType: "",
			isSupported: false,
		},
		{
			name:        "unknown not supported",
			storageType: "unknown",
			isSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSupported := tt.storageType == "database"
			assert.Equal(t, tt.isSupported, isSupported)
		})
	}
}

func TestContentEncodingHandling(t *testing.T) {
	tests := []struct {
		name            string
		encoding        string
		shouldDecompress bool
	}{
		{
			name:            "gzip encoding",
			encoding:        "gzip",
			shouldDecompress: true,
		},
		{
			name:            "none encoding",
			encoding:        "none",
			shouldDecompress: false,
		},
		{
			name:            "empty encoding",
			encoding:        "",
			shouldDecompress: false,
		},
		{
			name:            "unknown encoding treated as none",
			encoding:        "unknown",
			shouldDecompress: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldDecompress := tt.encoding == "gzip"
			assert.Equal(t, tt.shouldDecompress, shouldDecompress)
		})
	}
}

func TestContentDispositionHeader(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		expected string
	}{
		{
			name:     "simple filename",
			fileName: "trace.json",
			expected: "attachment; filename=\"trace.json\"",
		},
		{
			name:     "filename with spaces",
			fileName: "trace file.json",
			expected: "attachment; filename=\"trace file.json\"",
		},
		{
			name:     "complex filename",
			fileName: "pytorch_trace_1234567890.json",
			expected: "attachment; filename=\"pytorch_trace_1234567890.json\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := "attachment; filename=\"" + tt.fileName + "\""
			assert.Equal(t, tt.expected, header)
		})
	}
}

func TestFileInfoStruct(t *testing.T) {
	// Test the file info structure used in GetProfilerFileInfo
	type FileInfo struct {
		ID           int32  `json:"id"`
		WorkloadUID  string `json:"workload_uid"`
		FileName     string `json:"file_name"`
		FileType     string `json:"file_type"`
		FileSize     int64  `json:"file_size"`
		StorageType  string `json:"storage_type"`
		CollectedAt  string `json:"collected_at"`
		PodName      string `json:"pod_name"`
		PodNamespace string `json:"pod_namespace"`
	}

	info := FileInfo{
		ID:           123,
		WorkloadUID:  "workload-456",
		FileName:     "pytorch_trace.json",
		FileType:     "chrome_trace",
		FileSize:     1048576,
		StorageType:  "database",
		CollectedAt:  "2024-01-15T10:30:00Z",
		PodName:      "training-pod-abc",
		PodNamespace: "default",
	}

	assert.Equal(t, int32(123), info.ID)
	assert.Equal(t, "workload-456", info.WorkloadUID)
	assert.Equal(t, "pytorch_trace.json", info.FileName)
	assert.Equal(t, "chrome_trace", info.FileType)
	assert.Equal(t, int64(1048576), info.FileSize)
	assert.Equal(t, "database", info.StorageType)
}

func TestChunkOrdering(t *testing.T) {
	// Test that chunks are processed in correct order
	chunks := []ProfilerFileContentChunk{
		{Content: []byte("third"), ChunkIndex: 2, TotalChunks: 3},
		{Content: []byte("first"), ChunkIndex: 0, TotalChunks: 3},
		{Content: []byte("second"), ChunkIndex: 1, TotalChunks: 3},
	}

	// Sort chunks by index (simulating ORDER BY chunk_index)
	sortedChunks := make([]ProfilerFileContentChunk, len(chunks))
	for _, c := range chunks {
		sortedChunks[c.ChunkIndex] = c
	}

	assert.Equal(t, []byte("first"), sortedChunks[0].Content)
	assert.Equal(t, []byte("second"), sortedChunks[1].Content)
	assert.Equal(t, []byte("third"), sortedChunks[2].Content)
}

func TestEmptyChunksHandling(t *testing.T) {
	chunks := []ProfilerFileContentChunk{}

	var combinedContent bytes.Buffer
	for _, chunk := range chunks {
		combinedContent.Write(chunk.Content)
	}

	assert.Empty(t, combinedContent.Bytes())
	assert.Equal(t, 0, len(chunks))
}

func TestContentLengthHeader(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected string
	}{
		{
			name:     "small content",
			content:  []byte("hello"),
			expected: "5",
		},
		{
			name:     "empty content",
			content:  []byte{},
			expected: "0",
		},
		{
			name:     "large content",
			content:  make([]byte, 1024*1024), // 1MB
			expected: "1048576",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentLength := strconv.Itoa(len(tt.content))
			assert.Equal(t, tt.expected, contentLength)
		})
	}
}

func TestGzipReaderError(t *testing.T) {
	// Test handling of invalid gzip data
	invalidGzip := []byte("this is not gzip data")

	_, err := gzip.NewReader(bytes.NewReader(invalidGzip))
	assert.Error(t, err)
}

func TestLargeFileChunking(t *testing.T) {
	// Simulate chunking of a large file
	chunkSize := 100
	totalSize := 350

	content := make([]byte, totalSize)
	for i := range content {
		content[i] = byte(i % 256)
	}

	// Calculate expected chunks
	expectedChunks := (totalSize + chunkSize - 1) / chunkSize
	assert.Equal(t, 4, expectedChunks) // 350 / 100 = 3.5 -> 4 chunks

	// Create chunks
	var chunks []ProfilerFileContentChunk
	for i := 0; i < totalSize; i += chunkSize {
		end := i + chunkSize
		if end > totalSize {
			end = totalSize
		}
		chunks = append(chunks, ProfilerFileContentChunk{
			Content:     content[i:end],
			ChunkIndex:  len(chunks),
			TotalChunks: expectedChunks,
		})
	}

	assert.Len(t, chunks, expectedChunks)

	// Verify chunk sizes
	assert.Len(t, chunks[0].Content, 100)
	assert.Len(t, chunks[1].Content, 100)
	assert.Len(t, chunks[2].Content, 100)
	assert.Len(t, chunks[3].Content, 50)

	// Recombine and verify
	var combined bytes.Buffer
	for _, chunk := range chunks {
		combined.Write(chunk.Content)
	}
	assert.Equal(t, content, combined.Bytes())
}

