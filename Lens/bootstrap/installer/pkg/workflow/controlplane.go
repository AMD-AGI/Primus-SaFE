package workflow

import (
	"path/filepath"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/stage"
)

func init() {
	RegisterWorkflow("controlplane", NewControlplaneWorkflow)
}

// ControlplaneWorkflow implements the controlplane installation workflow
type ControlplaneWorkflow struct {
	*BaseWorkflow
	valuesFile string
}

// NewControlplaneWorkflow creates a new controlplane workflow
func NewControlplaneWorkflow(cfg *config.Config) (Workflow, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	wf := &ControlplaneWorkflow{
		BaseWorkflow: NewBaseWorkflow("controlplane", cfg),
		valuesFile:   "",
	}

	wf.setupStages()
	return wf, nil
}

// SetValuesFile sets the values file path and re-initializes stages
func (w *ControlplaneWorkflow) SetValuesFile(file string) {
	w.valuesFile = file
	// Re-initialize stages with the new values file
	w.stages = make([]Stage, 0)
	w.setupStages()
}

func (w *ControlplaneWorkflow) setupStages() {
	cfg := w.Config()
	chartPath := w.getChartPath()

	// Use the set valuesFile or fall back to default
	valuesFile := w.valuesFile
	if valuesFile == "" {
		valuesFile = w.getValuesFile()
	}

	// Stage 1: Install control plane components
	w.AddStage(stage.NewHelmStage(
		"controlplane-apps",
		"primus-lens-controlplane",
		chartPath,
		stage.WithValuesFile(valuesFile),
		stage.WithTimeout(10*time.Minute),
		stage.WithWait(false),
		stage.WithNamespace(cfg.Global.Namespace),
	))

	// Stage 2: Wait for control plane components
	w.AddStage(stage.NewWaitStage(
		"wait-controlplane",
		[]stage.WaitCondition{
			{
				Kind:          "Deployment",
				LabelSelector: "app.kubernetes.io/component=api",
				Condition:     "Available",
				Timeout:       5 * time.Minute,
			},
			{
				Kind:          "Deployment",
				LabelSelector: "app.kubernetes.io/component=adapter",
				Condition:     "Available",
				Timeout:       5 * time.Minute,
			},
		},
		stage.WithWaitTimeout(10*time.Minute),
	))
}

func (w *ControlplaneWorkflow) getChartPath() string {
	return filepath.Join("..", "..", "charts", "primus-lens-apps-control-plane")
}

func (w *ControlplaneWorkflow) getValuesFile() string {
	return filepath.Join("..", "..", "charts", "primus-lens-apps-control-plane", "values.yaml")
}
