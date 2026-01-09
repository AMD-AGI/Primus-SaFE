// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Executor handles py-spy command execution
type Executor struct {
	config        *config.PySpyConfig
	fileStore     *FileStore
	runningTasks  map[string]context.CancelFunc
	runningMu     sync.Mutex
	semaphore     chan struct{}
}

// NewExecutor creates a new py-spy executor
func NewExecutor(cfg *config.PySpyConfig, fileStore *FileStore) *Executor {
	return &Executor{
		config:       cfg,
		fileStore:    fileStore,
		runningTasks: make(map[string]context.CancelFunc),
		semaphore:    make(chan struct{}, cfg.MaxConcurrentJobs),
	}
}

// Execute runs py-spy record command
func (e *Executor) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	// Apply defaults
	duration := req.Duration
	if duration <= 0 {
		duration = e.config.DefaultDuration
	}

	rate := req.Rate
	if rate <= 0 {
		rate = e.config.DefaultRate
	}

	format := ParseOutputFormat(req.Format)

	// Acquire semaphore to limit concurrent jobs
	select {
	case e.semaphore <- struct{}{}:
		defer func() { <-e.semaphore }()
	case <-ctx.Done():
		return &ExecuteResponse{
			Success: false,
			Error:   "context cancelled while waiting for execution slot",
		}, nil
	default:
		return &ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("max concurrent jobs (%d) reached", e.config.MaxConcurrentJobs),
		}, nil
	}

	// Create context with timeout (duration + buffer for startup/cleanup)
	timeout := time.Duration(duration+30) * time.Second
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Track running task
	e.runningMu.Lock()
	e.runningTasks[req.TaskID] = cancel
	e.runningMu.Unlock()
	defer func() {
		e.runningMu.Lock()
		delete(e.runningTasks, req.TaskID)
		e.runningMu.Unlock()
	}()

	// Prepare output file
	outputFile, err := e.fileStore.PrepareOutputFile(req.TaskID, format)
	if err != nil {
		return &ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to prepare output file: %v", err),
		}, nil
	}

	// Build py-spy command
	args := e.buildArgs(req.HostPID, outputFile, duration, rate, format, req.Native, req.SubProcesses)

	log.Infof("Executing py-spy: %s %v", e.config.BinaryPath, args)

	// Execute py-spy
	cmd := exec.CommandContext(execCtx, e.config.BinaryPath, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if context was cancelled
		if execCtx.Err() == context.DeadlineExceeded {
			return &ExecuteResponse{
				Success: false,
				Error:   "py-spy execution timed out",
			}, nil
		}
		if execCtx.Err() == context.Canceled {
			return &ExecuteResponse{
				Success: false,
				Error:   "py-spy execution was cancelled",
			}, nil
		}

		log.Errorf("py-spy execution failed: %v, output: %s", err, string(output))
		return &ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("py-spy failed: %v, output: %s", err, string(output)),
		}, nil
	}

	// Get file info
	fileInfo, err := os.Stat(outputFile)
	if err != nil {
		return &ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("output file not found: %v", err),
		}, nil
	}

	// Register the file with file store
	e.fileStore.RegisterFile(req.TaskID, outputFile, string(format))

	log.Infof("py-spy execution completed: task=%s, file=%s, size=%d", req.TaskID, outputFile, fileInfo.Size())

	return &ExecuteResponse{
		Success:    true,
		OutputFile: outputFile,
		FileSize:   fileInfo.Size(),
	}, nil
}

// buildArgs constructs py-spy command arguments
func (e *Executor) buildArgs(pid int, outputFile string, duration, rate int, format OutputFormat, native, subprocesses bool) []string {
	args := []string{
		"record",
		"--pid", strconv.Itoa(pid),
		"--output", outputFile,
		"--duration", strconv.Itoa(duration),
		"--rate", strconv.Itoa(rate),
	}

	// Add format argument
	switch format {
	case FormatFlamegraph:
		args = append(args, "--format", "flamegraph")
	case FormatSpeedscope:
		args = append(args, "--format", "speedscope")
	case FormatRaw:
		args = append(args, "--format", "raw")
	}

	// Add optional flags
	if native {
		args = append(args, "--native")
	}
	if subprocesses {
		args = append(args, "--subprocesses")
	}

	return args
}

// CancelTask cancels a running py-spy task
func (e *Executor) CancelTask(taskID string) bool {
	e.runningMu.Lock()
	defer e.runningMu.Unlock()

	if cancel, ok := e.runningTasks[taskID]; ok {
		cancel()
		return true
	}
	return false
}

// IsTaskRunning checks if a task is currently running
func (e *Executor) IsTaskRunning(taskID string) bool {
	e.runningMu.Lock()
	defer e.runningMu.Unlock()

	_, ok := e.runningTasks[taskID]
	return ok
}

// GetRunningTaskCount returns the number of currently running tasks
func (e *Executor) GetRunningTaskCount() int {
	e.runningMu.Lock()
	defer e.runningMu.Unlock()

	return len(e.runningTasks)
}

// CheckPySpyAvailable checks if py-spy binary is available and executable
func (e *Executor) CheckPySpyAvailable() error {
	info, err := os.Stat(e.config.BinaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("py-spy binary not found at %s", e.config.BinaryPath)
		}
		return fmt.Errorf("failed to stat py-spy binary: %v", err)
	}

	if info.IsDir() {
		return fmt.Errorf("py-spy path is a directory: %s", e.config.BinaryPath)
	}

	// Check if executable
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("py-spy binary is not executable: %s", e.config.BinaryPath)
	}

	// Try to run py-spy --version
	cmd := exec.Command(e.config.BinaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("py-spy version check failed: %v, output: %s", err, string(output))
	}

	log.Infof("py-spy available: %s", string(output))
	return nil
}

// GetOutputFilePath returns the expected output file path for a task
func (e *Executor) GetOutputFilePath(taskID string, format OutputFormat) string {
	ext := format.GetFileExtension()
	fileName := fmt.Sprintf("profile.%s", ext)
	return filepath.Join(e.config.StoragePath, "profiles", taskID, fileName)
}

