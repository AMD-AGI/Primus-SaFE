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
		fmt.Printf("Error reading max_map_count: %v\n", err)
		return
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Printf("Error parsing max_map_count: %v\n", err)
		return
	}
	fmt.Printf("Current vm.max_map_count: %d\n", val)
	if val >= MaxMapCountThreshold {
		fmt.Printf("vm.max_map_count is already >= %d, no change needed\n", MaxMapCountThreshold)
		return
	}

	fmt.Printf("Current vm.max_map_count: %d\n", val)
	ensureSysctlFileValue()
	fmt.Println("Executing sysctl -p to apply changes")
	//nsenter --target 1 --mount --uts --ipc --net --pid --
	cmd := exec.Command("nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sysctl", "-p")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to apply sysctl: %v\n", err)
		return
	}

	fmt.Printf("vm.max_map_count set to %d\n", MaxMapCountThreshold)
}

func ensureSysctlFileValue() {
	const sysctlFile = "/etc/sysctl.conf"
	targetLine := fmt.Sprintf("vm.max_map_count=%d\n", MaxMapCountThreshold)

	content, err := os.ReadFile(sysctlFile)
	if err != nil {
		fmt.Printf("Error reading %s: %v\n", sysctlFile, err)
		return
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "vm.max_map_count") {
			currentValStr := strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
			currentVal, err := strconv.Atoi(currentValStr)
			if err != nil {
				fmt.Printf("Error parsing existing max_map_count in %s: %v\n", sysctlFile, err)
				lines[i] = targetLine
				found = true
				break
			}
			if currentVal >= MaxMapCountThreshold {
				fmt.Printf("max_map_count in %s is already >= %d, no change needed\n", sysctlFile, MaxMapCountThreshold)
				return
			}
			lines[i] = targetLine
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("Error: max_map_count not found in %s\n", sysctlFile)
		lines = append(lines, targetLine)
	}

	err = os.WriteFile(sysctlFile, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		fmt.Printf("Failed to write %s: %v\n", sysctlFile, err)
		return
	}
}

func checkAndSetMaxOpenFiles() {
	if err := ensureLimitLine(limitsFile, "soft", MaxOpenFilesThreshold); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	if err := ensureLimitLine(limitsFile, "hard", MaxOpenFilesThreshold); err != nil {
		fmt.Printf("Error: %v\n", err)
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
	fmt.Println("System-Tuner v0.1")
	for {
		checkAndSetMaxMapCount()
		checkAndSetMaxOpenFiles()
		time.Sleep(CheckInterval)
	}
}
