package processtree

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// NsenterExecutor executes commands in container namespaces
type NsenterExecutor struct{}

// NewNsenterExecutor creates a new nsenter executor
func NewNsenterExecutor() *NsenterExecutor {
	return &NsenterExecutor{}
}

// GetContainerPID gets the PID from container's perspective
func (e *NsenterExecutor) GetContainerPID(hostPID int) (int, error) {
	// Method 1: Read from /proc/[pid]/status (NSpid field)
	statusPath := fmt.Sprintf("/proc/%d/status", hostPID)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		// NSpid: <host_pid> <container_pid>
		if strings.HasPrefix(line, "NSpid:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				// Last PID is the innermost namespace (container)
				if containerPID, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
					return containerPID, nil
				}
			}
		}
	}

	return 0, fmt.Errorf("container PID not found for host PID %d", hostPID)
}

// ExecuteInContainer executes a command in container namespace
func (e *NsenterExecutor) ExecuteInContainer(hostPID int, command string) (string, error) {
	// Use nsenter to enter container's namespace
	cmd := exec.Command("nsenter",
		"-t", strconv.Itoa(hostPID),
		"-p", // PID namespace
		"-m", // Mount namespace
		"--",
		"sh", "-c", command,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("nsenter failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// GetContainerProcessList gets process list from inside the container
func (e *NsenterExecutor) GetContainerProcessList(hostPID int) ([]string, error) {
	// Execute 'ps aux' inside container namespace
	output, err := e.ExecuteInContainer(hostPID, "ps -e -o pid,ppid,comm,args --no-headers")
	if err != nil {
		log.Debugf("Failed to execute ps in container: %v", err)
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	return lines, nil
}

// CheckNsenterAvailable checks if nsenter command is available
func (e *NsenterExecutor) CheckNsenterAvailable() bool {
	_, err := exec.LookPath("nsenter")
	return err == nil
}

// GetContainerEnvironment gets environment variables from inside container
func (e *NsenterExecutor) GetContainerEnvironment(hostPID int) ([]string, error) {
	output, err := e.ExecuteInContainer(hostPID, "env")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	return lines, nil
}
