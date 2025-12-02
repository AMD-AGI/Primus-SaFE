package tuner

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// DefaultMaxMapCountThreshold vm.max_map_count 的默认阈值
	DefaultMaxMapCountThreshold = 262144
	// DefaultMaxOpenFilesThreshold 最大打开文件数的默认阈值
	DefaultMaxOpenFilesThreshold = 131072
)

// Config 系统调优配置
type Config struct {
	MaxMapCountThreshold  int
	MaxOpenFilesThreshold int
	MaxMapCountPath       string // /host-proc/sys/vm/max_map_count
	SysctlConfPath        string // /etc/sysctl.conf
	LimitsConfPath        string // /etc/security/limits.conf
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxMapCountThreshold:  DefaultMaxMapCountThreshold,
		MaxOpenFilesThreshold: DefaultMaxOpenFilesThreshold,
		MaxMapCountPath:       "/host-proc/sys/vm/max_map_count",
		SysctlConfPath:        "/etc/sysctl.conf",
		LimitsConfPath:        "/etc/security/limits.conf",
	}
}

// SystemTuner 系统调优器
type SystemTuner struct {
	config  *Config
	fs      FileSystem
	cmdExec CommandExecutor
}

// NewSystemTuner 创建系统调优器实例
func NewSystemTuner(config *Config, fs FileSystem, cmdExec CommandExecutor) *SystemTuner {
	if config == nil {
		config = DefaultConfig()
	}
	return &SystemTuner{
		config:  config,
		fs:      fs,
		cmdExec: cmdExec,
	}
}

// CheckAndSetMaxMapCount 检查并设置 vm.max_map_count
func (st *SystemTuner) CheckAndSetMaxMapCount() error {
	// 读取当前值
	data, err := st.fs.ReadFile(st.config.MaxMapCountPath)
	if err != nil {
		return fmt.Errorf("读取 max_map_count 失败: %w", err)
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("解析 max_map_count 失败: %w", err)
	}

	fmt.Printf("当前 vm.max_map_count: %d\n", val)

	// 如果已经满足要求，不需要修改
	if val >= st.config.MaxMapCountThreshold {
		fmt.Printf("vm.max_map_count 已经 >= %d，无需修改\n", st.config.MaxMapCountThreshold)
		return nil
	}

	// 更新 sysctl.conf 文件
	if err := st.ensureSysctlFileValue(); err != nil {
		return fmt.Errorf("更新 sysctl.conf 失败: %w", err)
	}

	// 执行 sysctl -p 应用更改
	fmt.Println("执行 sysctl -p 应用更改")
	err = st.cmdExec.Execute("nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sysctl", "-p")
	if err != nil {
		return fmt.Errorf("执行 sysctl -p 失败: %w", err)
	}

	fmt.Printf("vm.max_map_count 已设置为 %d\n", st.config.MaxMapCountThreshold)
	return nil
}

// ensureSysctlFileValue 确保 sysctl.conf 文件中包含正确的 vm.max_map_count 值
func (st *SystemTuner) ensureSysctlFileValue() error {
	targetLine := fmt.Sprintf("vm.max_map_count=%d", st.config.MaxMapCountThreshold)

	content, err := st.fs.ReadFile(st.config.SysctlConfPath)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", st.config.SysctlConfPath, err)
	}

	lines := strings.Split(string(content), "\n")
	found := false

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "vm.max_map_count") {
			// 解析现有值
			parts := strings.SplitN(trimmedLine, "=", 2)
			if len(parts) == 2 {
				currentValStr := strings.TrimSpace(parts[1])
				currentVal, err := strconv.Atoi(currentValStr)
				if err == nil && currentVal >= st.config.MaxMapCountThreshold {
					fmt.Printf("%s 中的 max_map_count 已经 >= %d，无需修改\n",
						st.config.SysctlConfPath, st.config.MaxMapCountThreshold)
					return nil
				}
			}
			lines[i] = targetLine
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("在 %s 中未找到 max_map_count，将添加该配置\n", st.config.SysctlConfPath)
		lines = append(lines, targetLine)
	}

	newContent := strings.Join(lines, "\n")
	return st.fs.WriteFile(st.config.SysctlConfPath, []byte(newContent), 0644)
}

// CheckAndSetMaxOpenFiles 检查并设置最大打开文件数限制
func (st *SystemTuner) CheckAndSetMaxOpenFiles() error {
	if err := st.ensureLimitLine("soft"); err != nil {
		return fmt.Errorf("设置 soft limit 失败: %w", err)
	}
	if err := st.ensureLimitLine("hard"); err != nil {
		return fmt.Errorf("设置 hard limit 失败: %w", err)
	}
	return nil
}

// ensureLimitLine 确保 limits.conf 中存在指定的限制行
func (st *SystemTuner) ensureLimitLine(limitType string) error {
	content, err := st.fs.ReadFile(st.config.LimitsConfPath)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", st.config.LimitsConfPath, err)
	}

	lines := strings.Split(string(content), "\n")
	updated := false

	for i, line := range lines {
		fields := strings.Fields(line)
		// 查找形如: * soft nofile 131072 或 * hard nofile 131072 的行
		if len(fields) == 4 && fields[0] == "*" && fields[1] == limitType && fields[2] == "nofile" {
			val, err := strconv.Atoi(fields[3])
			if err != nil {
				continue
			}
			if val < st.config.MaxOpenFilesThreshold {
				lines[i] = fmt.Sprintf("* %s nofile %d", limitType, st.config.MaxOpenFilesThreshold)
				fmt.Printf("在 %s 中更新 %s 为 %d\n", st.config.LimitsConfPath, limitType, st.config.MaxOpenFilesThreshold)
			} else {
				fmt.Printf("%s 中的 %s 已经 >= %d，无需修改\n", st.config.LimitsConfPath, limitType, st.config.MaxOpenFilesThreshold)
			}
			updated = true
			break
		}
	}

	if !updated {
		newLine := fmt.Sprintf("* %s nofile %d", limitType, st.config.MaxOpenFilesThreshold)
		lines = append(lines, newLine)
		fmt.Printf("在 %s 中添加新行: %s\n", st.config.LimitsConfPath, newLine)
	}

	newContent := strings.Join(lines, "\n")
	return st.fs.WriteFile(st.config.LimitsConfPath, []byte(newContent), 0644)
}

// GetCurrentMaxMapCount 获取当前的 vm.max_map_count 值（用于测试）
func (st *SystemTuner) GetCurrentMaxMapCount() (int, error) {
	data, err := st.fs.ReadFile(st.config.MaxMapCountPath)
	if err != nil {
		return 0, fmt.Errorf("读取 max_map_count 失败: %w", err)
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("解析 max_map_count 失败: %w", err)
	}

	return val, nil
}

