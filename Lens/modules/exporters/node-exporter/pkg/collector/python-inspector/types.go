package pythoninspector

import "time"

// ScriptMetadata represents the metadata of an inspection script
type ScriptMetadata struct {
	Name          string                 `yaml:"name" json:"name"`
	Version       string                 `yaml:"version" json:"version"`
	Description   string                 `yaml:"description" json:"description"`
	Author        string                 `yaml:"author" json:"author"`
	Email         string                 `yaml:"email" json:"email"`
	Category      string                 `yaml:"category" json:"category"` // universal, framework_specific
	Capabilities  []string               `yaml:"capabilities" json:"capabilities"`
	Frameworks    []string               `yaml:"frameworks" json:"frameworks"` // Empty means universal
	PythonVersion string                 `yaml:"python_version" json:"python_version"`
	Dependencies  []string               `yaml:"dependencies" json:"dependencies"`
	Targets       []Target               `yaml:"targets" json:"targets"`
	OutputSchema  map[string]SchemaField `yaml:"output_schema" json:"output_schema"`
	Timeout       int                    `yaml:"timeout" json:"timeout"`
	SafetyLevel   string                 `yaml:"safety_level" json:"safety_level"` // safe, cautious, dangerous
	Tags          []string               `yaml:"tags" json:"tags"`
	Enabled       bool                   `yaml:"enabled" json:"enabled"`
}

// Target represents a detection target
type Target struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"` // class, function, module
	Module string `yaml:"module"`
}

// SchemaField represents an output field schema
type SchemaField struct {
	Type        string              `yaml:"type"`
	Description string              `yaml:"description"`
	Items       []map[string]string `yaml:"items,omitempty"`
}

// InspectionScript represents a complete inspection script
type InspectionScript struct {
	Metadata   ScriptMetadata
	ScriptPath string
}

// InspectionResult represents the result of an inspection
type InspectionResult struct {
	Success   bool                   `json:"success"`
	PID       int                    `json:"pid"`
	Timestamp time.Time              `json:"timestamp"`
	Results   map[string]interface{} `json:"results"`
	Error     string                 `json:"error,omitempty"`
}

// ProcessInfo represents information about a process
type ProcessInfo struct {
	PID         int    `json:"pid"`
	Cmdline     string `json:"cmdline"`
	WorkingDir  string `json:"working_dir"`
	ContainerID string `json:"container_id,omitempty"`
}

// InspectRequest represents an inspection request
type InspectRequest struct {
	PID     int      `json:"pid" binding:"required"`
	Scripts []string `json:"scripts"` // Optional, if empty uses all enabled scripts
	Timeout int      `json:"timeout"` // Optional
}
