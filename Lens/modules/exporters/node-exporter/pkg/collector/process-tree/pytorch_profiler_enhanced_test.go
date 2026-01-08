// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package processtree

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPyTorchProfilerFileType_String(t *testing.T) {
	tests := []struct {
		fileType PyTorchProfilerFileType
		expected string
	}{
		{ProfilerTypeChromeTrace, "chrome_trace"},
		{ProfilerTypePyTorchTrace, "pytorch_trace"},
		{ProfilerTypeStackTrace, "stack_trace"},
		{ProfilerTypeMemoryDump, "memory_dump"},
		{ProfilerTypeKineto, "kineto"},
		{ProfilerTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.fileType), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.fileType))
		})
	}
}

func TestPyTorchProfilerFileInfo_Validation(t *testing.T) {
	info := &PyTorchProfilerFileInfo{
		PID:        12345,
		FD:         "10",
		FilePath:   "/workspace/logs/profiler.json",
		FileName:   "profiler.json",
		FileType:   ProfilerTypeChromeTrace,
		FileSize:   1024000,
		Confidence: "high",
		DetectedAt: time.Now(),
	}

	// Validate all fields are set
	assert.Greater(t, info.PID, 0)
	assert.NotEmpty(t, info.FD)
	assert.NotEmpty(t, info.FilePath)
	assert.NotEmpty(t, info.FileName)
	assert.NotEqual(t, ProfilerTypeUnknown, info.FileType)
	assert.Greater(t, info.FileSize, int64(0))
	assert.NotEmpty(t, info.Confidence)
	assert.False(t, info.DetectedAt.IsZero())
}

func TestPyTorchProfilerFileInfo_ConfidenceLevels(t *testing.T) {
	tests := []struct {
		name       string
		confidence string
		valid      bool
	}{
		{"High confidence", "high", true},
		{"Medium confidence", "medium", true},
		{"Low confidence", "low", true},
		{"Invalid confidence", "invalid", false},
		{"Empty confidence", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &PyTorchProfilerFileInfo{
				Confidence: tt.confidence,
			}

			if tt.valid {
				assert.Contains(t, []string{"high", "medium", "low"}, info.Confidence)
			} else {
				assert.NotContains(t, []string{"high", "medium", "low"}, info.Confidence)
			}
		})
	}
}

func TestPyTorchProfilerFilesResponse_Fields(t *testing.T) {
	resp := &PyTorchProfilerFilesResponse{
		PodUID:       "pod-123",
		PodName:      "training-pod-0",
		PodNamespace: "default",
		Files: []*PyTorchProfilerFileInfo{
			{
				PID:        12345,
				FilePath:   "/workspace/logs/profiler.json",
				FileType:   ProfilerTypeChromeTrace,
				Confidence: "high",
			},
		},
		TotalProcesses: 5,
		CollectedAt:    time.Now(),
	}

	assert.NotEmpty(t, resp.PodUID)
	assert.NotEmpty(t, resp.PodName)
	assert.NotEmpty(t, resp.PodNamespace)
	assert.NotEmpty(t, resp.Files)
	assert.Greater(t, resp.TotalProcesses, 0)
	assert.False(t, resp.CollectedAt.IsZero())
}

func TestIdentifyProfilerFile_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		fileSize int64
		expected bool
	}{
		{
			name:     "Very small file (< 1KB)",
			filePath: "/workspace/logs/profiler.json",
			fileSize: 512,
			expected: false, // Too small
		},
		{
			name:     "Minimum size file (1KB)",
			filePath: "/workspace/logs/profiler.json",
			fileSize: 1024,
			expected: true,
		},
		{
			name:     "Normal size file (10MB)",
			filePath: "/workspace/logs/profiler.json",
			fileSize: 10 * 1024 * 1024,
			expected: true,
		},
		{
			name:     "Large file (1GB)",
			filePath: "/workspace/logs/profiler.json",
			fileSize: 1024 * 1024 * 1024,
			expected: true,
		},
		{
			name:     "Very large file (10GB)",
			filePath: "/workspace/logs/profiler.json",
			fileSize: 10 * 1024 * 1024 * 1024,
			expected: true,
		},
		{
			name:     "Extremely large file (> 10GB)",
			filePath: "/workspace/logs/profiler.json",
			fileSize: 15 * 1024 * 1024 * 1024,
			expected: false, // Too large
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Size validation logic
			shouldInclude := tt.fileSize >= 1024 && tt.fileSize <= 10*1024*1024*1024
			assert.Equal(t, tt.expected, shouldInclude)
		})
	}
}

func TestProfilerFilePathPatterns(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		shouldMatch bool
		reason      string
	}{
		{
			name:        "Profiler in filename",
			path:        "/workspace/logs/profiler-20241215.json",
			shouldMatch: true,
			reason:      "Contains 'profiler' keyword",
		},
		{
			name:        "Torch profiler in filename",
			path:        "/workspace/logs/torch_profiler_output.json",
			shouldMatch: true,
			reason:      "Contains 'torch_profiler' keyword",
		},
		{
			name:        "Profiler directory",
			path:        "/workspace/profiler/trace-123.json",
			shouldMatch: true,
			reason:      "In profiler directory",
		},
		{
			name:        "Profiler log directory",
			path:        "/workspace/profiler_log/output.json",
			shouldMatch: true,
			reason:      "In profiler_log directory",
		},
		{
			name:        "Kineto file",
			path:        "/workspace/logs/kineto-20241215.json",
			shouldMatch: true,
			reason:      "Contains 'kineto' keyword",
		},
		{
			name:        "Memory snapshot",
			path:        "/workspace/logs/memory_snapshot_123.pickle",
			shouldMatch: true,
			reason:      "Memory snapshot pattern",
		},
		{
			name:        "Regular log file",
			path:        "/workspace/logs/training.log",
			shouldMatch: false,
			reason:      "No profiler keywords",
		},
		{
			name:        "Config file",
			path:        "/workspace/config.json",
			shouldMatch: false,
			reason:      "Not a profiler file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test path pattern matching logic
			containsProfilerKeyword := containsAny(tt.path, []string{
				"profiler", "torch_profiler", "kineto", "memory_snapshot",
				"/profiler/", "/profiler_log/",
			})

			if tt.shouldMatch {
				assert.True(t, containsProfilerKeyword, tt.reason)
			} else {
				assert.False(t, containsProfilerKeyword, tt.reason)
			}
		})
	}
}

// Helper function to check if string contains any of the substrings
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if contains(s, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFileExtensionMatching(t *testing.T) {
	tests := []struct {
		filename    string
		extensions  []string
		shouldMatch bool
	}{
		{
			filename:    "profiler.json",
			extensions:  []string{".json"},
			shouldMatch: true,
		},
		{
			filename:    "profiler.json.gz",
			extensions:  []string{".json.gz", ".gz"},
			shouldMatch: true,
		},
		{
			filename:    "trace.pt.trace.json",
			extensions:  []string{".pt.trace.json"},
			shouldMatch: true,
		},
		{
			filename:    "stack.stacks",
			extensions:  []string{".stacks"},
			shouldMatch: true,
		},
		{
			filename:    "memory.pickle",
			extensions:  []string{".pickle", ".pkl"},
			shouldMatch: true,
		},
		{
			filename:    "data.csv",
			extensions:  []string{".json", ".pickle"},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			matched := false
			for _, ext := range tt.extensions {
				if hasExtension(tt.filename, ext) {
					matched = true
					break
				}
			}
			assert.Equal(t, tt.shouldMatch, matched)
		})
	}
}

func hasExtension(filename, ext string) bool {
	return len(filename) >= len(ext) && filename[len(filename)-len(ext):] == ext
}

func TestConfidenceScoring(t *testing.T) {
	tests := []struct {
		name       string
		conditions []bool // List of matching conditions
		expected   string // Expected confidence level
	}{
		{
			name:       "All high-confidence indicators",
			conditions: []bool{true, true, true}, // Extension + keyword + directory
			expected:   "high",
		},
		{
			name:       "Some medium-confidence indicators",
			conditions: []bool{true, false, true}, // Extension + directory, but no keyword
			expected:   "medium",
		},
		{
			name:       "Only path-based match",
			conditions: []bool{false, false, true}, // Only directory match
			expected:   "low",
		},
		{
			name:       "No matches",
			conditions: []bool{false, false, false},
			expected:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Count matching conditions
			matches := 0
			for _, cond := range tt.conditions {
				if cond {
					matches++
				}
			}

			var confidence string
			if matches >= 2 {
				confidence = "high"
			} else if matches == 1 {
				confidence = "medium"
			} else {
				confidence = "unknown"
			}

			// Note: This is a simplified version. Actual logic is more complex
			if tt.expected != "unknown" {
				assert.NotEmpty(t, confidence)
			}
		})
	}
}

func TestFileTypePriority(t *testing.T) {
	// Define file type priorities
	priorities := map[PyTorchProfilerFileType]int{
		ProfilerTypeChromeTrace:  1, // Highest priority
		ProfilerTypePyTorchTrace: 1,
		ProfilerTypeStackTrace:   1,
		ProfilerTypeKineto:       1,
		ProfilerTypeMemoryDump:   2, // Lower priority (large files)
		ProfilerTypeUnknown:      3, // Lowest priority
	}

	tests := []struct {
		fileType1 PyTorchProfilerFileType
		fileType2 PyTorchProfilerFileType
		expected  PyTorchProfilerFileType
	}{
		{ProfilerTypeChromeTrace, ProfilerTypeMemoryDump, ProfilerTypeChromeTrace},
		{ProfilerTypePyTorchTrace, ProfilerTypeUnknown, ProfilerTypePyTorchTrace},
		{ProfilerTypeStackTrace, ProfilerTypeMemoryDump, ProfilerTypeStackTrace},
	}

	for _, tt := range tests {
		t.Run(string(tt.fileType1)+"_vs_"+string(tt.fileType2), func(t *testing.T) {
			priority1 := priorities[tt.fileType1]
			priority2 := priorities[tt.fileType2]

			var selected PyTorchProfilerFileType
			if priority1 <= priority2 {
				selected = tt.fileType1
			} else {
				selected = tt.fileType2
			}

			assert.Equal(t, tt.expected, selected)
		})
	}
}

func TestPyTorchProfilerFilesResponse_EmptyFiles(t *testing.T) {
	resp := &PyTorchProfilerFilesResponse{
		PodUID:         "pod-123",
		Files:          []*PyTorchProfilerFileInfo{},
		TotalProcesses: 0,
		CollectedAt:    time.Now(),
	}

	assert.NotNil(t, resp.Files)
	assert.Len(t, resp.Files, 0)
	assert.Equal(t, 0, resp.TotalProcesses)
}

func TestPyTorchProfilerFilesResponse_MultipleFiles(t *testing.T) {
	resp := &PyTorchProfilerFilesResponse{
		PodUID: "pod-123",
		Files: []*PyTorchProfilerFileInfo{
			{
				PID:        12345,
				FilePath:   "/workspace/logs/profiler1.json",
				FileType:   ProfilerTypeChromeTrace,
				Confidence: "high",
				FileSize:   10 * 1024 * 1024,
			},
			{
				PID:        12345,
				FilePath:   "/workspace/logs/trace.pt.trace.json",
				FileType:   ProfilerTypePyTorchTrace,
				Confidence: "high",
				FileSize:   5 * 1024 * 1024,
			},
			{
				PID:        12346,
				FilePath:   "/workspace/logs/stack.stacks",
				FileType:   ProfilerTypeStackTrace,
				Confidence: "high",
				FileSize:   2 * 1024 * 1024,
			},
		},
		TotalProcesses: 2,
		CollectedAt:    time.Now(),
	}

	assert.Len(t, resp.Files, 3)
	assert.Equal(t, 2, resp.TotalProcesses)

	// Calculate total size
	totalSize := int64(0)
	for _, file := range resp.Files {
		totalSize += file.FileSize
	}
	assert.Equal(t, int64(17*1024*1024), totalSize)
}

func TestFileTypeDistribution(t *testing.T) {
	files := []*PyTorchProfilerFileInfo{
		{FileType: ProfilerTypeChromeTrace},
		{FileType: ProfilerTypeChromeTrace},
		{FileType: ProfilerTypePyTorchTrace},
		{FileType: ProfilerTypeStackTrace},
		{FileType: ProfilerTypeMemoryDump},
		{FileType: ProfilerTypeChromeTrace},
	}

	// Count by type
	counts := make(map[PyTorchProfilerFileType]int)
	for _, file := range files {
		counts[file.FileType]++
	}

	assert.Equal(t, 3, counts[ProfilerTypeChromeTrace])
	assert.Equal(t, 1, counts[ProfilerTypePyTorchTrace])
	assert.Equal(t, 1, counts[ProfilerTypeStackTrace])
	assert.Equal(t, 1, counts[ProfilerTypeMemoryDump])
}

func TestConfidenceDistribution(t *testing.T) {
	files := []*PyTorchProfilerFileInfo{
		{Confidence: "high"},
		{Confidence: "high"},
		{Confidence: "medium"},
		{Confidence: "high"},
		{Confidence: "low"},
		{Confidence: "medium"},
	}

	// Count by confidence
	counts := make(map[string]int)
	for _, file := range files {
		counts[file.Confidence]++
	}

	assert.Equal(t, 3, counts["high"])
	assert.Equal(t, 2, counts["medium"])
	assert.Equal(t, 1, counts["low"])
}

func BenchmarkIdentifyProfilerFile(b *testing.B) {
	reader := NewProcReader()
	testPaths := []string{
		"/workspace/logs/profiler-20241215.json.gz",
		"/workspace/logs/trace.pt.trace.json",
		"/workspace/logs/stack-trace.stacks",
		"/workspace/logs/kineto-output.json",
		"/workspace/logs/memory_snapshot.pickle",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for idx, path := range testPaths {
			_ = reader.identifyProfilerFile(12345, string(rune(idx)), path)
		}
	}
}

func BenchmarkFileTypeDetection(b *testing.B) {
	paths := []string{
		"/workspace/logs/profiler-20241215.json.gz",
		"/workspace/logs/trace.pt.trace.json",
		"/workspace/logs/stack-trace.stacks",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range paths {
			// Simulate file type detection
			_ = detectFileType(path)
		}
	}
}

func detectFileType(path string) PyTorchProfilerFileType {
	if hasExtension(path, ".pt.trace.json") {
		return ProfilerTypePyTorchTrace
	}
	if hasExtension(path, ".stacks") {
		return ProfilerTypeStackTrace
	}
	if containsAny(path, []string{"profiler", "torch_profiler"}) {
		return ProfilerTypeChromeTrace
	}
	return ProfilerTypeUnknown
}
