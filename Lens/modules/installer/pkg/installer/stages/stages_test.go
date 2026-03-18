// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stages

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/installer/pkg/installer"
)

func TestGetMappedStageName(t *testing.T) {
	tests := []struct {
		oldName string
		want   string
	}{
		{"operators", "operator-pgo"},
		{"wait_operators", "infra-postgres"},
		{"infrastructure", "infra-postgres"},
		{"init", "database-init"},
		{"database_migration", "database-migration"},
		{"storage_secret", "storage-secret"},
		{"applications", "applications"},
		{"wait_applications", "applications"},
		{"pending", "pending"},
		{"unknown_stage", "unknown_stage"},
		{"", ""},
		{"database-migration", "database-migration"},
	}
	for _, tt := range tests {
		t.Run(tt.oldName, func(t *testing.T) {
			got := GetMappedStageName(tt.oldName)
			if got != tt.want {
				t.Errorf("GetMappedStageName(%q) = %q, want %q", tt.oldName, got, tt.want)
			}
		})
	}
}

func TestFindStageIndex(t *testing.T) {
	helmClient := installer.NewHelmClient()
	stages := GetFullStages(helmClient)
	if len(stages) == 0 {
		t.Fatal("GetFullStages returned empty")
	}
	got := FindStageIndex(stages, "applications")
	if got < 0 || got >= len(stages) {
		t.Errorf("FindStageIndex(applications) = %d, want in [0,%d)", got, len(stages))
	}
	if stages[got].Name() != "applications" {
		t.Errorf("stages[%d].Name() = %q, want applications", got, stages[got].Name())
	}
	got = FindStageIndex(stages, "no-such-stage")
	if got != -1 {
		t.Errorf("FindStageIndex(no-such-stage) = %d, want -1", got)
	}
}

func TestGetStagesByScope_ReturnsCorrectCount(t *testing.T) {
	helmClient := installer.NewHelmClient()

	infra := GetStagesByScope(installer.InstallScopeInfrastructure, helmClient)
	if len(infra) == 0 {
		t.Error("GetStagesByScope(infrastructure) returned no stages")
	}
	if len(infra) != 12 {
		t.Errorf("GetStagesByScope(infrastructure) len = %d, want 12", len(infra))
	}

	apps := GetStagesByScope(installer.InstallScopeApps, helmClient)
	if len(apps) != 1 {
		t.Errorf("GetStagesByScope(apps) len = %d, want 1", len(apps))
	}
	if len(apps) > 0 && apps[0].Name() != "applications" {
		t.Errorf("GetStagesByScope(apps)[0].Name() = %q, want applications", apps[0].Name())
	}

	full := GetStagesByScope(installer.InstallScopeFull, helmClient)
	if len(full) != len(infra)+len(apps) {
		t.Errorf("GetStagesByScope(full) len = %d, want %d", len(full), len(infra)+len(apps))
	}
}

func TestNewStageListProvider(t *testing.T) {
	helmClient := installer.NewHelmClient()
	p := NewStageListProvider(helmClient)
	if p == nil {
		t.Fatal("NewStageListProvider returned nil")
	}
	if got := p.GetMappedStageName("operators"); got != "operator-pgo" {
		t.Errorf("GetMappedStageName(operators) = %q, want operator-pgo", got)
	}
	if stages := p.GetAppsStages(); len(stages) != 1 {
		t.Errorf("GetAppsStages() len = %d, want 1", len(stages))
	}
	if stages := p.GetStagesByScope(installer.InstallScopeApps); len(stages) != 1 {
		t.Errorf("GetStagesByScope(apps) len = %d, want 1", len(stages))
	}
}
