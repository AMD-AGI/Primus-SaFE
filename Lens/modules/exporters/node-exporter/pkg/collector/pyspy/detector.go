package pyspy

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	processtree "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/process-tree"
)

// Detector checks py-spy compatibility for containers
type Detector struct {
	config    *config.PySpyConfig
	collector *processtree.Collector
}

// NewDetector creates a new compatibility detector
func NewDetector(cfg *config.PySpyConfig) *Detector {
	return &Detector{
		config:    cfg,
		collector: processtree.GetCollector(),
	}
}

// Check checks py-spy compatibility for a pod
func (d *Detector) Check(ctx context.Context, req *CheckRequest) (*CheckResponse, error) {
	response := &CheckResponse{
		CheckedAt: time.Now(),
	}

	// Step 1: Check if py-spy binary is available
	if !d.checkPySpyBinary() {
		response.Supported = false
		response.Reason = "py-spy binary not found or not executable"
		return response, nil
	}

	// Step 2: Find Python processes in the pod
	pythonPIDs, err := d.findPythonProcesses(ctx, req.PodUID)
	if err != nil {
		response.Supported = false
		response.Reason = fmt.Sprintf("failed to find processes: %v", err)
		return response, nil
	}

	if len(pythonPIDs) == 0 {
		response.Supported = false
		response.Reason = "no Python processes found in pod"
		return response, nil
	}

	response.PythonProcesses = pythonPIDs

	// Step 3: Check capabilities (best effort)
	capabilities := d.checkCapabilities(pythonPIDs[0])
	response.Capabilities = capabilities

	// Step 4: Determine support based on findings
	hasPtrace := false
	for _, cap := range capabilities {
		if cap == "CAP_SYS_PTRACE" {
			hasPtrace = true
			break
		}
	}

	if hasPtrace {
		response.Supported = true
	} else {
		// Without CAP_SYS_PTRACE, py-spy might still work if running as root
		// or if the process has the same UID
		response.Supported = true // Assume supported, py-spy will fail with clear error if not
		response.Reason = "CAP_SYS_PTRACE not detected, py-spy may fail for some processes"
	}

	return response, nil
}

// checkPySpyBinary checks if py-spy binary exists and is executable
func (d *Detector) checkPySpyBinary() bool {
	info, err := os.Stat(d.config.BinaryPath)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0111 != 0
}

// findPythonProcesses finds Python processes in a pod
func (d *Detector) findPythonProcesses(ctx context.Context, podUID string) ([]int, error) {
	if d.collector == nil {
		return nil, fmt.Errorf("process tree collector not initialized")
	}

	processes, err := d.collector.FindPythonProcesses(ctx, podUID)
	if err != nil {
		return nil, err
	}

	var pids []int
	for _, proc := range processes {
		pids = append(pids, proc.HostPID)
	}

	return pids, nil
}

// checkCapabilities checks capabilities for a process
func (d *Detector) checkCapabilities(pid int) []string {
	var capabilities []string

	// Read /proc/[pid]/status to get capabilities
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		log.Debugf("Failed to read process status for PID %d: %v", pid, err)
		return capabilities
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "CapEff:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				capHex := parts[1]
				// Parse capability bits
				caps := d.parseCapabilities(capHex)
				capabilities = append(capabilities, caps...)
			}
			break
		}
	}

	return capabilities
}

// parseCapabilities parses capability hex string to capability names
func (d *Detector) parseCapabilities(capHex string) []string {
	var caps []string

	// Common capability bit positions (simplified)
	// Full list: include/uapi/linux/capability.h
	capMap := map[int]string{
		0:  "CAP_CHOWN",
		1:  "CAP_DAC_OVERRIDE",
		2:  "CAP_DAC_READ_SEARCH",
		3:  "CAP_FOWNER",
		4:  "CAP_FSETID",
		5:  "CAP_KILL",
		6:  "CAP_SETGID",
		7:  "CAP_SETUID",
		8:  "CAP_SETPCAP",
		9:  "CAP_LINUX_IMMUTABLE",
		10: "CAP_NET_BIND_SERVICE",
		11: "CAP_NET_BROADCAST",
		12: "CAP_NET_ADMIN",
		13: "CAP_NET_RAW",
		14: "CAP_IPC_LOCK",
		15: "CAP_IPC_OWNER",
		16: "CAP_SYS_MODULE",
		17: "CAP_SYS_RAWIO",
		18: "CAP_SYS_CHROOT",
		19: "CAP_SYS_PTRACE", // This is what py-spy needs
		20: "CAP_SYS_PACCT",
		21: "CAP_SYS_ADMIN",
	}

	// Parse hex string
	var capBits uint64
	fmt.Sscanf(capHex, "%x", &capBits)

	for bit, name := range capMap {
		if capBits&(1<<bit) != 0 {
			caps = append(caps, name)
		}
	}

	return caps
}

// CheckProcessAccessible checks if a specific PID is accessible for profiling
func (d *Detector) CheckProcessAccessible(pid int) error {
	// Check if /proc/[pid] exists
	procPath := fmt.Sprintf("/proc/%d", pid)
	if _, err := os.Stat(procPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("process %d does not exist", pid)
		}
		return fmt.Errorf("cannot access process %d: %v", pid, err)
	}

	// Check if we can read /proc/[pid]/maps (needed by py-spy)
	mapsPath := fmt.Sprintf("/proc/%d/maps", pid)
	if _, err := os.ReadFile(mapsPath); err != nil {
		return fmt.Errorf("cannot read process maps: %v", err)
	}

	// Check if we can read /proc/[pid]/mem (needed by py-spy)
	memPath := fmt.Sprintf("/proc/%d/mem", pid)
	if _, err := os.Open(memPath); err != nil {
		// This is expected to fail for permission reasons in some cases
		log.Debugf("Cannot open process memory (expected in some cases): %v", err)
	}

	return nil
}

