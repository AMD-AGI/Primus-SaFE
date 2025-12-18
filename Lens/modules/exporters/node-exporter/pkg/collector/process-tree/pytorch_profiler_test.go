package processtree

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIdentifyProfilerFile(t *testing.T) {
	reader := NewProcReader()

	tests := []struct {
		name           string
		filePath       string
		expectedType   PyTorchProfilerFileType
		expectedConf   string
		shouldIdentify bool
	}{
		{
			name:           "Chrome Trace with profiler keyword",
			filePath:       "/workspace/logs/profiler-20241215.json.gz",
			expectedType:   ProfilerTypeChromeTrace,
			expectedConf:   "high",
			shouldIdentify: true,
		},
		{
			name:           "PyTorch Trace format",
			filePath:       "/workspace/logs/trace.pt.trace.json",
			expectedType:   ProfilerTypePyTorchTrace,
			expectedConf:   "high",
			shouldIdentify: true,
		},
		{
			name:           "Stack trace file",
			filePath:       "/workspace/logs/stack-trace.stacks",
			expectedType:   ProfilerTypeStackTrace,
			expectedConf:   "high",
			shouldIdentify: true,
		},
		{
			name:           "Kineto trace",
			filePath:       "/workspace/logs/kineto-trace.json",
			expectedType:   ProfilerTypeKineto,
			expectedConf:   "high",
			shouldIdentify: true,
		},
		{
			name:           "Memory snapshot pickle",
			filePath:       "/workspace/logs/memory_snapshot_123.pickle",
			expectedType:   ProfilerTypeMemoryDump,
			expectedConf:   "medium",
			shouldIdentify: true,
		},
		{
			name:           "Chrome Trace in profiler directory",
			filePath:       "/workspace/profiler/trace-123.json",
			expectedType:   ProfilerTypeChromeTrace,
			expectedConf:   "medium",
			shouldIdentify: true,
		},
		{
			name:           "Regular JSON file",
			filePath:       "/workspace/config.json",
			expectedType:   ProfilerTypeUnknown,
			expectedConf:   "",
			shouldIdentify: false,
		},
		{
			name:           "Regular text file",
			filePath:       "/workspace/readme.txt",
			expectedType:   ProfilerTypeUnknown,
			expectedConf:   "",
			shouldIdentify: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file with the expected filename pattern
			tmpDir := t.TempDir()
			// Extract the filename from the expected path to use in temp file
			expectedFileName := filepath.Base(tt.filePath)
			testFile := filepath.Join(tmpDir, expectedFileName)
			// Create file with enough content to pass size check (>1KB)
			content := make([]byte, 2*1024) // 2KB
			err := os.WriteFile(testFile, content, 0644)
			assert.NoError(t, err)

			// Use the temp file path for identification (filename matches pattern)
			fileInfo := reader.identifyProfilerFile(12345, "10", testFile)

			if tt.shouldIdentify {
				assert.NotNil(t, fileInfo, "Expected file to be identified for: %s", tt.name)
				if fileInfo != nil {
					assert.Equal(t, 12345, fileInfo.PID)
					assert.Equal(t, "10", fileInfo.FD)
					assert.Equal(t, tt.expectedType, fileInfo.FileType, "File type mismatch for: %s", tt.name)
					assert.Equal(t, tt.expectedConf, fileInfo.Confidence, "Confidence mismatch for: %s", tt.name)
				}
			} else {
				assert.Nil(t, fileInfo, "Expected file NOT to be identified for: %s", tt.name)
			}
		})
	}
}

func TestIdentifyProfilerFilePattern(t *testing.T) {
	reader := NewProcReader()

	// Create temp file for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "profiler-test.json.gz")
	err := os.WriteFile(testFile, make([]byte, 10*1024), 0644) // 10KB file
	assert.NoError(t, err)

	fileInfo := reader.identifyProfilerFile(999, "5", testFile)
	assert.NotNil(t, fileInfo)
	assert.Equal(t, 999, fileInfo.PID)
	assert.Equal(t, "5", fileInfo.FD)
	assert.Equal(t, ProfilerTypeChromeTrace, fileInfo.FileType)
	assert.Equal(t, "high", fileInfo.Confidence)
	assert.Greater(t, fileInfo.FileSize, int64(0))
}

func TestIdentifyProfilerFileEdgeCases(t *testing.T) {
	reader := NewProcReader()

	t.Run("File too small", func(t *testing.T) {
		tmpDir := t.TempDir()
		smallFile := filepath.Join(tmpDir, "profiler-small.json")
		err := os.WriteFile(smallFile, []byte("x"), 0644) // 1 byte
		assert.NoError(t, err)

		fileInfo := reader.identifyProfilerFile(123, "1", smallFile)
		assert.Nil(t, fileInfo) // Should reject files < 1KB
	})

	t.Run("Non-existent file", func(t *testing.T) {
		fileInfo := reader.identifyProfilerFile(123, "1", "/nonexistent/profiler.json")
		assert.Nil(t, fileInfo)
	})
}

func TestScanPyTorchProfilerFiles(t *testing.T) {
	reader := NewProcReader()

	// This test requires actual PIDs and /proc access
	// For now, test with empty PID list
	files := reader.ScanPyTorchProfilerFiles([]int{})
	// Empty result is expected (nil or empty slice)
	assert.True(t, files == nil || len(files) == 0, "Expected nil or empty slice for empty PID list")

	// Test with non-existent PID
	files = reader.ScanPyTorchProfilerFiles([]int{999999})
	// Should return nil or empty list for non-existent PID, not error
	assert.True(t, files == nil || len(files) == 0, "Expected nil or empty slice for non-existent PID")
}

func TestPyTorchProfilerFileInfo(t *testing.T) {
	now := time.Now()
	fileInfo := &PyTorchProfilerFileInfo{
		PID:        12345,
		FD:         "10",
		FilePath:   "/workspace/profiler.json",
		FileName:   "profiler.json",
		FileType:   ProfilerTypeChromeTrace,
		FileSize:   1024000,
		Confidence: "high",
		DetectedAt: now,
	}

	assert.Equal(t, 12345, fileInfo.PID)
	assert.Equal(t, "10", fileInfo.FD)
	assert.Equal(t, ProfilerTypeChromeTrace, fileInfo.FileType)
	assert.Equal(t, "high", fileInfo.Confidence)
	assert.Equal(t, int64(1024000), fileInfo.FileSize)
}

func TestPyTorchProfilerFileTypes(t *testing.T) {
	assert.Equal(t, "chrome_trace", string(ProfilerTypeChromeTrace))
	assert.Equal(t, "pytorch_trace", string(ProfilerTypePyTorchTrace))
	assert.Equal(t, "stack_trace", string(ProfilerTypeStackTrace))
	assert.Equal(t, "memory_dump", string(ProfilerTypeMemoryDump))
	assert.Equal(t, "kineto", string(ProfilerTypeKineto))
	assert.Equal(t, "unknown", string(ProfilerTypeUnknown))
}
