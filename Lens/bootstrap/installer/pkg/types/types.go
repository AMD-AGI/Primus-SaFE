package types

// State represents the state of a stage or resource
type State string

const (
	StateUnknown    State = "Unknown"
	StatePending    State = "Pending"
	StateInProgress State = "InProgress"
	StateReady      State = "Ready"
	StateFailed     State = "Failed"
)

// RunOptions contains options for running a workflow
type RunOptions struct {
	DryRun     bool
	Verbose    bool
	Kubeconfig string
	Namespace  string
}

// UninstallOptions contains options for uninstalling
type UninstallOptions struct {
	Force      bool
	DeleteData bool
}

// StageStatus represents the status of a stage
type StageStatus struct {
	Name      string           `json:"name"`
	State     State            `json:"state"`
	Message   string           `json:"message,omitempty"`
	Resources []ResourceStatus `json:"resources,omitempty"`
}

// ResourceStatus represents the status of a resource
type ResourceStatus struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	State   State  `json:"state"`
	Message string `json:"message,omitempty"`
}
