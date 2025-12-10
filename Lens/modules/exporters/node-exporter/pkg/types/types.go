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
	// Option 1: Specify PID directly (highest priority)
	PID int `json:"pid,omitempty"` // Process ID to access container filesystem

	// Option 2: Specify Pod (will auto-select first process in main container)
	PodUID        string `json:"pod_uid,omitempty"`        // Pod UID (alternative to PID)
	PodName       string `json:"pod_name,omitempty"`       // Pod name (for logging/identification)
	PodNamespace  string `json:"pod_namespace,omitempty"`  // Pod namespace (for logging/identification)
	ContainerName string `json:"container_name,omitempty"` // Specific container name (optional)

	// File access parameters
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
	// Option 1: Specify PID directly (highest priority)
	PID int `json:"pid,omitempty"` // Process ID to access container filesystem

	// Option 2: Specify Pod (will auto-select first process in main container)
	PodUID        string `json:"pod_uid,omitempty"`        // Pod UID (alternative to PID)
	PodName       string `json:"pod_name,omitempty"`       // Pod name (for logging/identification)
	PodNamespace  string `json:"pod_namespace,omitempty"`  // Pod namespace (for logging/identification)
	ContainerName string `json:"container_name,omitempty"` // Specific container name (optional)

	// Directory listing parameters
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
	LogDir       string               `json:"log_dir"`
	EventFiles   []*ContainerFileInfo `json:"event_files"`
	TotalSize    int64                `json:"total_size"`
	LatestUpdate time.Time            `json:"latest_update"`
}

// ============ TensorBoard Detection Types ============

// TensorboardFileInfo represents information about an open tensorboard file
type TensorboardFileInfo struct {
	PID      int    `json:"pid"`
	FD       string `json:"fd"`
	FilePath string `json:"file_path"`
	FileName string `json:"file_name"`
}

// TensorboardFilesResponse represents the response for tensorboard file scan
type TensorboardFilesResponse struct {
	PodUID         string                 `json:"pod_uid"`
	PodName        string                 `json:"pod_name,omitempty"`
	PodNamespace   string                 `json:"pod_namespace,omitempty"`
	Files          []*TensorboardFileInfo `json:"files"`
	TotalProcesses int                    `json:"total_processes"`
	CollectedAt    time.Time              `json:"collected_at"`
}

// ============ Process Environment & Arguments Types ============

// ProcessEnvRequest represents a request to get process environment variables
type ProcessEnvRequest struct {
	PodUID       string `json:"pod_uid" binding:"required"`
	PID          int    `json:"pid,omitempty"`           // specify process PID, 0 means get all processes
	FilterPrefix string `json:"filter_prefix,omitempty"` // environment variable prefix filter
}

// ProcessEnvResponse represents process environment variables response
type ProcessEnvResponse struct {
	PodUID    string            `json:"pod_uid"`
	Processes []*ProcessEnvInfo `json:"processes"`
	Collected time.Time         `json:"collected_at"`
}

// ProcessEnvInfo represents environment variables for a single process
type ProcessEnvInfo struct {
	PID         int               `json:"pid"`
	Cmdline     string            `json:"cmdline,omitempty"`
	Environment map[string]string `json:"environment"`
}

// ProcessArgsRequest represents a request to get process arguments
type ProcessArgsRequest struct {
	PodUID string `json:"pod_uid" binding:"required"`
	PID    int    `json:"pid,omitempty"` // specify process PID, 0 means get all Python processes
}

// ProcessArgsResponse represents process arguments response
type ProcessArgsResponse struct {
	PodUID    string            `json:"pod_uid"`
	Processes []*ProcessArgInfo `json:"processes"`
	Collected time.Time         `json:"collected_at"`
}

// ProcessArgInfo represents arguments for a single process
type ProcessArgInfo struct {
	PID     int      `json:"pid"`
	Cmdline string   `json:"cmdline"`
	Args    []string `json:"args"`
}
