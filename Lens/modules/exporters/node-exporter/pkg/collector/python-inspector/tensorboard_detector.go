package pythoninspector

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// TensorBoardInfo contains information about TensorBoard usage
type TensorBoardInfo struct {
	Enabled    bool     `json:"enabled"`
	LogDirs    []string `json:"log_dirs,omitempty"`
	EventFiles []string `json:"event_files,omitempty"`
	Method     string   `json:"detection_method"`
}

// DetectTensorBoardNonInvasive detects TensorBoard without injecting code
func DetectTensorBoardNonInvasive(pid int) (*TensorBoardInfo, error) {
	log.Infof("Starting non-invasive TensorBoard detection for PID %d", pid)

	// Strategy 1: Check open file descriptors (fastest, most reliable)
	if info, err := detectByOpenFiles(pid); err == nil && info.Enabled {
		info.Method = "open_files"
		return info, nil
	}

	// Strategy 2: Check memory maps
	if info, err := detectByMaps(pid); err == nil && info.Enabled {
		info.Method = "memory_maps"
		return info, nil
	}

	// Strategy 3: Search filesystem (slower but thorough)
	if info, err := detectByFileSystem(pid); err == nil && info.Enabled {
		info.Method = "filesystem_search"
		return info, nil
	}

	// Strategy 4: Check environment and cmdline
	if info, err := detectByEnvAndCmdline(pid); err == nil && info.Enabled {
		info.Method = "env_cmdline"
		return info, nil
	}

	// Strategy 5: Use py-spy if available
	if info, err := detectByPySpy(pid); err == nil && info.Enabled {
		info.Method = "py-spy"
		return info, nil
	}

	// Not detected
	return &TensorBoardInfo{
		Enabled: false,
		Method:  "none",
	}, nil
}

// detectByOpenFiles checks open file descriptors for TensorBoard event files
func detectByOpenFiles(pid int) (*TensorBoardInfo, error) {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, err
	}

	logDirsMap := make(map[string]bool)
	var eventFiles []string

	for _, entry := range entries {
		linkPath := filepath.Join(fdDir, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}

		// Check if it's a TensorBoard event file
		if strings.Contains(target, "events.out.tfevents") {
			eventFiles = append(eventFiles, target)
			dir := filepath.Dir(target)
			logDirsMap[dir] = true
		}
	}

	var logDirs []string
	for dir := range logDirsMap {
		logDirs = append(logDirs, dir)
	}

	return &TensorBoardInfo{
		Enabled:    len(eventFiles) > 0,
		LogDirs:    logDirs,
		EventFiles: eventFiles,
	}, nil
}

// detectByMaps checks memory maps for tensorboard modules
func detectByMaps(pid int) (*TensorBoardInfo, error) {
	mapsFile := fmt.Sprintf("/proc/%d/maps", pid)
	data, err := os.ReadFile(mapsFile)
	if err != nil {
		return nil, err
	}

	content := strings.ToLower(string(data))
	enabled := strings.Contains(content, "tensorboard") ||
		strings.Contains(content, "events.out.tfevents")

	return &TensorBoardInfo{
		Enabled: enabled,
	}, nil
}

// detectByFileSystem searches for TensorBoard event files in process working directory
func detectByFileSystem(pid int) (*TensorBoardInfo, error) {
	// Get process working directory
	cwdLink := fmt.Sprintf("/proc/%d/cwd", pid)
	cwd, err := os.Readlink(cwdLink)
	if err != nil {
		return nil, err
	}

	var eventFiles []string
	logDirsMap := make(map[string]bool)

	// Search for event files (limit depth to avoid too slow search)
	maxDepth := 5
	err = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}

		// Calculate depth
		rel, _ := filepath.Rel(cwd, path)
		depth := strings.Count(rel, string(os.PathSeparator))
		if depth > maxDepth {
			return filepath.SkipDir
		}

		if !d.IsDir() && strings.Contains(d.Name(), "events.out.tfevents") {
			eventFiles = append(eventFiles, path)
			dir := filepath.Dir(path)
			logDirsMap[dir] = true

			// Stop after finding a few files to avoid slow search
			if len(eventFiles) >= 5 {
				return filepath.SkipAll
			}
		}
		return nil
	})

	var logDirs []string
	for dir := range logDirsMap {
		logDirs = append(logDirs, dir)
	}

	return &TensorBoardInfo{
		Enabled:    len(eventFiles) > 0,
		LogDirs:    logDirs,
		EventFiles: eventFiles,
	}, nil
}

// detectByEnvAndCmdline checks environment variables and command line
func detectByEnvAndCmdline(pid int) (*TensorBoardInfo, error) {
	info := &TensorBoardInfo{}

	// Check environment variables
	envFile := fmt.Sprintf("/proc/%d/environ", pid)
	if envData, err := os.ReadFile(envFile); err == nil {
		envVars := strings.Split(string(envData), "\x00")
		for _, env := range envVars {
			envLower := strings.ToLower(env)
			if strings.Contains(envLower, "tensorboard") {
				info.Enabled = true
				// Try to extract log dir from env
				if strings.HasPrefix(envLower, "tensorboard_log_dir=") ||
					strings.HasPrefix(envLower, "tb_log_dir=") {
					parts := strings.SplitN(env, "=", 2)
					if len(parts) == 2 {
						info.LogDirs = append(info.LogDirs, parts[1])
					}
				}
			}
		}
	}

	// Check command line arguments
	cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", pid)
	if cmdlineData, err := os.ReadFile(cmdlineFile); err == nil {
		cmdline := strings.ToLower(string(cmdlineData))
		if strings.Contains(cmdline, "tensorboard") ||
			strings.Contains(cmdline, "--log") ||
			strings.Contains(cmdline, "--logdir") {
			info.Enabled = true
		}
	}

	return info, nil
}

// detectByPySpy uses py-spy to check stack traces (requires py-spy installed)
func detectByPySpy(pid int) (*TensorBoardInfo, error) {
	// Check if py-spy is available
	if _, err := exec.LookPath("py-spy"); err != nil {
		return &TensorBoardInfo{Enabled: false}, fmt.Errorf("py-spy not available: %w", err)
	}

	// Run py-spy dump (non-blocking, doesn't stop the process)
	cmd := exec.Command("py-spy", "dump", "--pid", fmt.Sprintf("%d", pid))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("py-spy failed: %w", err)
	}

	content := strings.ToLower(string(output))
	enabled := strings.Contains(content, "tensorboard") ||
		strings.Contains(content, "summarywriter") ||
		strings.Contains(content, "filewriter")

	return &TensorBoardInfo{
		Enabled: enabled,
	}, nil
}

