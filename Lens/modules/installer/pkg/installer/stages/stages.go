// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package stages provides individual installation stages for the Primus-Lens dataplane.
// Each stage is responsible for a single component and implements the StageV2 interface.
package stages

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/installer/pkg/installer"
)

// GetInfrastructureStages returns the stages for infrastructure installation.
// This includes operators and storage components.
func GetInfrastructureStages(helmClient *installer.HelmClient) []installer.StageV2 {
	stages := []installer.StageV2{
		// Phase 2: Operators
		NewPGOOperatorStage(helmClient),
		NewVMOperatorStage(helmClient),
		NewOpenSearchOperatorStage(helmClient),
		NewGrafanaOperatorStage(helmClient),
		NewFluentOperatorStage(helmClient),
		NewKubeStateMetricsStage(helmClient),

		// Phase 3: Infrastructure CRs
		NewPostgresStage(helmClient),
		NewVictoriaMetricsStage(helmClient),
		NewOpenSearchStage(helmClient),

		// Phase 4: Database Setup
		NewDatabaseInitStage(helmClient),
		NewDatabaseMigrationStage(helmClient),

		// Storage secret for apps
		NewStorageSecretStage(),
	}

	return stages
}

// GetAppsStages returns the stages for application deployment only.
// This assumes infrastructure is already set up.
func GetAppsStages(helmClient *installer.HelmClient) []installer.StageV2 {
	return []installer.StageV2{
		NewApplicationsStage(helmClient),
	}
}

// GetFullStages returns all stages for a complete installation.
func GetFullStages(helmClient *installer.HelmClient) []installer.StageV2 {
	stages := GetInfrastructureStages(helmClient)
	stages = append(stages, GetAppsStages(helmClient)...)
	return stages
}

// GetStagesByScope returns stages based on install scope.
func GetStagesByScope(scope string, helmClient *installer.HelmClient) []installer.StageV2 {
	switch scope {
	case installer.InstallScopeInfrastructure:
		return GetInfrastructureStages(helmClient)
	case installer.InstallScopeApps:
		return GetAppsStages(helmClient)
	default:
		return GetFullStages(helmClient)
	}
}

// StageNameMapping maps old stage names to new stage names for backward compatibility.
// This is used when resuming from a previous installation.
var StageNameMapping = map[string]string{
	// Old stage names -> New stage names
	"operators":           "operator-pgo",          // Start from first operator
	"wait_operators":      "infra-postgres",        // Skip directly to infrastructure
	"infrastructure":      "infra-postgres",        // Start from postgres
	"wait_infrastructure": "database-init",         // Skip to database setup
	"init":                "database-init",         // Same
	"database_migration":  "database-migration",    // Same
	"storage_secret":      "storage-secret",        // Same
	"applications":        "applications",          // Same
	"wait_applications":   "applications",          // Applications handles its own wait
}

// GetMappedStageName returns the new stage name for an old stage name.
func GetMappedStageName(oldName string) string {
	if newName, ok := StageNameMapping[oldName]; ok {
		return newName
	}
	return oldName
}

// FindStageIndex finds the index of a stage by name in the given stage list.
// Returns -1 if not found.
func FindStageIndex(stages []installer.StageV2, stageName string) int {
	for i, stage := range stages {
		if stage.Name() == stageName {
			return i
		}
	}
	return -1
}
