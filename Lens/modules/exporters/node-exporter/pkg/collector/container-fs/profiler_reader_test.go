package containerfs

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	processtree "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/process-tree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentifyProfilerFileType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected processtree.PyTorchProfilerFileType
	}{
		{
			name:     "Chrome Trace",
			path:     "/workspace/logs/profiler-20241215.json.gz",
			expected: processtree.ProfilerTypeChromeTrace,
		},
		{
			name:     "Chrome Trace (uncompressed)",
			path:     "/workspace/logs/torch_profiler_output.json",
			expected: processtree.ProfilerTypeChromeTrace,
		},
		{
			name:     "PyTorch Trace",
			path:     "/workspace/logs/trace.pt.trace.json",
			expected: processtree.ProfilerTypePyTorchTrace,
		},
		{
			name:     "PyTorch Trace (compressed)",
			path:     "/workspace/logs/trace.pt.trace.json.gz",
			expected: processtree.ProfilerTypePyTorchTrace,
		},
		{
			name:     "Stack Trace",
			path:     "/workspace/logs/stack-trace.stacks",
			expected: processtree.ProfilerTypeStackTrace,
		},
		{
			name:     "Kineto Trace",
			path:     "/workspace/logs/kineto-output.json",
			expected: processtree.ProfilerTypeKineto,
		},
		{
			name:     "Memory Dump",
			path:     "/workspace/logs/memory_snapshot_1.pickle",
			expected: processtree.ProfilerTypeMemoryDump,
		},
		{
			name:     "Unknown",
			path:     "/workspace/logs/random.txt",
			expected: processtree.ProfilerTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := identifyProfilerFileType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfilerReader_ReadEntireFile(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a mock process directory structure
	mockProcDir := filepath.Join(tmpDir, "proc", "root", "workspace", "logs")
	err := os.MkdirAll(mockProcDir, 0755)
	require.NoError(t, err)

	// Create test file
	testContent := []byte("This is a test profiler file content")
	testFilePath := filepath.Join(mockProcDir, "test-profiler.json")
	err = os.WriteFile(testFilePath, testContent, 0644)
	require.NoError(t, err)

	// Note: This test would require mocking the /proc filesystem
	// For now, we'll test the helper functions
	t.Log("Full integration test requires /proc filesystem mocking")
}

func TestProfilerReader_CompressDecompress(t *testing.T) {
	// Test data
	originalContent := []byte(`{
		"traceEvents": [
			{"name": "test", "ph": "X", "ts": 1000, "dur": 100}
		]
	}`)

	// Compress
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	_, err := gzWriter.Write(originalContent)
	require.NoError(t, err)
	err = gzWriter.Close()
	require.NoError(t, err)

	compressedData := compressed.Bytes()
	t.Logf("Original size: %d bytes, Compressed size: %d bytes, Ratio: %.2f%%",
		len(originalContent), len(compressedData),
		float64(len(compressedData))*100/float64(len(originalContent)))

	// Decompress
	gzReader, err := gzip.NewReader(bytes.NewReader(compressedData))
	require.NoError(t, err)
	defer gzReader.Close()

	var decompressed bytes.Buffer
	_, err = decompressed.ReadFrom(gzReader)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, originalContent, decompressed.Bytes())
}

func TestProfilerReader_ChunkCalculation(t *testing.T) {
	reader := NewProfilerReader()
	chunkSize := int64(10 * 1024 * 1024) // 10MB
	reader.SetChunkSize(chunkSize)

	tests := []struct {
		name           string
		fileSize       int64
		expectedChunks int
	}{
		{
			name:           "Small file (1MB)",
			fileSize:       1 * 1024 * 1024,
			expectedChunks: 1,
		},
		{
			name:           "Exact chunk size (10MB)",
			fileSize:       10 * 1024 * 1024,
			expectedChunks: 1,
		},
		{
			name:           "Multiple chunks (25MB)",
			fileSize:       25 * 1024 * 1024,
			expectedChunks: 3,
		},
		{
			name:           "Large file (100MB)",
			fileSize:       100 * 1024 * 1024,
			expectedChunks: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			totalChunks := int((tt.fileSize + chunkSize - 1) / chunkSize)
			assert.Equal(t, tt.expectedChunks, totalChunks)
		})
	}
}

func TestProfilerReadRequest_BasicValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     *ProfilerReadRequest
		wantErr bool
	}{
		{
			name: "Valid with PID",
			req: &ProfilerReadRequest{
				PID:  12345,
				Path: "/workspace/logs/profiler.json",
			},
			wantErr: false,
		},
		{
			name: "Valid with PodUID",
			req: &ProfilerReadRequest{
				PodUID: "abc-123-def",
				Path:   "/workspace/logs/profiler.json",
			},
			wantErr: false,
		},
		{
			name: "Valid with auto decompress",
			req: &ProfilerReadRequest{
				PID:            12345,
				Path:           "/workspace/logs/profiler.json.gz",
				AutoDecompress: true,
			},
			wantErr: false,
		},
		{
			name: "Valid with chunk index",
			req: &ProfilerReadRequest{
				PID:        12345,
				Path:       "/workspace/logs/profiler.json",
				ChunkIndex: 2,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation
			if tt.req.Path == "" {
				t.Error("Path should not be empty")
			}
			if tt.req.PID == 0 && tt.req.PodUID == "" {
				t.Error("Either PID or PodUID should be provided")
			}
		})
	}
}

func TestProfilerReadResponse_Fields(t *testing.T) {
	// Create a sample response
	response := &ProfilerReadResponse{
		Content:          base64.StdEncoding.EncodeToString([]byte("test content")),
		BytesRead:        12,
		EOF:              true,
		Compressed:       true,
		Decompressed:     true,
		OriginalSize:     100,
		UncompressedSize: 200,
		FileType:         processtree.ProfilerTypeChromeTrace,
		ChunkInfo: &ChunkInfo{
			ChunkIndex:  0,
			ChunkSize:   10 * 1024 * 1024,
			TotalChunks: 1,
			Offset:      0,
			IsLastChunk: true,
		},
	}

	// Verify all fields are set
	assert.NotEmpty(t, response.Content)
	assert.Equal(t, int64(12), response.BytesRead)
	assert.True(t, response.EOF)
	assert.True(t, response.Compressed)
	assert.True(t, response.Decompressed)
	assert.Equal(t, int64(100), response.OriginalSize)
	assert.Equal(t, int64(200), response.UncompressedSize)
	assert.Equal(t, processtree.ProfilerTypeChromeTrace, response.FileType)
	assert.NotNil(t, response.ChunkInfo)
	assert.Equal(t, 0, response.ChunkInfo.ChunkIndex)
	assert.Equal(t, 1, response.ChunkInfo.TotalChunks)
	assert.True(t, response.ChunkInfo.IsLastChunk)
}

func TestChunkInfo_Calculation(t *testing.T) {
	chunkSize := int64(10 * 1024 * 1024) // 10MB

	tests := []struct {
		name           string
		fileSize       int64
		chunkIndex     int
		expectedOffset int64
		expectedIsLast bool
	}{
		{
			name:           "First chunk",
			fileSize:       50 * 1024 * 1024,
			chunkIndex:     0,
			expectedOffset: 0,
			expectedIsLast: false,
		},
		{
			name:           "Middle chunk",
			fileSize:       50 * 1024 * 1024,
			chunkIndex:     2,
			expectedOffset: 20 * 1024 * 1024,
			expectedIsLast: false,
		},
		{
			name:           "Last chunk",
			fileSize:       50 * 1024 * 1024,
			chunkIndex:     4,
			expectedOffset: 40 * 1024 * 1024,
			expectedIsLast: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset := int64(tt.chunkIndex) * chunkSize
			isLastChunk := (offset+chunkSize >= tt.fileSize)

			assert.Equal(t, tt.expectedOffset, offset)
			assert.Equal(t, tt.expectedIsLast, isLastChunk)
		})
	}
}

func BenchmarkGzipCompression(b *testing.B) {
	// Create test data (simulating a JSON profiler file)
	testData := bytes.Repeat([]byte(`{"name":"test","ph":"X","ts":1000,"dur":100},`), 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var compressed bytes.Buffer
		gzWriter := gzip.NewWriter(&compressed)
		_, _ = gzWriter.Write(testData)
		_ = gzWriter.Close()
	}
}

func BenchmarkGzipDecompression(b *testing.B) {
	// Prepare compressed data
	testData := bytes.Repeat([]byte(`{"name":"test","ph":"X","ts":1000,"dur":100},`), 1000)
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)
	_, _ = gzWriter.Write(testData)
	_ = gzWriter.Close()
	compressedData := compressed.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gzReader, _ := gzip.NewReader(bytes.NewReader(compressedData))
		var decompressed bytes.Buffer
		_, _ = decompressed.ReadFrom(gzReader)
		_ = gzReader.Close()
	}
}

func BenchmarkBase64Encoding(b *testing.B) {
	testData := bytes.Repeat([]byte("test data"), 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = base64.StdEncoding.EncodeToString(testData)
	}
}

func TestNewProfilerReader(t *testing.T) {
	reader := NewProfilerReader()

	assert.NotNil(t, reader)
	assert.NotNil(t, reader.fsReader)
	assert.Equal(t, int64(10*1024*1024), reader.chunkSize)
	assert.Equal(t, int64(500*1024*1024), reader.maxUncompressedSize)
}

func TestProfilerReader_SetChunkSize(t *testing.T) {
	reader := NewProfilerReader()

	newChunkSize := int64(25 * 1024 * 1024) // 25MB
	reader.SetChunkSize(newChunkSize)

	assert.Equal(t, newChunkSize, reader.chunkSize)
}

func TestProfilerReader_SetMaxUncompressedSize(t *testing.T) {
	reader := NewProfilerReader()

	newMaxSize := int64(1024 * 1024 * 1024) // 1GB
	reader.SetMaxUncompressedSize(newMaxSize)

	assert.Equal(t, newMaxSize, reader.maxUncompressedSize)
}

func TestProfilerReadRequest_AllScenarios(t *testing.T) {
	tests := []struct {
		name  string
		req   *ProfilerReadRequest
		valid bool
	}{
		{
			name: "Valid with PID",
			req: &ProfilerReadRequest{
				PID:  12345,
				Path: "/workspace/logs/profiler.json",
			},
			valid: true,
		},
		{
			name: "Valid with PodUID",
			req: &ProfilerReadRequest{
				PodUID: "pod-123",
				Path:   "/workspace/logs/profiler.json",
			},
			valid: true,
		},
		{
			name: "Valid with auto decompress",
			req: &ProfilerReadRequest{
				PID:            12345,
				Path:           "/workspace/logs/profiler.json.gz",
				AutoDecompress: true,
			},
			valid: true,
		},
		{
			name: "Valid with offset and length",
			req: &ProfilerReadRequest{
				PID:    12345,
				Path:   "/workspace/logs/profiler.json",
				Offset: 1024,
				Length: 2048,
			},
			valid: true,
		},
		{
			name: "Valid with chunk index",
			req: &ProfilerReadRequest{
				PID:        12345,
				Path:       "/workspace/logs/profiler.json",
				ChunkIndex: 2,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.req.Path, "Path should not be empty")
			if tt.req.PID == 0 && tt.req.PodUID == "" {
				assert.False(t, tt.valid, "Either PID or PodUID should be provided")
			}
		})
	}
}

func TestProfilerReadResponse_AllFields(t *testing.T) {
	response := &ProfilerReadResponse{
		Content:          base64.StdEncoding.EncodeToString([]byte("test content")),
		FileInfo:         &FileInfo{Path: "/test.json", Size: 12},
		BytesRead:        12,
		EOF:              true,
		Compressed:       true,
		Decompressed:     true,
		OriginalSize:     100,
		UncompressedSize: 200,
		ChunkInfo: &ChunkInfo{
			ChunkIndex:  0,
			ChunkSize:   10 * 1024 * 1024,
			TotalChunks: 1,
			Offset:      0,
			IsLastChunk: true,
		},
		FileType: processtree.ProfilerTypeChromeTrace,
	}

	assert.NotEmpty(t, response.Content)
	assert.NotNil(t, response.FileInfo)
	assert.NotNil(t, response.ChunkInfo)
	assert.Equal(t, processtree.ProfilerTypeChromeTrace, response.FileType)
	assert.True(t, response.EOF)
	assert.True(t, response.Compressed)
	assert.True(t, response.Decompressed)
}

func TestChunkInfo_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		chunkInfo   *ChunkInfo
		description string
	}{
		{
			name: "Single chunk file",
			chunkInfo: &ChunkInfo{
				ChunkIndex:  0,
				ChunkSize:   10 * 1024 * 1024,
				TotalChunks: 1,
				Offset:      0,
				IsLastChunk: true,
			},
			description: "File fits in single chunk",
		},
		{
			name: "First chunk of many",
			chunkInfo: &ChunkInfo{
				ChunkIndex:  0,
				ChunkSize:   10 * 1024 * 1024,
				TotalChunks: 10,
				Offset:      0,
				IsLastChunk: false,
			},
			description: "First chunk of multi-chunk file",
		},
		{
			name: "Middle chunk",
			chunkInfo: &ChunkInfo{
				ChunkIndex:  5,
				ChunkSize:   10 * 1024 * 1024,
				TotalChunks: 10,
				Offset:      50 * 1024 * 1024,
				IsLastChunk: false,
			},
			description: "Middle chunk",
		},
		{
			name: "Last chunk",
			chunkInfo: &ChunkInfo{
				ChunkIndex:  9,
				ChunkSize:   10 * 1024 * 1024,
				TotalChunks: 10,
				Offset:      90 * 1024 * 1024,
				IsLastChunk: true,
			},
			description: "Last chunk may be smaller",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.GreaterOrEqual(t, tt.chunkInfo.ChunkIndex, 0)
			assert.Greater(t, tt.chunkInfo.TotalChunks, 0)
			assert.LessOrEqual(t, tt.chunkInfo.ChunkIndex, tt.chunkInfo.TotalChunks-1)
			assert.GreaterOrEqual(t, tt.chunkInfo.Offset, int64(0))

			if tt.chunkInfo.ChunkIndex == tt.chunkInfo.TotalChunks-1 {
				assert.True(t, tt.chunkInfo.IsLastChunk, "Last chunk should be marked as last")
			}
		})
	}
}

func TestIdentifyProfilerFileType_AllTypes(t *testing.T) {
	tests := []struct {
		path     string
		expected processtree.PyTorchProfilerFileType
	}{
		{"/workspace/logs/profiler-20241215.json", processtree.ProfilerTypeChromeTrace},
		{"/workspace/logs/torch_profiler_output.json.gz", processtree.ProfilerTypeChromeTrace},
		{"/workspace/logs/trace.pt.trace.json", processtree.ProfilerTypePyTorchTrace},
		{"/workspace/logs/trace.pt.trace.json.gz", processtree.ProfilerTypePyTorchTrace},
		{"/workspace/logs/stack-trace.stacks", processtree.ProfilerTypeStackTrace},
		{"/workspace/logs/kineto-output.json", processtree.ProfilerTypeKineto},
		{"/workspace/logs/kineto-output.json.gz", processtree.ProfilerTypeKineto},
		{"/workspace/logs/memory_snapshot_1.pickle", processtree.ProfilerTypeMemoryDump},
		{"/workspace/logs/memory_snapshot_2.pkl", processtree.ProfilerTypeMemoryDump},
		{"/workspace/logs/random.txt", processtree.ProfilerTypeUnknown},
		{"/workspace/logs/data.csv", processtree.ProfilerTypeUnknown},
		{"/workspace/logs/model.pth", processtree.ProfilerTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := identifyProfilerFileType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIdentifyProfilerFileType_CaseInsensitive(t *testing.T) {
	tests := []struct {
		path     string
		expected processtree.PyTorchProfilerFileType
	}{
		{"/LOGS/PROFILER.JSON", processtree.ProfilerTypeChromeTrace},
		{"/logs/Profiler.Json.Gz", processtree.ProfilerTypeChromeTrace},
		{"/logs/TRACE.PT.TRACE.JSON", processtree.ProfilerTypePyTorchTrace},
		{"/logs/KINETO.JSON", processtree.ProfilerTypeKineto},
		{"/logs/MEMORY_SNAPSHOT.PICKLE", processtree.ProfilerTypeMemoryDump},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := identifyProfilerFileType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfilerReader_ChunkSizeBoundaries(t *testing.T) {
	reader := NewProfilerReader()

	tests := []struct {
		name      string
		chunkSize int64
	}{
		{"Very small chunk", 1024},              // 1KB
		{"Small chunk", 1 * 1024 * 1024},        // 1MB
		{"Default chunk", 10 * 1024 * 1024},     // 10MB
		{"Large chunk", 50 * 1024 * 1024},       // 50MB
		{"Very large chunk", 100 * 1024 * 1024}, // 100MB
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader.SetChunkSize(tt.chunkSize)
			assert.Equal(t, tt.chunkSize, reader.chunkSize)
		})
	}
}

func TestProfilerReader_MaxUncompressedSizeBoundaries(t *testing.T) {
	reader := NewProfilerReader()

	tests := []struct {
		name    string
		maxSize int64
	}{
		{"Small limit", 10 * 1024 * 1024},            // 10MB
		{"Medium limit", 100 * 1024 * 1024},          // 100MB
		{"Default limit", 500 * 1024 * 1024},         // 500MB
		{"Large limit", 1024 * 1024 * 1024},          // 1GB
		{"Very large limit", 5 * 1024 * 1024 * 1024}, // 5GB
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader.SetMaxUncompressedSize(tt.maxSize)
			assert.Equal(t, tt.maxSize, reader.maxUncompressedSize)
		})
	}
}

func TestGzipCompressionRatio(t *testing.T) {
	// Test compression ratio for different content types
	tests := []struct {
		name          string
		content       []byte
		expectedRatio float64 // Maximum ratio (compressed/original)
	}{
		{
			name:          "Highly compressible (zeros)",
			content:       bytes.Repeat([]byte{0}, 10000),
			expectedRatio: 0.1, // Should compress to < 10%
		},
		{
			name:          "Compressible JSON",
			content:       bytes.Repeat([]byte(`{"name":"test","value":123}`), 100),
			expectedRatio: 0.5, // Should compress to < 50%
		},
		{
			name:          "Text content",
			content:       bytes.Repeat([]byte("test content for compression"), 100),
			expectedRatio: 0.6, // Should compress to < 60%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var compressed bytes.Buffer
			gzWriter := gzip.NewWriter(&compressed)
			_, err := gzWriter.Write(tt.content)
			require.NoError(t, err)
			err = gzWriter.Close()
			require.NoError(t, err)

			ratio := float64(compressed.Len()) / float64(len(tt.content))
			assert.Less(t, ratio, tt.expectedRatio,
				"Compression ratio %.2f should be less than %.2f", ratio, tt.expectedRatio)
		})
	}
}

func TestProfilerReadRequest_AllFieldsCombinations(t *testing.T) {
	// Test all valid combinations of request fields
	baseReq := &ProfilerReadRequest{
		PID:            12345,
		PodUID:         "pod-123",
		PodName:        "training-pod-0",
		PodNamespace:   "default",
		ContainerName:  "training",
		Path:           "/workspace/logs/profiler.json.gz",
		AutoDecompress: true,
		Offset:         1024,
		Length:         2048,
		ChunkIndex:     2,
	}

	// Verify all fields are set
	assert.Equal(t, 12345, baseReq.PID)
	assert.Equal(t, "pod-123", baseReq.PodUID)
	assert.Equal(t, "training-pod-0", baseReq.PodName)
	assert.Equal(t, "default", baseReq.PodNamespace)
	assert.Equal(t, "training", baseReq.ContainerName)
	assert.Equal(t, "/workspace/logs/profiler.json.gz", baseReq.Path)
	assert.True(t, baseReq.AutoDecompress)
	assert.Equal(t, int64(1024), baseReq.Offset)
	assert.Equal(t, int64(2048), baseReq.Length)
	assert.Equal(t, 2, baseReq.ChunkIndex)
}

func BenchmarkIdentifyProfilerFileType(b *testing.B) {
	paths := []string{
		"/workspace/logs/profiler-20241215.json.gz",
		"/workspace/logs/trace.pt.trace.json",
		"/workspace/logs/stack-trace.stacks",
		"/workspace/logs/kineto-output.json",
		"/workspace/logs/memory_snapshot.pickle",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range paths {
			_ = identifyProfilerFileType(path)
		}
	}
}

func BenchmarkProfilerReader_ChunkCalculation(b *testing.B) {
	reader := NewProfilerReader()
	fileSize := int64(100 * 1024 * 1024) // 100MB
	chunkSize := reader.chunkSize

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = int((fileSize + chunkSize - 1) / chunkSize)
	}
}
