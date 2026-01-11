// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package processtree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProcReader(t *testing.T) {
	reader := NewProcReader()
	assert.NotNil(t, reader)
}

func TestNormalizeContainerID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "containerd prefix",
			input:    "containerd://abc123def456",
			expected: "abc123def456",
		},
		{
			name:     "docker prefix",
			input:    "docker://xyz789",
			expected: "xyz789",
		},
		{
			name:     "cri-o prefix",
			input:    "cri-o://container123",
			expected: "container123",
		},
		{
			name:     "no prefix",
			input:    "barecontainerid",
			expected: "barecontainerid",
		},
		{
			name:     "multiple prefixes",
			input:    "containerd://docker://nested",
			expected: "nested", // Will strip both prefixes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeContainerID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractContainerIDFromCgroup(t *testing.T) {
	tests := []struct {
		name     string
		cgroup   string
		expected string
	}{
		{
			name:     "containerd format",
			cgroup:   "0::/system.slice/cri-containerd-abc123def456.scope",
			expected: "abc123def456",
		},
		{
			name:     "docker format",
			cgroup:   "0::/system.slice/docker-xyz789abc.scope",
			expected: "xyz789abc",
		},
		{
			name:     "cri-o format",
			cgroup:   "0::/system.slice/crio-container123.scope",
			expected: "container123",
		},
		{
			name: "direct container ID",
			cgroup: "11:freezer:/kubepods/burstable/pod123/abc123def456ghi789jkl012mno345pqr678stu901",
			expected: "abc123def456ghi789jkl012mno345pqr678stu901",
		},
		{
			name:     "no container ID",
			cgroup:   "0::/system.slice/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractContainerIDFromCgroup(tt.cgroup)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAlphanumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "alphanumeric only",
			input:    "abc123DEF456",
			expected: true,
		},
		{
			name:     "with hyphen",
			input:    "abc-123",
			expected: false,
		},
		{
			name:     "with underscore",
			input:    "abc_123",
			expected: false,
		},
		{
			name:     "with slash",
			input:    "abc/123",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "numbers only",
			input:    "123456",
			expected: true,
		},
		{
			name:     "letters only",
			input:    "abcdef",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAlphanumeric(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFileName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "full path",
			path:     "/var/log/tensorboard/events.out.tfevents.123",
			expected: "events.out.tfevents.123",
		},
		{
			name:     "relative path",
			path:     "logs/events.out.tfevents.456",
			expected: "events.out.tfevents.456",
		},
		{
			name:     "just filename",
			path:     "events.out.tfevents.789",
			expected: "events.out.tfevents.789",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFileName(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test with real /proc filesystem (only if running on Linux)
func TestGetProcessInfo_Self(t *testing.T) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		t.Skip("Skipping /proc filesystem tests")
	}

	reader := NewProcReader()
	req := &ProcessTreeRequest{
		IncludeCmdline:   true,
		IncludeEnv:       false, // Skip env to avoid permission issues
		IncludeResources: true,
	}

	// Get info for the current process (self)
	pid := os.Getpid()
	info, err := reader.GetProcessInfo(pid, req)

	// If we're not on Linux or /proc is not available, this will fail
	if err != nil {
		t.Skipf("Cannot access /proc filesystem: %v", err)
	}

	require.NoError(t, err)
	assert.Equal(t, pid, info.HostPID)
	assert.NotEmpty(t, info.Comm)
	assert.NotEmpty(t, info.State)
	assert.NotEmpty(t, info.Cmdline)
}

func TestReadStat_Integration(t *testing.T) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		t.Skip("Skipping /proc filesystem tests")
	}

	reader := NewProcReader()
	pid := os.Getpid()

	info := &ProcessInfo{}
	err := reader.readStat(pid, info)

	if err != nil {
		t.Skipf("Cannot access /proc/%d/stat: %v", pid, err)
	}

	require.NoError(t, err)
	assert.NotEmpty(t, info.Comm)
	assert.NotEmpty(t, info.State)
	assert.Greater(t, info.HostPPID, 0)
}

func TestReadCmdline_Integration(t *testing.T) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		t.Skip("Skipping /proc filesystem tests")
	}

	reader := NewProcReader()
	pid := os.Getpid()

	cmdline, err := reader.readCmdline(pid)

	if err != nil {
		t.Skipf("Cannot access /proc/%d/cmdline: %v", pid, err)
	}

	require.NoError(t, err)
	assert.NotEmpty(t, cmdline)
}

func TestScanTensorboardFiles(t *testing.T) {
	reader := NewProcReader()

	t.Run("empty pid list", func(t *testing.T) {
		files := reader.ScanTensorboardFiles([]int{})
		assert.Empty(t, files)
	})

	t.Run("non-existent pids", func(t *testing.T) {
		files := reader.ScanTensorboardFiles([]int{999999, 999998})
		assert.Empty(t, files)
	})
}

func TestGetProcessEnvironment_Integration(t *testing.T) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		t.Skip("Skipping /proc filesystem tests")
	}

	reader := NewProcReader()
	pid := os.Getpid()

	t.Run("without filter", func(t *testing.T) {
		env, cmdline, err := reader.GetProcessEnvironment(pid, "")

		if err != nil {
			t.Skipf("Cannot access /proc/%d/environ: %v", pid, err)
		}

		require.NoError(t, err)
		assert.NotNil(t, env)
		assert.NotEmpty(t, cmdline)
	})

	t.Run("with filter", func(t *testing.T) {
		// Set a test environment variable
		os.Setenv("TEST_VAR_123", "test_value")
		defer os.Unsetenv("TEST_VAR_123")

		env, _, err := reader.GetProcessEnvironment(pid, "TEST_")

		if err != nil {
			t.Skipf("Cannot access /proc/%d/environ: %v", pid, err)
		}

		require.NoError(t, err)
		assert.NotNil(t, env)
		// Check that only TEST_ prefixed variables are included
		for key := range env {
			assert.True(t, len(key) >= 5 && key[:5] == "TEST_")
		}
	})

	t.Run("non-existent pid", func(t *testing.T) {
		_, _, err := reader.GetProcessEnvironment(999999, "")
		assert.Error(t, err)
	})
}

func TestGetProcessArguments_Integration(t *testing.T) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		t.Skip("Skipping /proc filesystem tests")
	}

	reader := NewProcReader()
	pid := os.Getpid()

	t.Run("success", func(t *testing.T) {
		cmdline, args, err := reader.GetProcessArguments(pid)

		if err != nil {
			t.Skipf("Cannot access /proc/%d/cmdline: %v", pid, err)
		}

		require.NoError(t, err)
		assert.NotEmpty(t, cmdline)
		assert.NotEmpty(t, args)
		assert.Greater(t, len(args), 0)
	})

	t.Run("non-existent pid", func(t *testing.T) {
		_, _, err := reader.GetProcessArguments(999999)
		assert.Error(t, err)
	})
}

func TestFindContainerProcesses_Integration(t *testing.T) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		t.Skip("Skipping /proc filesystem tests")
	}

	reader := NewProcReader()

	t.Run("non-existent container", func(t *testing.T) {
		pids := reader.FindContainerProcesses("nonexistent-container-id-12345")
		// May return nil or empty slice depending on /proc access
		if pids == nil {
			assert.Nil(t, pids)
		} else {
			assert.Empty(t, pids)
		}
	})
}

func TestFindPodContainersByUID_Integration(t *testing.T) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		t.Skip("Skipping /proc filesystem tests")
	}

	reader := NewProcReader()

	t.Run("non-existent pod", func(t *testing.T) {
		containers := reader.FindPodContainersByUID("nonexistent-pod-uid-12345")
		// May return nil or empty slice depending on /proc access
		if containers == nil {
			assert.Nil(t, containers)
		} else {
			assert.Empty(t, containers)
		}
	})
}

// Benchmark tests
func BenchmarkNormalizeContainerID(b *testing.B) {
	id := "containerd://abc123def456ghi789"
	for i := 0; i < b.N; i++ {
		normalizeContainerID(id)
	}
}

func BenchmarkIsAlphanumeric(b *testing.B) {
	s := "abc123def456ghi789jkl012mno345pqr678stu901vwx234"
	for i := 0; i < b.N; i++ {
		isAlphanumeric(s)
	}
}

func BenchmarkExtractContainerIDFromCgroup(b *testing.B) {
	cgroup := "11:freezer:/kubepods/burstable/pod123/abc123def456ghi789jkl012mno345pqr678stu901"
	for i := 0; i < b.N; i++ {
		extractContainerIDFromCgroup(cgroup)
	}
}

// Test helpers for creating test /proc structure
func createTestProcFile(t *testing.T, path, content string) string {
	tmpDir := t.TempDir()
	fullPath := filepath.Join(tmpDir, path)
	
	dir := filepath.Dir(fullPath)
	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)
	
	err = os.WriteFile(fullPath, []byte(content), 0644)
	require.NoError(t, err)
	
	return tmpDir
}

