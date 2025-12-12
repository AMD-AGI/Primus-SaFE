package tuner

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewSystemTuner(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	if tuner == nil {
		t.Fatal("NewSystemTuner returned nil")
	}
	if tuner.config != config {
		t.Error("Config not set correctly")
	}
	if tuner.fs != fs {
		t.Error("FileSystem not set correctly")
	}
	if tuner.cmdExec != cmdExec {
		t.Error("CommandExecutor not set correctly")
	}
}

func TestNewSystemTunerWithNilConfig(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()

	tuner := NewSystemTuner(nil, fs, cmdExec)

	if tuner == nil {
		t.Fatal("NewSystemTuner returned nil")
	}
	if tuner.config == nil {
		t.Error("Should use default config")
	}
	if tuner.config.MaxMapCountThreshold != DefaultMaxMapCountThreshold {
		t.Errorf("Default threshold incorrect, expected %d, got %d",
			DefaultMaxMapCountThreshold, tuner.config.MaxMapCountThreshold)
	}
}

func TestGetCurrentMaxMapCount(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	tests := []struct {
		name        string
		fileContent string
		expected    int
		expectError bool
	}{
		{
			name:        "Normal read",
			fileContent: "262144\n",
			expected:    262144,
			expectError: false,
		},
		{
			name:        "Read small value",
			fileContent: "65530",
			expected:    65530,
			expectError: false,
		},
		{
			name:        "Invalid format",
			fileContent: "invalid",
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs.SetFileContent(config.MaxMapCountPath, []byte(tt.fileContent))

			val, err := tuner.GetCurrentMaxMapCount()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if val != tt.expected {
					t.Errorf("Expected value %d, got %d", tt.expected, val)
				}
			}
		})
	}
}

func TestGetCurrentMaxMapCountReadError(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)
	fs.SetReadError(config.MaxMapCountPath, errors.New("permission denied"))

	_, err := tuner.GetCurrentMaxMapCount()
	if err == nil {
		t.Error("Expected read error but got none")
	}
}

func TestCheckAndSetMaxMapCount_AlreadySatisfied(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// Set current value already satisfies requirement
	fs.SetFileContent(config.MaxMapCountPath, []byte("262144\n"))

	err := tuner.CheckAndSetMaxMapCount()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should not execute any commands
	calls := cmdExec.GetExecuteCalls()
	if len(calls) != 0 {
		t.Errorf("Should not execute any commands, but executed %d", len(calls))
	}
}

func TestCheckAndSetMaxMapCount_NeedsUpdate(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// Set current value below threshold
	fs.SetFileContent(config.MaxMapCountPath, []byte("65530\n"))
	fs.SetFileContent(config.SysctlConfPath, []byte("# Empty config file\n"))

	err := tuner.CheckAndSetMaxMapCount()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should execute nsenter sysctl -p
	calls := cmdExec.GetExecuteCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 command execution, got %d", len(calls))
	}

	expectedCmd := []string{"nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sysctl", "-p"}
	if !equalStringSlice(calls[0], expectedCmd) {
		t.Errorf("Expected command %v, got %v", expectedCmd, calls[0])
	}

	// Check if sysctl.conf was correctly updated
	content := string(fs.GetFileContent(config.SysctlConfPath))
	if !strings.Contains(content, "vm.max_map_count=262144") {
		t.Errorf("sysctl.conf content doesn't contain expected config: %s", content)
	}
}

func TestCheckAndSetMaxMapCount_ReadError(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)
	fs.SetReadError(config.MaxMapCountPath, errors.New("permission denied"))

	err := tuner.CheckAndSetMaxMapCount()
	if err == nil {
		t.Error("Expected error but got none")
	}
}

func TestCheckAndSetMaxMapCount_CommandError(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	fs.SetFileContent(config.MaxMapCountPath, []byte("65530\n"))
	fs.SetFileContent(config.SysctlConfPath, []byte("# Empty config file\n"))
	cmdExec.SetExecuteError(errors.New("command failed"))

	err := tuner.CheckAndSetMaxMapCount()
	if err == nil {
		t.Error("Expected command execution error but got none")
	}
}

func TestEnsureSysctlFileValue_AddNew(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// Set initial content (does not contain vm.max_map_count)
	initialContent := "# Sysctl configuration\nnet.ipv4.ip_forward=1\n"
	fs.SetFileContent(config.SysctlConfPath, []byte(initialContent))

	err := tuner.ensureSysctlFileValue()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	content := string(fs.GetFileContent(config.SysctlConfPath))
	if !strings.Contains(content, "vm.max_map_count=262144") {
		t.Error("Should add vm.max_map_count config")
	}
	if !strings.Contains(content, "net.ipv4.ip_forward=1") {
		t.Error("Should not modify existing config")
	}
}

func TestEnsureSysctlFileValue_UpdateExisting(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// Set initial content (contains low value of vm.max_map_count)
	initialContent := "# Sysctl configuration\nvm.max_map_count=65530\nnet.ipv4.ip_forward=1\n"
	fs.SetFileContent(config.SysctlConfPath, []byte(initialContent))

	err := tuner.ensureSysctlFileValue()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	content := string(fs.GetFileContent(config.SysctlConfPath))
	if !strings.Contains(content, "vm.max_map_count=262144") {
		t.Error("Should update vm.max_map_count config")
	}
	if strings.Contains(content, "vm.max_map_count=65530") {
		t.Error("Should not keep old value")
	}
}

func TestEnsureSysctlFileValue_AlreadyHighEnough(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// Set initial content (contains high value of vm.max_map_count)
	initialContent := "vm.max_map_count=500000\n"
	fs.SetFileContent(config.SysctlConfPath, []byte(initialContent))

	err := tuner.ensureSysctlFileValue()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	content := string(fs.GetFileContent(config.SysctlConfPath))
	if !strings.Contains(content, "vm.max_map_count=500000") {
		t.Error("Should not modify value that is already high enough")
	}
}

func TestCheckAndSetMaxOpenFiles(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// Set initial content (does not contain nofile config)
	initialContent := "# Limits configuration\n"
	fs.SetFileContent(config.LimitsConfPath, []byte(initialContent))

	err := tuner.CheckAndSetMaxOpenFiles()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	content := string(fs.GetFileContent(config.LimitsConfPath))
	if !strings.Contains(content, "* soft nofile 131072") {
		t.Error("Should add soft nofile config")
	}
	if !strings.Contains(content, "* hard nofile 131072") {
		t.Error("Should add hard nofile config")
	}
}

func TestEnsureLimitLine_AddNew(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	initialContent := "# Empty limits file\n"
	fs.SetFileContent(config.LimitsConfPath, []byte(initialContent))

	err := tuner.ensureLimitLine("soft")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	content := string(fs.GetFileContent(config.LimitsConfPath))
	expectedLine := fmt.Sprintf("* soft nofile %d", config.MaxOpenFilesThreshold)
	if !strings.Contains(content, expectedLine) {
		t.Errorf("Should contain %s, actual content: %s", expectedLine, content)
	}
}

func TestEnsureLimitLine_UpdateExisting(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// Set initial content (contains low value)
	initialContent := "* soft nofile 1024\n* hard nofile 4096\n"
	fs.SetFileContent(config.LimitsConfPath, []byte(initialContent))

	err := tuner.ensureLimitLine("soft")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	content := string(fs.GetFileContent(config.LimitsConfPath))
	expectedLine := fmt.Sprintf("* soft nofile %d", config.MaxOpenFilesThreshold)
	if !strings.Contains(content, expectedLine) {
		t.Errorf("Should contain %s", expectedLine)
	}
	if strings.Contains(content, "* soft nofile 1024") {
		t.Error("Should not keep old value")
	}
}

func TestEnsureLimitLine_AlreadyHighEnough(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// Set initial content (already high enough)
	initialContent := "* soft nofile 200000\n"
	fs.SetFileContent(config.LimitsConfPath, []byte(initialContent))

	err := tuner.ensureLimitLine("soft")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	content := string(fs.GetFileContent(config.LimitsConfPath))
	if !strings.Contains(content, "* soft nofile 200000") {
		t.Error("Should not modify value that is already high enough")
	}
}

func TestEnsureLimitLine_ReadError(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)
	fs.SetReadError(config.LimitsConfPath, errors.New("permission denied"))

	err := tuner.ensureLimitLine("soft")
	if err == nil {
		t.Error("Expected read error but got none")
	}
}

func TestEnsureLimitLine_WriteError(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	fs.SetFileContent(config.LimitsConfPath, []byte("# Empty\n"))
	fs.SetWriteError(config.LimitsConfPath, errors.New("disk full"))

	err := tuner.ensureLimitLine("soft")
	if err == nil {
		t.Error("Expected write error but got none")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxMapCountThreshold != DefaultMaxMapCountThreshold {
		t.Errorf("MaxMapCountThreshold incorrect, expected %d, got %d",
			DefaultMaxMapCountThreshold, config.MaxMapCountThreshold)
	}
	if config.MaxOpenFilesThreshold != DefaultMaxOpenFilesThreshold {
		t.Errorf("MaxOpenFilesThreshold incorrect, expected %d, got %d",
			DefaultMaxOpenFilesThreshold, config.MaxOpenFilesThreshold)
	}
	if config.MaxMapCountPath != "/host-proc/sys/vm/max_map_count" {
		t.Errorf("MaxMapCountPath incorrect: %s", config.MaxMapCountPath)
	}
	if config.SysctlConfPath != "/etc/sysctl.conf" {
		t.Errorf("SysctlConfPath incorrect: %s", config.SysctlConfPath)
	}
	if config.LimitsConfPath != "/etc/security/limits.conf" {
		t.Errorf("LimitsConfPath incorrect: %s", config.LimitsConfPath)
	}
}

// Helper function: compare if string slices are equal
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
