package pyspy

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
)

func TestNewExecutor(t *testing.T) {
	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       "/tmp/pyspy-test",
		BinaryPath:        "/usr/local/bin/py-spy",
		MaxConcurrentJobs: 5,
		DefaultDuration:   30,
		DefaultRate:       100,
	}

	fs, err := NewFileStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create file store: %v", err)
	}

	executor := NewExecutor(cfg, fs)
	if executor == nil {
		t.Fatal("NewExecutor returned nil")
	}

	if executor.config != cfg {
		t.Error("Config not set correctly")
	}

	if executor.fileStore != fs {
		t.Error("FileStore not set correctly")
	}
}

func TestBuildArgs(t *testing.T) {
	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       "/tmp/pyspy-test",
		BinaryPath:        "/usr/local/bin/py-spy",
		MaxConcurrentJobs: 5,
		DefaultDuration:   30,
		DefaultRate:       100,
	}

	fs, _ := NewFileStore(cfg)
	executor := NewExecutor(cfg, fs)

	tests := []struct {
		name         string
		pid          int
		outputFile   string
		duration     int
		rate         int
		format       OutputFormat
		native       bool
		subprocesses bool
		expected     []string
	}{
		{
			name:         "basic flamegraph",
			pid:          1234,
			outputFile:   "/tmp/output.svg",
			duration:     30,
			rate:         100,
			format:       FormatFlamegraph,
			native:       false,
			subprocesses: false,
			expected:     []string{"record", "--pid", "1234", "--output", "/tmp/output.svg", "--duration", "30", "--rate", "100", "--format", "flamegraph"},
		},
		{
			name:         "speedscope with native",
			pid:          5678,
			outputFile:   "/tmp/output.json",
			duration:     60,
			rate:         200,
			format:       FormatSpeedscope,
			native:       true,
			subprocesses: false,
			expected:     []string{"record", "--pid", "5678", "--output", "/tmp/output.json", "--duration", "60", "--rate", "200", "--format", "speedscope", "--native"},
		},
		{
			name:         "raw with subprocesses",
			pid:          9999,
			outputFile:   "/tmp/output.txt",
			duration:     10,
			rate:         50,
			format:       FormatRaw,
			native:       false,
			subprocesses: true,
			expected:     []string{"record", "--pid", "9999", "--output", "/tmp/output.txt", "--duration", "10", "--rate", "50", "--format", "raw", "--subprocesses"},
		},
		{
			name:         "all options",
			pid:          1111,
			outputFile:   "/tmp/output.svg",
			duration:     120,
			rate:         500,
			format:       FormatFlamegraph,
			native:       true,
			subprocesses: true,
			expected:     []string{"record", "--pid", "1111", "--output", "/tmp/output.svg", "--duration", "120", "--rate", "500", "--format", "flamegraph", "--native", "--subprocesses"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := executor.buildArgs(tc.pid, tc.outputFile, tc.duration, tc.rate, tc.format, tc.native, tc.subprocesses)

			if len(args) != len(tc.expected) {
				t.Errorf("Expected %d args, got %d: %v", len(tc.expected), len(args), args)
				return
			}

			for i, arg := range args {
				if arg != tc.expected[i] {
					t.Errorf("Arg %d: expected %s, got %s", i, tc.expected[i], arg)
				}
			}
		})
	}
}

func TestGetRunningTaskCount(t *testing.T) {
	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       "/tmp/pyspy-test",
		BinaryPath:        "/usr/local/bin/py-spy",
		MaxConcurrentJobs: 5,
		DefaultDuration:   30,
		DefaultRate:       100,
	}

	fs, _ := NewFileStore(cfg)
	executor := NewExecutor(cfg, fs)

	if count := executor.GetRunningTaskCount(); count != 0 {
		t.Errorf("Expected 0 running tasks, got %d", count)
	}
}

func TestIsTaskRunning(t *testing.T) {
	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       "/tmp/pyspy-test",
		BinaryPath:        "/usr/local/bin/py-spy",
		MaxConcurrentJobs: 5,
		DefaultDuration:   30,
		DefaultRate:       100,
	}

	fs, _ := NewFileStore(cfg)
	executor := NewExecutor(cfg, fs)

	if executor.IsTaskRunning("nonexistent") {
		t.Error("Expected false for nonexistent task")
	}
}

func TestCancelTask(t *testing.T) {
	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       "/tmp/pyspy-test",
		BinaryPath:        "/usr/local/bin/py-spy",
		MaxConcurrentJobs: 5,
		DefaultDuration:   30,
		DefaultRate:       100,
	}

	fs, _ := NewFileStore(cfg)
	executor := NewExecutor(cfg, fs)

	if executor.CancelTask("nonexistent") {
		t.Error("Expected false when cancelling nonexistent task")
	}
}

func TestGetOutputFilePath(t *testing.T) {
	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       "/var/lib/lens/pyspy",
		BinaryPath:        "/usr/local/bin/py-spy",
		MaxConcurrentJobs: 5,
		DefaultDuration:   30,
		DefaultRate:       100,
	}

	fs, _ := NewFileStore(cfg)
	executor := NewExecutor(cfg, fs)

	tests := []struct {
		taskID   string
		format   OutputFormat
		expected string
	}{
		{"task-123", FormatFlamegraph, "/var/lib/lens/pyspy/profiles/task-123/profile.svg"},
		{"task-456", FormatSpeedscope, "/var/lib/lens/pyspy/profiles/task-456/profile.json"},
		{"task-789", FormatRaw, "/var/lib/lens/pyspy/profiles/task-789/profile.txt"},
	}

	for _, tc := range tests {
		result := executor.GetOutputFilePath(tc.taskID, tc.format)
		if result != tc.expected {
			t.Errorf("Expected %s, got %s", tc.expected, result)
		}
	}
}

