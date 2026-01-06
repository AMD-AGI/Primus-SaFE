package pyspy

import "time"

// ExecuteRequest represents a request to execute py-spy sampling
type ExecuteRequest struct {
	TaskID       string `json:"task_id" binding:"required"`
	PodUID       string `json:"pod_uid" binding:"required"`
	HostPID      int    `json:"host_pid" binding:"required"`
	ContainerPID int    `json:"container_pid,omitempty"`
	Duration     int    `json:"duration"`  // seconds
	Rate         int    `json:"rate"`      // Hz
	Format       string `json:"format"`    // flamegraph, speedscope, raw
	Native       bool   `json:"native"`
	SubProcesses bool   `json:"subprocesses"`
}

// ExecuteResponse represents the response from py-spy execution
type ExecuteResponse struct {
	Success    bool   `json:"success"`
	OutputFile string `json:"output_file,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
	Error      string `json:"error,omitempty"`
}

// CheckRequest represents a request to check py-spy compatibility
type CheckRequest struct {
	PodUID      string `json:"pod_uid" binding:"required"`
	ContainerID string `json:"container_id,omitempty"`
}

// CheckResponse represents the py-spy compatibility check result
type CheckResponse struct {
	Supported       bool      `json:"supported"`
	Reason          string    `json:"reason,omitempty"`
	PythonProcesses []int     `json:"python_processes,omitempty"`
	Capabilities    []string  `json:"capabilities,omitempty"`
	CheckedAt       time.Time `json:"checked_at"`
}

// FileListRequest represents a request to list py-spy files
type FileListRequest struct {
	TaskID string `json:"task_id,omitempty"`
	PodUID string `json:"pod_uid,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// FileInfo represents metadata for a py-spy output file
type FileInfo struct {
	TaskID    string    `json:"task_id"`
	FileName  string    `json:"file_name"`
	FilePath  string    `json:"file_path"`
	FileSize  int64     `json:"file_size"`
	Format    string    `json:"format"`
	CreatedAt time.Time `json:"created_at"`
}

// FileListResponse represents the response listing py-spy files
type FileListResponse struct {
	Files      []*FileInfo `json:"files"`
	TotalCount int         `json:"total_count"`
}

// OutputFormat represents the output format of py-spy
type OutputFormat string

const (
	FormatFlamegraph OutputFormat = "flamegraph"
	FormatSpeedscope OutputFormat = "speedscope"
	FormatRaw        OutputFormat = "raw"
)

// GetFileExtension returns the file extension for the output format
func (f OutputFormat) GetFileExtension() string {
	switch f {
	case FormatFlamegraph:
		return "svg"
	case FormatSpeedscope:
		return "json"
	case FormatRaw:
		return "txt"
	default:
		return "svg"
	}
}

// ParseOutputFormat parses a string to OutputFormat
func ParseOutputFormat(s string) OutputFormat {
	switch s {
	case "flamegraph", "svg":
		return FormatFlamegraph
	case "speedscope", "json":
		return FormatSpeedscope
	case "raw", "txt":
		return FormatRaw
	default:
		return FormatFlamegraph
	}
}

