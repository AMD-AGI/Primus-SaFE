package pythoninspector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

var (
	inspectorInstance *Inspector
	once              sync.Once
)

// Inspector is the main inspection engine
type Inspector struct {
	scriptsDir    string
	scriptManager *ScriptManager
	timeout       time.Duration
	cache         sync.Map // pid -> InspectionResult
}

// InitInspector initializes the inspector
func InitInspector(ctx context.Context, scriptsDir string) error {
	var initErr error
	once.Do(func() {
		inspectorInstance = &Inspector{
			scriptsDir:    scriptsDir,
			scriptManager: NewScriptManager(scriptsDir),
			timeout:       30 * time.Second,
		}

		// Load all scripts
		if err := inspectorInstance.scriptManager.LoadScripts(); err != nil {
			log.Errorf("Failed to load scripts: %v", err)
			initErr = err
		}

		log.Info("Python Inspector initialized")
	})
	return initErr
}

// GetInspector returns the global inspector instance
func GetInspector() *Inspector {
	return inspectorInstance
}

// GetScriptManager returns the script manager
func (i *Inspector) GetScriptManager() *ScriptManager {
	return i.scriptManager
}

// InspectWithScripts inspects a process using specified scripts
func (i *Inspector) InspectWithScripts(ctx context.Context, pid int, scriptNames []string, timeout int) (*InspectionResult, error) {
	// Verify process exists
	if !i.processExists(pid) {
		return nil, fmt.Errorf("process %d not found", pid)
	}

	if !i.isPythonProcess(pid) {
		return nil, fmt.Errorf("process %d is not a Python process", pid)
	}

	// If no scripts specified, use all enabled scripts
	if len(scriptNames) == 0 {
		enabledScripts := i.scriptManager.ListEnabledScripts()
		for _, script := range enabledScripts {
			scriptNames = append(scriptNames, script.Metadata.Name)
		}
	}

	// Validate all scripts exist
	scripts := make([]*InspectionScript, 0, len(scriptNames))
	for _, name := range scriptNames {
		script, err := i.scriptManager.GetScript(name)
		if err != nil {
			log.Warnf("Script %s not available: %v", name, err)
			continue
		}
		scripts = append(scripts, script)
	}

	if len(scripts) == 0 {
		return nil, fmt.Errorf("no valid scripts to execute")
	}

	// Execute inspection
	results := make(map[string]interface{})
	startTime := time.Now()

	for _, script := range scripts {
		scriptStart := time.Now()
		result, err := i.executeScript(ctx, pid, script, timeout)
		scriptDuration := time.Since(scriptStart).Seconds()

		if err != nil {
			log.Errorf("Failed to execute script %s: %v", script.Metadata.Name, err)
			// Store error result with metadata
			results[script.Metadata.Name] = map[string]interface{}{
				"success":  false,
				"error":    err.Error(),
				"duration": scriptDuration,
			}
			continue
		}

		// Store successful result with metadata
		results[script.Metadata.Name] = map[string]interface{}{
			"success":  true,
			"data":     result,
			"duration": scriptDuration,
		}
	}

	inspectionResult := &InspectionResult{
		Success:   true,
		PID:       pid,
		Timestamp: startTime,
		Results:   results,
	}

	// Cache the result
	i.cache.Store(pid, inspectionResult)

	return inspectionResult, nil
}

// executeScript executes a single script
func (i *Inspector) executeScript(ctx context.Context, pid int, script *InspectionScript, timeoutSec int) (interface{}, error) {
	// Output file path as seen by the target process (in its /tmp)
	outputFileName := fmt.Sprintf("python_inspect_%s_%d_%d.json",
		script.Metadata.Name, pid, time.Now().Unix())
	outputFileInTarget := fmt.Sprintf("/tmp/%s", outputFileName)

	// Path to read the file from the target process's filesystem via /proc
	outputFileToRead := fmt.Sprintf("/proc/%d/root/tmp/%s", pid, outputFileName)

	// Cleanup: remove from target process's /tmp via /proc
	defer func() {
		if err := os.Remove(outputFileToRead); err != nil {
			log.Debugf("Failed to cleanup output file %s: %v", outputFileToRead, err)
		}
	}()

	// Create a temporary wrapper script that sets the output file and calls the actual script
	// This is more reliable than passing environment variables through pyrasite
	wrapperScript := fmt.Sprintf(`
import os
import sys

# Set the output file environment variable
os.environ['INSPECTOR_OUTPUT_FILE'] = '%s'

# Execute the actual inspection script
exec(open('%s').read())
`, outputFileInTarget, script.ScriptPath)

	// Write wrapper script to a temp file
	wrapperFile := fmt.Sprintf("/tmp/inspector_wrapper_%s_%d_%d.py",
		script.Metadata.Name, pid, time.Now().Unix())
	if err := os.WriteFile(wrapperFile, []byte(wrapperScript), 0644); err != nil {
		return nil, fmt.Errorf("failed to create wrapper script: %w", err)
	}
	defer os.Remove(wrapperFile)

	// Determine timeout
	timeout := i.timeout
	if timeoutSec > 0 {
		timeout = time.Duration(timeoutSec) * time.Second
	} else if script.Metadata.Timeout > 0 {
		timeout = time.Duration(script.Metadata.Timeout) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use pyrasite to inject wrapper script
	cmd := exec.CommandContext(cmdCtx, "pyrasite",
		strconv.Itoa(pid),
		wrapperFile,
	)

	log.Debugf("Executing script %s on PID %d, output file in target: %s, read from: %s",
		script.Metadata.Name, pid, outputFileInTarget, outputFileToRead)

	// Capture stdout and stderr
	output, err := cmd.CombinedOutput()

	// CRITICAL: Immediately send SIGCONT to resume the process
	// pyrasite uses gdb which stops the process, we need to resume it ASAP
	resumeCmd := exec.Command("kill", "-CONT", strconv.Itoa(pid))
	if resumeErr := resumeCmd.Run(); resumeErr != nil {
		log.Warnf("Failed to send SIGCONT to PID %d: %v", pid, resumeErr)
	} else {
		log.Debugf("Sent SIGCONT to PID %d to resume execution", pid)
	}

	// Always log the output for debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		log.Infof("Script %s execution output: %s", script.Metadata.Name, outputStr)
	}

	if err != nil {
		return nil, fmt.Errorf("script execution failed: %w, output: %s", err, outputStr)
	}

	// Wait a bit for the file to be written
	time.Sleep(200 * time.Millisecond)

	// Read result from target process's filesystem via /proc
	data, err := os.ReadFile(outputFileToRead)
	if err != nil {
		return nil, fmt.Errorf("failed to read result from %s: %w (make sure /proc is mounted)", outputFileToRead, err)
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return result, nil
}

// ListPythonProcesses lists all Python processes
func (i *Inspector) ListPythonProcesses() ([]ProcessInfo, error) {
	var processes []ProcessInfo

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pidStr := entry.Name()
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		if i.isPythonProcess(pid) {
			info := i.getProcessInfo(pid)
			processes = append(processes, info)
		}
	}

	return processes, nil
}

// processExists checks if a process exists
func (i *Inspector) processExists(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

// isPythonProcess checks if a process is a Python process
func (i *Inspector) isPythonProcess(pid int) bool {
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	return strings.Contains(string(cmdline), "python")
}

// getProcessInfo retrieves process information
func (i *Inspector) getProcessInfo(pid int) ProcessInfo {
	info := ProcessInfo{PID: pid}

	// Read cmdline
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		info.Cmdline = strings.ReplaceAll(string(data), "\x00", " ")
	}

	// Read cwd
	if cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid)); err == nil {
		info.WorkingDir = cwd
	}

	// Try to get container ID
	if cgroupData, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid)); err == nil {
		info.ContainerID = extractContainerID(string(cgroupData))
	}

	return info
}

// extractContainerID extracts container ID from cgroup data
func extractContainerID(cgroup string) string {
	lines := strings.Split(cgroup, "\n")
	for _, line := range lines {
		if strings.Contains(line, "docker") || strings.Contains(line, "containerd") {
			parts := strings.Split(line, "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	return ""
}
