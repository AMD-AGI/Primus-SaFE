package tuner

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// DefaultMaxMapCountThreshold default threshold for vm.max_map_count
	DefaultMaxMapCountThreshold = 262144
	// DefaultMaxOpenFilesThreshold default threshold for max open files
	DefaultMaxOpenFilesThreshold = 131072
)

// Config system tuning configuration
type Config struct {
	MaxMapCountThreshold  int
	MaxOpenFilesThreshold int
	MaxMapCountPath       string // /host-proc/sys/vm/max_map_count
	SysctlConfPath        string // /etc/sysctl.conf
	LimitsConfPath        string // /etc/security/limits.conf
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		MaxMapCountThreshold:  DefaultMaxMapCountThreshold,
		MaxOpenFilesThreshold: DefaultMaxOpenFilesThreshold,
		MaxMapCountPath:       "/host-proc/sys/vm/max_map_count",
		SysctlConfPath:        "/etc/sysctl.conf",
		LimitsConfPath:        "/etc/security/limits.conf",
	}
}

// SystemTuner system tuner
type SystemTuner struct {
	config  *Config
	fs      FileSystem
	cmdExec CommandExecutor
}

// NewSystemTuner creates a system tuner instance
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

// CheckAndSetMaxMapCount checks and sets vm.max_map_count
func (st *SystemTuner) CheckAndSetMaxMapCount() error {
	// Read current value
	data, err := st.fs.ReadFile(st.config.MaxMapCountPath)
	if err != nil {
		return fmt.Errorf("failed to read max_map_count: %w", err)
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("failed to parse max_map_count: %w", err)
	}

	fmt.Printf("Current vm.max_map_count: %d\n", val)

	// If already satisfies requirement, no need to modify
	if val >= st.config.MaxMapCountThreshold {
		fmt.Printf("vm.max_map_count is already >= %d, no modification needed\n", st.config.MaxMapCountThreshold)
		return nil
	}

	// Update sysctl.conf file
	if err := st.ensureSysctlFileValue(); err != nil {
		return fmt.Errorf("failed to update sysctl.conf: %w", err)
	}

	// Execute sysctl -p to apply changes
	fmt.Println("Executing sysctl -p to apply changes")
	err = st.cmdExec.Execute("nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sysctl", "-p")
	if err != nil {
		return fmt.Errorf("failed to execute sysctl -p: %w", err)
	}

	fmt.Printf("vm.max_map_count has been set to %d\n", st.config.MaxMapCountThreshold)
	return nil
}

// ensureSysctlFileValue ensures sysctl.conf file contains correct vm.max_map_count value
func (st *SystemTuner) ensureSysctlFileValue() error {
	targetLine := fmt.Sprintf("vm.max_map_count=%d", st.config.MaxMapCountThreshold)

	content, err := st.fs.ReadFile(st.config.SysctlConfPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", st.config.SysctlConfPath, err)
	}

	lines := strings.Split(string(content), "\n")
	found := false

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "vm.max_map_count") {
			// Parse existing value
			parts := strings.SplitN(trimmedLine, "=", 2)
			if len(parts) == 2 {
				currentValStr := strings.TrimSpace(parts[1])
				currentVal, err := strconv.Atoi(currentValStr)
				if err == nil && currentVal >= st.config.MaxMapCountThreshold {
					fmt.Printf("max_map_count in %s is already >= %d, no modification needed\n",
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
		fmt.Printf("max_map_count not found in %s, will add this config\n", st.config.SysctlConfPath)
		lines = append(lines, targetLine)
	}

	newContent := strings.Join(lines, "\n")
	return st.fs.WriteFile(st.config.SysctlConfPath, []byte(newContent), 0644)
}

// CheckAndSetMaxOpenFiles checks and sets max open files limit
func (st *SystemTuner) CheckAndSetMaxOpenFiles() error {
	if err := st.ensureLimitLine("soft"); err != nil {
		return fmt.Errorf("failed to set soft limit: %w", err)
	}
	if err := st.ensureLimitLine("hard"); err != nil {
		return fmt.Errorf("failed to set hard limit: %w", err)
	}
	return nil
}

// ensureLimitLine ensures the specified limit line exists in limits.conf
func (st *SystemTuner) ensureLimitLine(limitType string) error {
	content, err := st.fs.ReadFile(st.config.LimitsConfPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", st.config.LimitsConfPath, err)
	}

	lines := strings.Split(string(content), "\n")
	updated := false

	for i, line := range lines {
		fields := strings.Fields(line)
		// Look for lines like: * soft nofile 131072 or * hard nofile 131072
		if len(fields) == 4 && fields[0] == "*" && fields[1] == limitType && fields[2] == "nofile" {
			val, err := strconv.Atoi(fields[3])
			if err != nil {
				continue
			}
			if val < st.config.MaxOpenFilesThreshold {
				lines[i] = fmt.Sprintf("* %s nofile %d", limitType, st.config.MaxOpenFilesThreshold)
				fmt.Printf("Updated %s to %d in %s\n", limitType, st.config.MaxOpenFilesThreshold, st.config.LimitsConfPath)
			} else {
				fmt.Printf("%s in %s is already >= %d, no modification needed\n", limitType, st.config.LimitsConfPath, st.config.MaxOpenFilesThreshold)
			}
			updated = true
			break
		}
	}

	if !updated {
		newLine := fmt.Sprintf("* %s nofile %d", limitType, st.config.MaxOpenFilesThreshold)
		lines = append(lines, newLine)
		fmt.Printf("Added new line in %s: %s\n", st.config.LimitsConfPath, newLine)
	}

	newContent := strings.Join(lines, "\n")
	return st.fs.WriteFile(st.config.LimitsConfPath, []byte(newContent), 0644)
}

// GetCurrentMaxMapCount gets the current vm.max_map_count value (for testing)
func (st *SystemTuner) GetCurrentMaxMapCount() (int, error) {
	data, err := st.fs.ReadFile(st.config.MaxMapCountPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read max_map_count: %w", err)
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("failed to parse max_map_count: %w", err)
	}

	return val, nil
}

