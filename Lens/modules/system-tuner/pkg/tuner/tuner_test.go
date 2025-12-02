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
		t.Fatal("NewSystemTuner 返回了 nil")
	}
	if tuner.config != config {
		t.Error("配置未正确设置")
	}
	if tuner.fs != fs {
		t.Error("文件系统未正确设置")
	}
	if tuner.cmdExec != cmdExec {
		t.Error("命令执行器未正确设置")
	}
}

func TestNewSystemTunerWithNilConfig(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()

	tuner := NewSystemTuner(nil, fs, cmdExec)

	if tuner == nil {
		t.Fatal("NewSystemTuner 返回了 nil")
	}
	if tuner.config == nil {
		t.Error("应该使用默认配置")
	}
	if tuner.config.MaxMapCountThreshold != DefaultMaxMapCountThreshold {
		t.Errorf("默认阈值不正确，期望 %d，实际 %d",
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
			name:        "正常读取",
			fileContent: "262144\n",
			expected:    262144,
			expectError: false,
		},
		{
			name:        "读取小值",
			fileContent: "65530",
			expected:    65530,
			expectError: false,
		},
		{
			name:        "无效格式",
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
					t.Error("期望出错但没有出错")
				}
			} else {
				if err != nil {
					t.Errorf("不期望出错但出错了: %v", err)
				}
				if val != tt.expected {
					t.Errorf("期望值 %d，实际值 %d", tt.expected, val)
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
		t.Error("期望读取错误但没有出错")
	}
}

func TestCheckAndSetMaxMapCount_AlreadySatisfied(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// 设置当前值已经满足要求
	fs.SetFileContent(config.MaxMapCountPath, []byte("262144\n"))

	err := tuner.CheckAndSetMaxMapCount()
	if err != nil {
		t.Errorf("不期望出错: %v", err)
	}

	// 不应该执行任何命令
	calls := cmdExec.GetExecuteCalls()
	if len(calls) != 0 {
		t.Errorf("不应该执行任何命令，但执行了 %d 个", len(calls))
	}
}

func TestCheckAndSetMaxMapCount_NeedsUpdate(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// 设置当前值低于阈值
	fs.SetFileContent(config.MaxMapCountPath, []byte("65530\n"))
	fs.SetFileContent(config.SysctlConfPath, []byte("# 空配置文件\n"))

	err := tuner.CheckAndSetMaxMapCount()
	if err != nil {
		t.Errorf("不期望出错: %v", err)
	}

	// 应该执行 nsenter sysctl -p
	calls := cmdExec.GetExecuteCalls()
	if len(calls) != 1 {
		t.Fatalf("期望执行 1 个命令，实际执行了 %d 个", len(calls))
	}

	expectedCmd := []string{"nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sysctl", "-p"}
	if !equalStringSlice(calls[0], expectedCmd) {
		t.Errorf("期望命令 %v，实际命令 %v", expectedCmd, calls[0])
	}

	// 检查 sysctl.conf 是否被正确更新
	content := string(fs.GetFileContent(config.SysctlConfPath))
	if !strings.Contains(content, "vm.max_map_count=262144") {
		t.Errorf("sysctl.conf 内容不包含预期配置: %s", content)
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
		t.Error("期望出错但没有出错")
	}
}

func TestCheckAndSetMaxMapCount_CommandError(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	fs.SetFileContent(config.MaxMapCountPath, []byte("65530\n"))
	fs.SetFileContent(config.SysctlConfPath, []byte("# 空配置文件\n"))
	cmdExec.SetExecuteError(errors.New("command failed"))

	err := tuner.CheckAndSetMaxMapCount()
	if err == nil {
		t.Error("期望命令执行错误但没有出错")
	}
}

func TestEnsureSysctlFileValue_AddNew(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// 设置初始内容（不包含 vm.max_map_count）
	initialContent := "# Sysctl configuration\nnet.ipv4.ip_forward=1\n"
	fs.SetFileContent(config.SysctlConfPath, []byte(initialContent))

	err := tuner.ensureSysctlFileValue()
	if err != nil {
		t.Errorf("不期望出错: %v", err)
	}

	content := string(fs.GetFileContent(config.SysctlConfPath))
	if !strings.Contains(content, "vm.max_map_count=262144") {
		t.Error("应该添加 vm.max_map_count 配置")
	}
	if !strings.Contains(content, "net.ipv4.ip_forward=1") {
		t.Error("不应该修改原有配置")
	}
}

func TestEnsureSysctlFileValue_UpdateExisting(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// 设置初始内容（包含低值的 vm.max_map_count）
	initialContent := "# Sysctl configuration\nvm.max_map_count=65530\nnet.ipv4.ip_forward=1\n"
	fs.SetFileContent(config.SysctlConfPath, []byte(initialContent))

	err := tuner.ensureSysctlFileValue()
	if err != nil {
		t.Errorf("不期望出错: %v", err)
	}

	content := string(fs.GetFileContent(config.SysctlConfPath))
	if !strings.Contains(content, "vm.max_map_count=262144") {
		t.Error("应该更新 vm.max_map_count 配置")
	}
	if strings.Contains(content, "vm.max_map_count=65530") {
		t.Error("不应该保留旧值")
	}
}

func TestEnsureSysctlFileValue_AlreadyHighEnough(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// 设置初始内容（包含高值的 vm.max_map_count）
	initialContent := "vm.max_map_count=500000\n"
	fs.SetFileContent(config.SysctlConfPath, []byte(initialContent))

	err := tuner.ensureSysctlFileValue()
	if err != nil {
		t.Errorf("不期望出错: %v", err)
	}

	content := string(fs.GetFileContent(config.SysctlConfPath))
	if !strings.Contains(content, "vm.max_map_count=500000") {
		t.Error("不应该修改已经足够高的值")
	}
}

func TestCheckAndSetMaxOpenFiles(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// 设置初始内容（不包含 nofile 配置）
	initialContent := "# Limits configuration\n"
	fs.SetFileContent(config.LimitsConfPath, []byte(initialContent))

	err := tuner.CheckAndSetMaxOpenFiles()
	if err != nil {
		t.Errorf("不期望出错: %v", err)
	}

	content := string(fs.GetFileContent(config.LimitsConfPath))
	if !strings.Contains(content, "* soft nofile 131072") {
		t.Error("应该添加 soft nofile 配置")
	}
	if !strings.Contains(content, "* hard nofile 131072") {
		t.Error("应该添加 hard nofile 配置")
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
		t.Errorf("不期望出错: %v", err)
	}

	content := string(fs.GetFileContent(config.LimitsConfPath))
	expectedLine := fmt.Sprintf("* soft nofile %d", config.MaxOpenFilesThreshold)
	if !strings.Contains(content, expectedLine) {
		t.Errorf("应该包含 %s，实际内容: %s", expectedLine, content)
	}
}

func TestEnsureLimitLine_UpdateExisting(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// 设置初始内容（包含低值）
	initialContent := "* soft nofile 1024\n* hard nofile 4096\n"
	fs.SetFileContent(config.LimitsConfPath, []byte(initialContent))

	err := tuner.ensureLimitLine("soft")
	if err != nil {
		t.Errorf("不期望出错: %v", err)
	}

	content := string(fs.GetFileContent(config.LimitsConfPath))
	expectedLine := fmt.Sprintf("* soft nofile %d", config.MaxOpenFilesThreshold)
	if !strings.Contains(content, expectedLine) {
		t.Errorf("应该包含 %s", expectedLine)
	}
	if strings.Contains(content, "* soft nofile 1024") {
		t.Error("不应该保留旧值")
	}
}

func TestEnsureLimitLine_AlreadyHighEnough(t *testing.T) {
	fs := NewMockFileSystem()
	cmdExec := NewMockCommandExecutor()
	config := DefaultConfig()

	tuner := NewSystemTuner(config, fs, cmdExec)

	// 设置初始内容（已经足够高）
	initialContent := "* soft nofile 200000\n"
	fs.SetFileContent(config.LimitsConfPath, []byte(initialContent))

	err := tuner.ensureLimitLine("soft")
	if err != nil {
		t.Errorf("不期望出错: %v", err)
	}

	content := string(fs.GetFileContent(config.LimitsConfPath))
	if !strings.Contains(content, "* soft nofile 200000") {
		t.Error("不应该修改已经足够高的值")
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
		t.Error("期望读取错误但没有出错")
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
		t.Error("期望写入错误但没有出错")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxMapCountThreshold != DefaultMaxMapCountThreshold {
		t.Errorf("MaxMapCountThreshold 错误，期望 %d，实际 %d",
			DefaultMaxMapCountThreshold, config.MaxMapCountThreshold)
	}
	if config.MaxOpenFilesThreshold != DefaultMaxOpenFilesThreshold {
		t.Errorf("MaxOpenFilesThreshold 错误，期望 %d，实际 %d",
			DefaultMaxOpenFilesThreshold, config.MaxOpenFilesThreshold)
	}
	if config.MaxMapCountPath != "/host-proc/sys/vm/max_map_count" {
		t.Errorf("MaxMapCountPath 错误: %s", config.MaxMapCountPath)
	}
	if config.SysctlConfPath != "/etc/sysctl.conf" {
		t.Errorf("SysctlConfPath 错误: %s", config.SysctlConfPath)
	}
	if config.LimitsConfPath != "/etc/security/limits.conf" {
		t.Errorf("LimitsConfPath 错误: %s", config.LimitsConfPath)
	}
}

// 辅助函数：比较字符串切片是否相等
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

