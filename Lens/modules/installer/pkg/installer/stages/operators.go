// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stages

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/installer/pkg/installer"
)

// Operator chart paths (relative to /app/charts in the installer container)
// Uses new modular chart structure: charts/operators/<operator-name>/
const (
	ChartPGO              = "operators/pgo"
	ChartVMOperator       = "operators/victoria-metrics-operator"
	ChartOpenSearchOp     = "operators/opensearch-operator"
	ChartGrafanaOperator  = "operators/grafana-operator"
	ChartFluentOperator   = "operators/fluent-operator"
	ChartKubeStateMetrics = "operators/kube-state-metrics"
)

// Infrastructure chart paths
const (
	ChartPostgres         = "infrastructure/postgres"
	ChartVictoriaMetrics  = "infrastructure/victoriametrics"
	ChartOpenSearch       = "infrastructure/opensearch"
)

// NewPGOOperatorStage creates the PostgreSQL Operator (PGO) installation stage
func NewPGOOperatorStage(helmClient *installer.HelmClient) *OperatorStage {
	return NewOperatorStage(OperatorConfig{
		Name:                 "pgo",
		ChartName:            ChartPGO,
		ReleaseName:          "pgo",
		Namespace:            "postgres-operator",
		DetectionClusterRole: "pgo",
		DeploymentName:       "pgo",
		Values: map[string]interface{}{
			"singleNamespace": false,
		},
		Required: true,
	}, helmClient)
}

// NewVMOperatorStage creates the VictoriaMetrics Operator installation stage
func NewVMOperatorStage(helmClient *installer.HelmClient) *OperatorStage {
	return NewOperatorStage(OperatorConfig{
		Name:                 "victoriametrics",
		ChartName:            ChartVMOperator,
		ReleaseName:          "vm-operator",
		Namespace:            "vm-operator",
		DetectionClusterRole: "vm-operator-victoria-metrics-operator",
		DeploymentName:       "vm-operator-victoria-metrics-operator",
		Values:               map[string]interface{}{},
		Required:             true,
	}, helmClient)
}

// NewOpenSearchOperatorStage creates the OpenSearch Operator installation stage
func NewOpenSearchOperatorStage(helmClient *installer.HelmClient) *OperatorStage {
	return NewOperatorStage(OperatorConfig{
		Name:                 "opensearch",
		ChartName:            ChartOpenSearchOp,
		ReleaseName:          "opensearch-operator",
		Namespace:            "opensearch-operator",
		DetectionClusterRole: "opensearch-operator-manager-role",
		DeploymentName:       "opensearch-operator-controller-manager",
		Values:               map[string]interface{}{},
		Required:             false, // OpenSearch is optional
	}, helmClient)
}

// NewGrafanaOperatorStage creates the Grafana Operator installation stage
func NewGrafanaOperatorStage(helmClient *installer.HelmClient) *OperatorStage {
	return NewOperatorStage(OperatorConfig{
		Name:                 "grafana",
		ChartName:            ChartGrafanaOperator,
		ReleaseName:          "grafana-operator",
		Namespace:            "grafana-operator",
		DetectionClusterRole: "grafana-operator-manager-role",
		DeploymentName:       "grafana-operator-controller-manager",
		Values:               map[string]interface{}{},
		Required:             false, // Grafana is optional
	}, helmClient)
}

// NewFluentOperatorStage creates the Fluent Operator installation stage
func NewFluentOperatorStage(helmClient *installer.HelmClient) *OperatorStage {
	return NewOperatorStage(OperatorConfig{
		Name:                 "fluent",
		ChartName:            ChartFluentOperator,
		ReleaseName:          "fluent-operator",
		Namespace:            "fluent",
		DetectionClusterRole: "fluent-operator",
		DeploymentName:       "fluent-operator",
		Values:               map[string]interface{}{},
		Required:             false, // Fluent is optional
	}, helmClient)
}

// NewKubeStateMetricsStage creates the Kube State Metrics installation stage
func NewKubeStateMetricsStage(helmClient *installer.HelmClient) *OperatorStage {
	return NewOperatorStage(OperatorConfig{
		Name:                 "kube-state-metrics",
		ChartName:            ChartKubeStateMetrics,
		ReleaseName:          "kube-state-metrics",
		Namespace:            "kube-system",
		DetectionClusterRole: "kube-state-metrics",
		DeploymentName:       "kube-state-metrics",
		Values:               map[string]interface{}{},
		Required:             false, // KSM is optional
	}, helmClient)
}

// GetOperatorStages returns all operator stages in the correct order
func GetOperatorStages(helmClient *installer.HelmClient) []installer.StageV2 {
	return []installer.StageV2{
		NewPGOOperatorStage(helmClient),
		NewVMOperatorStage(helmClient),
		NewOpenSearchOperatorStage(helmClient),
		NewGrafanaOperatorStage(helmClient),
		NewFluentOperatorStage(helmClient),
		NewKubeStateMetricsStage(helmClient),
	}
}
