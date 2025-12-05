package types

import (
	"time"
)

// ============ Process Tree Types ============

// ProcessTreeRequest represents process tree request
type ProcessTreeRequest struct {
	PodName          string `json:"pod_name"`
	PodNamespace     string `json:"pod_namespace"`
	PodUID           string `json:"pod_uid"`
	IncludeEnv       bool   `json:"include_env"`
	IncludeCmdline   bool   `json:"include_cmdline"`
	IncludeResources bool   `json:"include_resources"`
}


// ProcessInfo represents process information
type ProcessInfo struct {
	HostPID       int            `json:"host_pid"`
	HostPPID      int            `json:"host_ppid"`
	ContainerPID  int            `json:"container_pid,omitempty"`
	ContainerPPID int            `json:"container_ppid,omitempty"`
	Cmdline       string         `json:"cmdline"`
	Comm          string         `json:"comm"`
	Exe           string         `json:"exe,omitempty"`
	Args          []string       `json:"args,omitempty"`
	Env           []string       `json:"env,omitempty"`
	Cwd           string         `json:"cwd,omitempty"`
	State         string         `json:"state"`
	Threads       int            `json:"threads"`
	CPUTime       uint64         `json:"cpu_time,omitempty"`
	MemoryRSS     uint64         `json:"memory_rss,omitempty"`
	MemoryVirtual uint64         `json:"memory_virtual,omitempty"`
	ContainerID   string         `json:"container_id,omitempty"`
	ContainerName string         `json:"container_name,omitempty"`
	PodUID        string         `json:"pod_uid,omitempty"`
	PodName       string         `json:"pod_name,omitempty"`
	PodNamespace  string         `json:"pod_namespace,omitempty"`
	IsPython      bool           `json:"is_python"`
	IsJava        bool           `json:"is_java"`
	StartTime     int64          `json:"start_time,omitempty"`
	Children      []*ProcessInfo `json:"children,omitempty"`
}

// ContainerProcessTree represents container process tree
type ContainerProcessTree struct {
	ContainerID   string         `json:"container_id"`
	ContainerName string         `json:"container_name"`
	ImageName     string         `json:"image_name,omitempty"`
	RootProcess   *ProcessInfo   `json:"root_process"`
	AllProcesses  []*ProcessInfo `json:"all_processes"`
	ProcessCount  int            `json:"process_count"`
	PythonCount   int            `json:"python_count"`
}

// PodProcessTree represents pod process tree
type PodProcessTree struct {
	PodName        string                  `json:"pod_name"`
	PodNamespace   string                  `json:"pod_namespace"`
	PodUID         string                  `json:"pod_uid"`
	NodeName       string                  `json:"node_name,omitempty"`
	Containers     []*ContainerProcessTree `json:"containers"`
	TotalProcesses int                     `json:"total_processes"`
	TotalPython    int                     `json:"total_python"`
	CollectedAt    time.Time               `json:"collected_at"`
}

// ============ Python Inspector Types ============

// InspectRequest represents inspection request
type InspectRequest struct {
	PID     int      `json:"pid"`
	Scripts []string `json:"scripts"`
	Timeout int      `json:"timeout,omitempty"` // seconds
}

// InspectionResult represents inspection result
type InspectionResult struct {
	PID       int                    `json:"pid"`
	Success   bool                   `json:"success"`
	Script    string                 `json:"script"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  float64                `json:"duration"` // seconds
}

// ScriptMetadata represents inspection script metadata
type ScriptMetadata struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Description  string                 `json:"description"`
	Author       string                 `json:"author,omitempty"`
	Category     string                 `json:"category,omitempty"` // universal, framework_specific
	Capabilities []string               `json:"capabilities"`
	Frameworks   []string               `json:"frameworks"` // Empty means universal
	Tags         []string               `json:"tags,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
}

// ============ Container Filesystem Types ============

// ContainerFileInfo represents file metadata from container
type ContainerFileInfo struct {
	Path          string    `json:"path"`
	Size          int64     `json:"size"`
	Mode          string    `json:"mode"`
	ModTime       time.Time `json:"mod_time"`
	IsDir         bool      `json:"is_dir"`
	IsSymlink     bool      `json:"is_symlink"`
	SymlinkTarget string    `json:"symlink_target,omitempty"`
}

// ContainerFileReadRequest represents a file read request
type ContainerFileReadRequest struct {
	PID            int    `json:"pid" binding:"required"`
	Path           string `json:"path" binding:"required"`
	Offset         int64  `json:"offset,omitempty"`
	Length         int64  `json:"length,omitempty"`
	FollowSymlinks bool   `json:"follow_symlinks,omitempty"`
}

// ContainerFileReadResponse represents file read response
type ContainerFileReadResponse struct {
	Content   string             `json:"content,omitempty"`
	FileInfo  *ContainerFileInfo `json:"file_info"`
	BytesRead int64              `json:"bytes_read"`
	EOF       bool               `json:"eof"`
	IsBinary  bool               `json:"is_binary,omitempty"`
}

// ContainerDirectoryListRequest represents a directory listing request
type ContainerDirectoryListRequest struct {
	PID       int    `json:"pid" binding:"required"`
	Path      string `json:"path" binding:"required"`
	Recursive bool   `json:"recursive,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

// ContainerDirectoryListResponse represents directory listing response
type ContainerDirectoryListResponse struct {
	Files []*ContainerFileInfo `json:"files"`
	Total int                  `json:"total"`
}

// TensorBoardLogInfo represents TensorBoard log directory information
type TensorBoardLogInfo struct {
	LogDir       string              `json:"log_dir"`
	EventFiles   []*ContainerFileInfo `json:"event_files"`
	TotalSize    int64               `json:"total_size"`
	LatestUpdate time.Time           `json:"latest_update"`
}

