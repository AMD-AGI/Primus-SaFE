package processtree

import "time"

// ProcessInfo represents detailed information about a process
type ProcessInfo struct {
	// Host-level information
	HostPID   int    `json:"host_pid"`
	HostPPID  int    `json:"host_ppid"`
	
	// Container-level information
	ContainerPID  int    `json:"container_pid,omitempty"`
	ContainerPPID int    `json:"container_ppid,omitempty"`
	
	// Process details
	Cmdline       string   `json:"cmdline"`
	Comm          string   `json:"comm"`
	Exe           string   `json:"exe,omitempty"`
	Args          []string `json:"args,omitempty"`
	Env           []string `json:"env,omitempty"`
	Cwd           string   `json:"cwd,omitempty"`
	
	// Process state
	State         string `json:"state"`
	Threads       int    `json:"threads"`
	
	// Resource usage
	CPUTime       uint64 `json:"cpu_time,omitempty"`
	MemoryRSS     uint64 `json:"memory_rss,omitempty"`
	MemoryVirtual uint64 `json:"memory_virtual,omitempty"`
	
	// Container context
	ContainerID   string `json:"container_id,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	PodUID        string `json:"pod_uid,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	PodNamespace  string `json:"pod_namespace,omitempty"`
	
	// Process classification
	IsPython      bool   `json:"is_python"`
	IsJava        bool   `json:"is_java"`
	
	// Timestamps
	StartTime     int64  `json:"start_time,omitempty"`
	
	// Tree structure
	Children      []*ProcessInfo `json:"children,omitempty"`
}

// ContainerProcessTree represents the process tree for a container
type ContainerProcessTree struct {
	ContainerID   string         `json:"container_id"`
	ContainerName string         `json:"container_name"`
	ImageName     string         `json:"image_name,omitempty"`
	RootProcess   *ProcessInfo   `json:"root_process"`
	AllProcesses  []*ProcessInfo `json:"all_processes"`
	ProcessCount  int            `json:"process_count"`
	PythonCount   int            `json:"python_count"`
}

// PodProcessTree represents the complete process tree for a pod
type PodProcessTree struct {
	// Pod information
	PodName      string    `json:"pod_name"`
	PodNamespace string    `json:"pod_namespace"`
	PodUID       string    `json:"pod_uid"`
	NodeName     string    `json:"node_name,omitempty"`
	
	// Container trees
	Containers   []*ContainerProcessTree `json:"containers"`
	
	// Summary
	TotalProcesses int `json:"total_processes"`
	TotalPython    int `json:"total_python"`
	
	// Timestamps
	CollectedAt  time.Time `json:"collected_at"`
}

// ProcessTreeRequest represents a request to get process tree
type ProcessTreeRequest struct {
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	PodUID       string `json:"pod_uid"`
	
	// Options
	IncludeEnv       bool `json:"include_env"`
	IncludeCmdline   bool `json:"include_cmdline"`
	IncludeResources bool `json:"include_resources"`
}

// ProcessState represents process states
type ProcessState string

const (
	ProcessStateRunning            ProcessState = "R" // Running
	ProcessStateSleeping           ProcessState = "S" // Sleeping (interruptible)
	ProcessStateDiskSleep          ProcessState = "D" // Disk sleep (uninterruptible)
	ProcessStateZombie             ProcessState = "Z" // Zombie
	ProcessStateStopped            ProcessState = "T" // Stopped
	ProcessStateTracingStop        ProcessState = "t" // Tracing stop
	ProcessStateDead               ProcessState = "X" // Dead
)

