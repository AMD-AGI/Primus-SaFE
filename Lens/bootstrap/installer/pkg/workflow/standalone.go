package workflow

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/config"
)

func init() {
	RegisterWorkflow("standalone", NewStandaloneWorkflow)
}

// StandaloneWorkflow combines dataplane and controlplane into a single workflow
type StandaloneWorkflow struct {
	name         string
	config       *config.Config
	dataplane    Workflow
	controlplane Workflow
}

// NewStandaloneWorkflow creates a new standalone workflow
func NewStandaloneWorkflow(cfg *config.Config) (Workflow, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	dataplane, err := NewDataplaneWorkflow(cfg)
	if err != nil {
		return nil, err
	}

	controlplane, err := NewControlplaneWorkflow(cfg)
	if err != nil {
		return nil, err
	}

	return &StandaloneWorkflow{
		name:         "standalone",
		config:       cfg,
		dataplane:    dataplane,
		controlplane: controlplane,
	}, nil
}

// Name returns the workflow name
func (w *StandaloneWorkflow) Name() string {
	return w.name
}

// SetValuesFile sets the values file path for both dataplane and controlplane
func (w *StandaloneWorkflow) SetValuesFile(file string) {
	if setter, ok := w.dataplane.(interface{ SetValuesFile(string) }); ok {
		setter.SetValuesFile(file)
	}
	if setter, ok := w.controlplane.(interface{ SetValuesFile(string) }); ok {
		setter.SetValuesFile(file)
	}
}

// Install runs both dataplane and controlplane installation
func (w *StandaloneWorkflow) Install(ctx context.Context, opts RunOptions) error {
	// First install dataplane
	if err := w.dataplane.Install(ctx, opts); err != nil {
		return err
	}

	// Then install controlplane
	return w.controlplane.Install(ctx, opts)
}

// Uninstall runs both controlplane and dataplane uninstallation (reverse order)
func (w *StandaloneWorkflow) Uninstall(ctx context.Context, opts RunOptions, uninstallOpts UninstallOptions) error {
	// First uninstall controlplane
	if err := w.controlplane.Uninstall(ctx, opts, uninstallOpts); err != nil {
		if !uninstallOpts.Force {
			return err
		}
	}

	// Then uninstall dataplane
	return w.dataplane.Uninstall(ctx, opts, uninstallOpts)
}

// Status returns combined status
func (w *StandaloneWorkflow) Status(ctx context.Context, opts RunOptions) (*Status, error) {
	dpStatus, err := w.dataplane.Status(ctx, opts)
	if err != nil {
		return nil, err
	}

	cpStatus, err := w.controlplane.Status(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Combine statuses
	combined := &Status{
		WorkflowName: w.name,
		OverallState: StateReady,
		Stages:       append(dpStatus.Stages, cpStatus.Stages...),
	}

	// Calculate overall state
	for _, stage := range combined.Stages {
		if stage.State == StateFailed {
			combined.OverallState = StateFailed
			break
		} else if stage.State == StateInProgress {
			combined.OverallState = StateInProgress
		} else if stage.State == StatePending && combined.OverallState == StateReady {
			combined.OverallState = StatePending
		}
	}

	return combined, nil
}
