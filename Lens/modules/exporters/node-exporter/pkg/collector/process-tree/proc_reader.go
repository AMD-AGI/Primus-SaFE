package processtree

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ProcReader reads process information from /proc filesystem
type ProcReader struct{}

// NewProcReader creates a new proc reader
func NewProcReader() *ProcReader {
	return &ProcReader{}
}

// ContainerInfo represents container information
type ContainerInfo struct {
	ID    string
	Name  string
	Image string
}

// GetProcessInfo reads process information from /proc
func (r *ProcReader) GetProcessInfo(pid int, req *ProcessTreeRequest) (*ProcessInfo, error) {
	info := &ProcessInfo{
		HostPID: pid,
	}
	
	// Read /proc/[pid]/stat
	if err := r.readStat(pid, info); err != nil {
		return nil, err
	}
	
	// Read cmdline
	if req.IncludeCmdline {
		if cmdline, err := r.readCmdline(pid); err == nil {
			info.Cmdline = cmdline
			info.IsPython = strings.Contains(cmdline, "python")
			info.IsJava = strings.Contains(cmdline, "java")
		}
	}
	
	// Read exe
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		info.Exe = exe
	}
	
	// Read cwd
	if cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid)); err == nil {
		info.Cwd = cwd
	}
	
	// Read env
	if req.IncludeEnv {
		if env, err := r.readEnviron(pid); err == nil {
			info.Env = env
		}
	}
	
	// Read resource usage
	if req.IncludeResources {
		r.readStatus(pid, info)
	}
	
	return info, nil
}

// readStat reads /proc/[pid]/stat
func (r *ProcReader) readStat(pid int, info *ProcessInfo) error {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return err
	}
	
	statStr := string(data)
	
	// Parse comm (command name in parentheses)
	commStart := strings.Index(statStr, "(")
	commEnd := strings.LastIndex(statStr, ")")
	if commStart < 0 || commEnd <= commStart {
		return fmt.Errorf("invalid stat format")
	}
	
	info.Comm = statStr[commStart+1 : commEnd]
	
	// Parse fields after comm
	afterComm := strings.TrimSpace(statStr[commEnd+1:])
	fields := strings.Fields(afterComm)
	
	if len(fields) >= 2 {
		info.State = fields[0]
		if ppid, err := strconv.Atoi(fields[1]); err == nil {
			info.HostPPID = ppid
		}
	}
	
	// Parse thread count (field 19)
	if len(fields) >= 19 {
		if threads, err := strconv.Atoi(fields[17]); err == nil {
			info.Threads = threads
		}
	}
	
	// Parse start time (field 22)
	if len(fields) >= 22 {
		if startTime, err := strconv.ParseInt(fields[19], 10, 64); err == nil {
			info.StartTime = startTime
		}
	}
	
	return nil
}

// readCmdline reads /proc/[pid]/cmdline
func (r *ProcReader) readCmdline(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(data), "\x00", " "), nil
}

// readEnviron reads /proc/[pid]/environ
func (r *ProcReader) readEnviron(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\x00"), nil
}

// readStatus reads /proc/[pid]/status for memory info
func (r *ProcReader) readStatus(pid int, info *ProcessInfo) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return
	}
	
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "VmRSS:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if rss, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
					info.MemoryRSS = rss * 1024 // Convert KB to bytes
				}
			}
		} else if strings.HasPrefix(line, "VmSize:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if size, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
					info.MemoryVirtual = size * 1024
				}
			}
		}
	}
}

// FindContainerProcesses finds all processes belonging to a container
func (r *ProcReader) FindContainerProcesses(containerID string) []int {
	var pids []int
	
	// Normalize container ID
	normalizedID := normalizeContainerID(containerID)
	
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
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
		
		// Check cgroup
		cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
		data, err := os.ReadFile(cgroupPath)
		if err != nil {
			continue
		}
		
		if strings.Contains(string(data), normalizedID) {
			pids = append(pids, pid)
		}
	}
	
	return pids
}

// FindPodContainersByUID finds all containers for a pod by scanning /proc
func (r *ProcReader) FindPodContainersByUID(podUID string) []*ContainerInfo {
	containerMap := make(map[string]*ContainerInfo)
	
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
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
		
		// Read cgroup
		cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
		data, err := os.ReadFile(cgroupPath)
		if err != nil {
			continue
		}
		
		cgroupStr := string(data)
		if !strings.Contains(cgroupStr, podUID) {
			continue
		}
		
		// Extract container ID
		containerID := extractContainerIDFromCgroup(cgroupStr)
		if containerID != "" {
			if _, exists := containerMap[containerID]; !exists {
				containerMap[containerID] = &ContainerInfo{
					ID:   containerID,
					Name: fmt.Sprintf("container-%s", containerID[:12]),
				}
			}
		}
	}
	
	// Convert map to slice
	containers := make([]*ContainerInfo, 0, len(containerMap))
	for _, container := range containerMap {
		containers = append(containers, container)
	}
	
	return containers
}

// normalizeContainerID removes common prefixes from container ID
func normalizeContainerID(id string) string {
	id = strings.TrimPrefix(id, "containerd://")
	id = strings.TrimPrefix(id, "docker://")
	id = strings.TrimPrefix(id, "cri-o://")
	return id
}

// extractContainerIDFromCgroup extracts container ID from cgroup data
func extractContainerIDFromCgroup(cgroup string) string {
	lines := strings.Split(cgroup, "\n")
	for _, line := range lines {
		parts := strings.Split(line, "/")
		for idx, part := range parts {
			// containerd format: cri-containerd-<id>.scope
			if strings.HasPrefix(part, "cri-containerd-") {
				id := strings.TrimPrefix(part, "cri-containerd-")
				id = strings.TrimSuffix(id, ".scope")
				return id
			}
			// docker format: docker-<id>.scope
			if strings.HasPrefix(part, "docker-") {
				id := strings.TrimPrefix(part, "docker-")
				id = strings.TrimSuffix(id, ".scope")
				return id
			}
			// cri-o format: crio-<id>.scope
			if strings.HasPrefix(part, "crio-") {
				id := strings.TrimPrefix(part, "crio-")
				id = strings.TrimSuffix(id, ".scope")
				return id
			}
			// Direct container ID (alphanumeric, >40 chars)
			if idx+1 < len(parts) && len(parts[idx+1]) >= 40 {
				potentialID := parts[idx+1]
				if isAlphanumeric(potentialID) {
					return potentialID
				}
			}
		}
	}
	return ""
}

// isAlphanumeric checks if a string contains only alphanumeric characters
func isAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return len(s) > 0
}

// ScanTensorboardFiles scans all processes for open tensorboard event files
func (r *ProcReader) ScanTensorboardFiles(pids []int) []*TensorboardFileInfo {
	var tensorboardFiles []*TensorboardFileInfo
	
	for _, pid := range pids {
		// Read /proc/[pid]/fd directory
		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fdEntries, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		
		// Check each file descriptor
		for _, fdEntry := range fdEntries {
			fdPath := fmt.Sprintf("%s/%s", fdDir, fdEntry.Name())
			target, err := os.Readlink(fdPath)
			if err != nil {
				continue
			}
			
			// Check if the file is a tensorboard event file
			if strings.Contains(target, "tensorboard") || strings.Contains(target, "tfevents") {
				tensorboardFiles = append(tensorboardFiles, &TensorboardFileInfo{
					PID:      pid,
					FD:       fdEntry.Name(),
					FilePath: target,
					FileName: extractFileName(target),
				})
			}
		}
	}
	
	return tensorboardFiles
}

// extractFileName extracts the filename from a full path
func extractFileName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

