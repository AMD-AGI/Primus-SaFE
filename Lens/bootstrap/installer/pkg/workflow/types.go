package workflow

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/types"
)

// Re-export types for convenience
type (
	State            = types.State
	RunOptions       = types.RunOptions
	UninstallOptions = types.UninstallOptions
	StageStatus      = types.StageStatus
	ResourceStatus   = types.ResourceStatus
)

// Re-export constants
const (
	StateUnknown    = types.StateUnknown
	StatePending    = types.StatePending
	StateInProgress = types.StateInProgress
	StateReady      = types.StateReady
	StateFailed     = types.StateFailed
)

// Workflow defines the interface for installation workflows
type Workflow interface {
	// Name returns the workflow name
	Name() string

	// Install runs the installation workflow
	Install(ctx context.Context, opts RunOptions) error

	// Uninstall runs the uninstallation workflow
	Uninstall(ctx context.Context, opts RunOptions, uninstallOpts UninstallOptions) error

	// Status returns the current status of the workflow
	Status(ctx context.Context, opts RunOptions) (*Status, error)
}

// Status represents the status of a workflow
type Status struct {
	WorkflowName string        `json:"workflowName"`
	OverallState State         `json:"overallState"`
	Stages       []StageStatus `json:"stages"`
}

// Stage defines the interface for a deployment stage
type Stage interface {
	// Name returns the stage name
	Name() string

	// Run executes the stage
	Run(ctx context.Context, opts RunOptions) error

	// Verify verifies that the stage completed successfully
	Verify(ctx context.Context, opts RunOptions) (*StageStatus, error)

	// Rollback rolls back the stage
	Rollback(ctx context.Context, opts RunOptions) error
}

// WorkflowFactory creates workflows based on name
type WorkflowFactory func(cfg *config.Config) (Workflow, error)

// registry holds registered workflow factories
var registry = make(map[string]WorkflowFactory)

// RegisterWorkflow registers a workflow factory
func RegisterWorkflow(name string, factory WorkflowFactory) {
	registry[name] = factory
}

// NewWorkflow creates a new workflow instance
func NewWorkflow(name string, cfg *config.Config) (Workflow, error) {
	factory, ok := registry[name]
	if !ok {
		return nil, &ErrUnknownWorkflow{Name: name}
	}
	return factory(cfg)
}

// ListWorkflows returns the names of registered workflows
func ListWorkflows() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// ErrUnknownWorkflow is returned when an unknown workflow is requested
type ErrUnknownWorkflow struct {
	Name string
}

func (e *ErrUnknownWorkflow) Error() string {
	return "unknown workflow: " + e.Name
}
