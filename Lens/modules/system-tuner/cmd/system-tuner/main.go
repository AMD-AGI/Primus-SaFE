package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	MaxMapCountThreshold  = 262144
	MaxOpenFilesThreshold = 131072

	CheckInterval = 30 * time.Second
)

const (
	limitsFile = "/etc/security/limits.conf"
)

func checkAndSetMaxMapCount() {

	data, err := os.ReadFile("/host-proc/sys/vm/max_map_count")
	if err != nil {
		fmt.Println(fmt.Sprintf("Error reading max_map_count: %v", err))
		return
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Println(fmt.Sprintf("Error parsing max_map_count: %v", err))
		return
	}
	fmt.Println(fmt.Sprintf(fmt.Sprintf("Current vm.max_map_count: %d", val)))
	if val >= MaxMapCountThreshold {
		fmt.Println(fmt.Sprintf("vm.max_map_count is already >= %d, no change needed", MaxMapCountThreshold))
		return
	}

	fmt.Println(fmt.Sprintf("Current vm.max_map_count: %d", val))
	ensureSysctlFileValue()
	fmt.Println(fmt.Sprintf("Executing sysctl -p to apply changes"))
	//nsenter --target 1 --mount --uts --ipc --net --pid --
	cmd := exec.Command("nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sysctl", "-p")
	if err := cmd.Run(); err != nil {
		fmt.Println(fmt.Sprintf("Failed to apply sysctl: %v", err))
		return
	}

	fmt.Println(fmt.Sprintf("vm.max_map_count set to %d", MaxMapCountThreshold))
}

func ensureSysctlFileValue() {
	const sysctlFile = "/etc/sysctl.conf"
	targetLine := fmt.Sprintf("vm.max_map_count=%d\n", MaxMapCountThreshold)

	content, err := os.ReadFile(sysctlFile)
	if err != nil {
		fmt.Println(fmt.Sprintf("Error reading %s: %v", sysctlFile, err))
		return
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "vm.max_map_count") {
			currentValStr := strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
			currentVal, err := strconv.Atoi(currentValStr)
			if err != nil {
				fmt.Println(fmt.Sprintf("Error parsing existing max_map_count in %s: %v", sysctlFile, err))
				lines[i] = targetLine
				found = true
				break
			}
			if currentVal >= MaxMapCountThreshold {
				fmt.Println(fmt.Sprintf("max_map_count in %s is already >= %d, no change needed", sysctlFile, MaxMapCountThreshold))
				return
			}
			lines[i] = targetLine
			found = true
			break
		}
	}

	if !found {
		fmt.Println(fmt.Sprintf("Error: max_map_count not found in %s", sysctlFile))
		lines = append(lines, targetLine)
	}

	err = os.WriteFile(sysctlFile, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		fmt.Println(fmt.Sprintf("Failed to write %s: %v", sysctlFile, err))
		return
	}
}

func checkAndSetMaxOpenFiles() {
	if err := ensureLimitLine(limitsFile, "soft", MaxOpenFilesThreshold); err != nil {
		fmt.Println(fmt.Sprintf("Error: %v", err))
	}
	if err := ensureLimitLine(limitsFile, "hard", MaxOpenFilesThreshold); err != nil {
		fmt.Println(fmt.Sprintf("Error: %v", err))
	}
}

func ensureLimitLine(file string, lineType string, threshold int) error {
	f, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	lines := strings.Split(string(f), "\n")
	updated := false

	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 4 && fields[0] == "*" && fields[1] == lineType && fields[2] == "nofile" {
			val, err := strconv.Atoi(fields[3])
			if err != nil {
				continue
			}
			if val < threshold {
				lines[i] = fmt.Sprintf("* %s nofile %d", lineType, threshold)
				fmt.Printf("Updated %s in %s to %d\n", lineType, file, threshold)
			} else {
				fmt.Printf("%s in %s already >= %d, no change\n", lineType, file, threshold)
			}
			updated = true
			break
		}
	}

	if !updated {
		newLine := fmt.Sprintf("* %s nofile %d", lineType, threshold)
		lines = append(lines, newLine)
		fmt.Printf("Added line to %s: %s\n", file, newLine)
	}

	return os.WriteFile(file, []byte(strings.Join(lines, "\n")), 0644)
}

func main() {
	fmt.Println(fmt.Sprintf("System-Tuner v0.1"))
	for {
		checkAndSetMaxMapCount()
		checkAndSetMaxOpenFiles()
		time.Sleep(CheckInterval)
	}
}
