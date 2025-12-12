package workflow

import (
	"path/filepath"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/stage"
)

func init() {
	RegisterWorkflow("dataplane", NewDataplaneWorkflow)
}

// DataplaneWorkflow implements the dataplane installation workflow
type DataplaneWorkflow struct {
	*BaseWorkflow
	chartsDir  string
	valuesFile string
}

// NewDataplaneWorkflow creates a new dataplane workflow
func NewDataplaneWorkflow(cfg *config.Config) (Workflow, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	wf := &DataplaneWorkflow{
		BaseWorkflow: NewBaseWorkflow("dataplane", cfg),
		chartsDir:    filepath.Join("..", "..", "charts"),
		valuesFile:   "",
	}

	wf.setupStages()
	return wf, nil
}

// SetChartsDir sets the charts directory path and re-initializes stages
func (w *DataplaneWorkflow) SetChartsDir(dir string) {
	w.chartsDir = dir
	// Re-initialize stages with the new charts directory
	w.stages = make([]Stage, 0)
	w.setupStages()
}

// SetValuesFile sets the values file path and re-initializes stages
func (w *DataplaneWorkflow) SetValuesFile(file string) {
	w.valuesFile = file
	// Re-initialize stages with the new values file
	w.stages = make([]Stage, 0)
	w.setupStages()
}

func (w *DataplaneWorkflow) setupStages() {
	cfg := w.Config()

	// =========================================================================
	// Stage 1: Install Operators
	// Deploys all required Kubernetes operators (VM, PGO, OpenSearch, Grafana)
	// =========================================================================
	operatorsOpts := []stage.HelmStageOption{
		stage.WithTimeout(15 * time.Minute),
		stage.WithWait(false),
		stage.WithNamespace(cfg.Global.Namespace),
		stage.WithUpdateDeps(true), // Update helm dependencies before install
	}
	if w.valuesFile != "" {
		operatorsOpts = append(operatorsOpts, stage.WithValuesFile(w.valuesFile))
	}

	w.AddStage(stage.NewHelmStage(
		"install-operators",
		"plo", // Very short name (8 chars max) to avoid 63-char limit in generated resource names
		filepath.Join(w.chartsDir, "primus-lens-operators"),
		operatorsOpts...,
	))

	// =========================================================================
	// Stage 2: Wait for Operators to be ready
	// =========================================================================
	w.AddStage(stage.NewWaitStage(
		"wait-operators",
		w.buildOperatorConditions(),
		stage.WithWaitTimeout(10*time.Minute),
	))

	// =========================================================================
	// Stage 3: Install Infrastructure CRs
	// Creates PostgreSQL, OpenSearch, and VictoriaMetrics clusters
	// =========================================================================
	infraOpts := []stage.HelmStageOption{
		stage.WithTimeout(5 * time.Minute),
		stage.WithWait(false),
		stage.WithNamespace(cfg.Global.Namespace),
	}
	if w.valuesFile != "" {
		infraOpts = append(infraOpts, stage.WithValuesFile(w.valuesFile))
	}

	w.AddStage(stage.NewHelmStage(
		"install-infrastructure",
		"primus-lens-infrastructure",
		filepath.Join(w.chartsDir, "primus-lens-infrastructure"),
		infraOpts...,
	))

	// =========================================================================
	// Stage 4: Wait for Infrastructure to be ready
	// =========================================================================
	infraConditions := w.buildInfraConditions()
	if len(infraConditions) > 0 {
		w.AddStage(stage.NewWaitStage(
			"wait-infrastructure",
			infraConditions,
			stage.WithWaitTimeout(15*time.Minute),
		))
	}

	// =========================================================================
	// Stage 5: Run Init Jobs (Database initialization)
	// =========================================================================
	if cfg.Database.Enabled {
		initOpts := []stage.HelmStageOption{
			stage.WithTimeout(10 * time.Minute),
			stage.WithWait(true), // Wait for job to complete
			stage.WithNamespace(cfg.Global.Namespace),
		}
		if w.valuesFile != "" {
			initOpts = append(initOpts, stage.WithValuesFile(w.valuesFile))
		}

		w.AddStage(stage.NewHelmStage(
			"run-init-jobs",
			"primus-lens-init",
			filepath.Join(w.chartsDir, "primus-lens-init"),
			initOpts...,
		))
	}

	// =========================================================================
	// Stage 6: Install Applications
	// =========================================================================
	if cfg.Apps.Enabled {
		appsOpts := []stage.HelmStageOption{
			stage.WithTimeout(10 * time.Minute),
			stage.WithWait(false),
			stage.WithNamespace(cfg.Global.Namespace),
		}
		if w.valuesFile != "" {
			appsOpts = append(appsOpts, stage.WithValuesFile(w.valuesFile))
		}

		w.AddStage(stage.NewHelmStage(
			"install-applications",
			"primus-lens-apps",
			filepath.Join(w.chartsDir, "primus-lens-apps-dataplane"),
			appsOpts...,
		))

		// =========================================================================
		// Stage 7: Wait for Applications to be ready
		// =========================================================================
		w.AddStage(stage.NewWaitStage(
			"wait-applications",
			w.buildAppConditions(),
			stage.WithWaitTimeout(10*time.Minute),
		))
	}
}

// buildOperatorConditions creates wait conditions for operators
func (w *DataplaneWorkflow) buildOperatorConditions() []stage.WaitCondition {
	cfg := w.Config()
	conditions := []stage.WaitCondition{}

	// VictoriaMetrics Operator
	// Labels: app.kubernetes.io/instance=plo, app.kubernetes.io/name=vm-operator
	if cfg.VM.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Deployment",
			LabelSelector: "app.kubernetes.io/instance=plo,app.kubernetes.io/name=vm-operator",
			Condition:     "Available",
			Timeout:       5 * time.Minute,
		})
	}

	// Postgres Operator (PGO)
	// Labels: postgres-operator.crunchydata.com/control-plane=pgo
	if cfg.Database.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Deployment",
			LabelSelector: "postgres-operator.crunchydata.com/control-plane=pgo",
			Condition:     "Available",
			Timeout:       5 * time.Minute,
		})
	}

	// OpenSearch Operator
	// Labels: control-plane=controller-manager
	if cfg.OpenSearch.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Deployment",
			LabelSelector: "control-plane=controller-manager",
			Condition:     "Available",
			Timeout:       5 * time.Minute,
		})
	}

	// Grafana Operator
	// Labels: app.kubernetes.io/name=grafana-operator, app.kubernetes.io/part-of=grafana-operator
	if cfg.Grafana.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Deployment",
			LabelSelector: "app.kubernetes.io/name=grafana-operator,app.kubernetes.io/part-of=grafana-operator",
			Condition:     "Available",
			Timeout:       5 * time.Minute,
		})
	}

	// Fluent Operator
	// Labels: app.kubernetes.io/component=operator, app.kubernetes.io/name=fluent-operator
	if cfg.Logging.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Deployment",
			LabelSelector: "app.kubernetes.io/component=operator,app.kubernetes.io/name=fluent-operator",
			Condition:     "Available",
			Timeout:       5 * time.Minute,
		})
	}

	// Kube State Metrics
	// Labels: app.kubernetes.io/name=kube-state-metrics, app.kubernetes.io/part-of=kube-state-metrics
	if cfg.Monitoring.KubeStateMetrics.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Deployment",
			LabelSelector: "app.kubernetes.io/name=kube-state-metrics,app.kubernetes.io/part-of=kube-state-metrics",
			Condition:     "Available",
			Timeout:       5 * time.Minute,
		})
	}

	return conditions
}

// buildInfraConditions creates wait conditions for infrastructure
func (w *DataplaneWorkflow) buildInfraConditions() []stage.WaitCondition {
	cfg := w.Config()
	conditions := []stage.WaitCondition{}

	if cfg.Database.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Pod",
			LabelSelector: "postgres-operator.crunchydata.com/cluster=primus-lens,postgres-operator.crunchydata.com/instance",
			Condition:     "Ready",
			Timeout:       10 * time.Minute,
		})
	}

	if cfg.OpenSearch.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Pod",
			LabelSelector: "opster.io/opensearch-cluster=" + cfg.OpenSearch.ClusterName,
			Condition:     "Ready",
			Timeout:       10 * time.Minute,
		})
	}

	if cfg.VM.Enabled {
		conditions = append(conditions, stage.WaitCondition{
			Kind:          "Pod",
			LabelSelector: "app.kubernetes.io/name=vmstorage,app.kubernetes.io/instance=primus-lens-vmcluster",
			Condition:     "Ready",
			Timeout:       10 * time.Minute,
		})
	}

	return conditions
}

// buildAppConditions creates wait conditions for applications
// Labels format: app=primus-lens-apps-<component>
// NOTE: web is part of control-plane, not data-plane
func (w *DataplaneWorkflow) buildAppConditions() []stage.WaitCondition {
	return []stage.WaitCondition{
		{
			Kind:          "Deployment",
			LabelSelector: "app=primus-lens-apps-telemetry-processor",
			Condition:     "Available",
			Timeout:       5 * time.Minute,
		},
	}
}
