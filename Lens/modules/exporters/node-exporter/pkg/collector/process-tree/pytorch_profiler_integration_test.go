// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// +build integration

package processtree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorFindPyTorchProfilerFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	collector := &Collector{
		procReader: NewProcReader(),
		cacheTTL:   5 * time.Minute,
	}

	ctx := context.Background()

	t.Run("Empty pod UID", func(t *testing.T) {
		_, err := collector.FindPyTorchProfilerFiles(ctx, "", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no containers found")
	})

	t.Run("Non-existent pod", func(t *testing.T) {
		_, err := collector.FindPyTorchProfilerFiles(ctx, "non-existent-pod-uid", "test-pod", "default")
		assert.Error(t, err)
	})
}

func TestEndToEndProfilerFileDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a real pod with profiler files
	// Skip if TEST_POD_UID is not set
	podUID := os.Getenv("TEST_POD_UID")
	if podUID == "" {
		t.Skip("TEST_POD_UID not set, skipping end-to-end test")
	}

	collector := &Collector{
		procReader: NewProcReader(),
		cacheTTL:   5 * time.Minute,
	}

	ctx := context.Background()
	response, err := collector.FindPyTorchProfilerFiles(ctx, podUID, "test-pod", "default")

	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, podUID, response.PodUID)
	assert.GreaterOrEqual(t, response.TotalProcesses, 0)
	assert.NotNil(t, response.Files)
}

func TestProfilerFileDiscoveryWithMockProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory structure to simulate profiler files
	tmpDir := t.TempDir()

	// Create mock profiler files
	files := []struct {
		name string
		size int
	}{
		{"profiler-trace-20241215.json.gz", 10 * 1024 * 1024},      // 10MB
		{"torch_trace.pt.trace.json", 5 * 1024 * 1024},             // 5MB
		{"stack-trace.stacks", 2 * 1024 * 1024},                    // 2MB
		{"memory_snapshot_001.pickle", 100 * 1024 * 1024},          // 100MB
		{"kineto-trace-worker0.json", 15 * 1024 * 1024},            // 15MB
	}

	for _, f := range files {
		filePath := filepath.Join(tmpDir, f.name)
		err := os.WriteFile(filePath, make([]byte, f.size), 0644)
		require.NoError(t, err)
	}

	t.Logf("Created mock profiler files in: %s", tmpDir)

	// Verify files were created
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, len(files), len(entries))
}

func TestProfilerFileIdentificationAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	reader := NewProcReader()
	tmpDir := t.TempDir()

	testCases := []struct {
		fileName     string
		fileSize     int
		expectedType PyTorchProfilerFileType
		expectedConf string
	}{
		{
			fileName:     "profiler-20241215-160530.json.gz",
			fileSize:     15 * 1024 * 1024,
			expectedType: ProfilerTypeChromeTrace,
			expectedConf: "high",
		},
		{
			fileName:     "trace_worker_0.pt.trace.json",
			fileSize:     8 * 1024 * 1024,
			expectedType: ProfilerTypePyTorchTrace,
			expectedConf: "high",
		},
		{
			fileName:     "stack_trace_002.stacks",
			fileSize:     3 * 1024 * 1024,
			expectedType: ProfilerTypeStackTrace,
			expectedConf: "high",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.fileName, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, tc.fileName)
			err := os.WriteFile(filePath, make([]byte, tc.fileSize), 0644)
			require.NoError(t, err)

			fileInfo := reader.identifyProfilerFile(999, "10", filePath)
			require.NotNil(t, fileInfo)

			assert.Equal(t, tc.expectedType, fileInfo.FileType)
			assert.Equal(t, tc.expectedConf, fileInfo.Confidence)
			assert.Equal(t, int64(tc.fileSize), fileInfo.FileSize)
			assert.Equal(t, tc.fileName, fileInfo.FileName)
		})
	}
}

func TestConcurrentProfilerFileScanning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	reader := NewProcReader()

	// Test concurrent scanning with empty PID list
	// This ensures thread safety
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			files := reader.ScanPyTorchProfilerFiles([]int{})
			assert.NotNil(t, files)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestProfilerResponseStructure(t *testing.T) {
	response := &PyTorchProfilerFilesResponse{
		PodUID:       "test-pod-uid-123",
		PodName:      "training-pod-0",
		PodNamespace: "default",
		Files: []*PyTorchProfilerFileInfo{
			{
				PID:        12345,
				FD:         "10",
				FilePath:   "/workspace/profiler.json",
				FileName:   "profiler.json",
				FileType:   ProfilerTypeChromeTrace,
				FileSize:   10485760,
				Confidence: "high",
				DetectedAt: time.Now(),
			},
		},
		TotalProcesses: 5,
		CollectedAt:    time.Now(),
	}

	assert.Equal(t, "test-pod-uid-123", response.PodUID)
	assert.Equal(t, "training-pod-0", response.PodName)
	assert.Equal(t, "default", response.PodNamespace)
	assert.Equal(t, 1, len(response.Files))
	assert.Equal(t, 5, response.TotalProcesses)

	file := response.Files[0]
	assert.Equal(t, ProfilerTypeChromeTrace, file.FileType)
	assert.Equal(t, "high", file.Confidence)
	assert.Equal(t, int64(10485760), file.FileSize)
}

