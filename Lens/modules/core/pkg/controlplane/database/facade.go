// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Build trigger: 202601301210

package database

import (
	"sync"

	"gorm.io/gorm"
)

// ControlPlaneFacade is the unified entry point for control plane database operations
type ControlPlaneFacade struct {
	ClusterConfig            ClusterConfigFacadeInterface
	ClusterDeploymentHistory ClusterDeploymentHistoryFacadeInterface
	DataplaneInstallTask     DataplaneInstallTaskFacadeInterface
	ReleaseVersion           ReleaseVersionFacadeInterface
	ClusterReleaseConfig     ClusterReleaseConfigFacadeInterface
	ReleaseHistory           ReleaseHistoryFacadeInterface
	GpuUsageWeeklyReport     GpuUsageWeeklyReportFacadeInterface
	TraceLensSession         TraceLensSessionFacadeInterface
	ControlPlaneConfig       ControlPlaneConfigFacadeInterface
	// Skills Repository
	Skill             SkillFacadeInterface
	SkillVersion      SkillVersionFacadeInterface
	SkillEmbedding    SkillEmbeddingFacadeInterface
	SkillExecution    SkillExecutionFacadeInterface
	SkillFeedback     SkillFeedbackFacadeInterface
	SkillQualityStats SkillQualityStatsFacadeInterface
}

// NewControlPlaneFacade creates a new ControlPlaneFacade instance
func NewControlPlaneFacade(db *gorm.DB) *ControlPlaneFacade {
	return &ControlPlaneFacade{
		ClusterConfig:            NewClusterConfigFacade(db),
		ClusterDeploymentHistory: NewClusterDeploymentHistoryFacade(db),
		DataplaneInstallTask:     NewDataplaneInstallTaskFacade(db),
		ReleaseVersion:           NewReleaseVersionFacade(db),
		ClusterReleaseConfig:     NewClusterReleaseConfigFacade(db),
		ReleaseHistory:           NewReleaseHistoryFacade(db),
		GpuUsageWeeklyReport:     NewGpuUsageWeeklyReportFacade(db),
		TraceLensSession:         NewTraceLensSessionFacade(db),
		ControlPlaneConfig:       NewControlPlaneConfigFacade(db),
		// Skills Repository
		Skill:             NewSkillFacade(db),
		SkillVersion:      NewSkillVersionFacade(db),
		SkillEmbedding:    NewSkillEmbeddingFacade(db),
		SkillExecution:    NewSkillExecutionFacade(db),
		SkillFeedback:     NewSkillFeedbackFacade(db),
		SkillQualityStats: NewSkillQualityStatsFacade(db),
	}
}

// GetClusterConfig returns the ClusterConfig Facade interface
func (f *ControlPlaneFacade) GetClusterConfig() ClusterConfigFacadeInterface {
	return f.ClusterConfig
}

// GetClusterDeploymentHistory returns the ClusterDeploymentHistory Facade interface
func (f *ControlPlaneFacade) GetClusterDeploymentHistory() ClusterDeploymentHistoryFacadeInterface {
	return f.ClusterDeploymentHistory
}

// GetDataplaneInstallTask returns the DataplaneInstallTask Facade interface
func (f *ControlPlaneFacade) GetDataplaneInstallTask() DataplaneInstallTaskFacadeInterface {
	return f.DataplaneInstallTask
}

// GetReleaseVersion returns the ReleaseVersion Facade interface
func (f *ControlPlaneFacade) GetReleaseVersion() ReleaseVersionFacadeInterface {
	return f.ReleaseVersion
}

// GetClusterReleaseConfig returns the ClusterReleaseConfig Facade interface
func (f *ControlPlaneFacade) GetClusterReleaseConfig() ClusterReleaseConfigFacadeInterface {
	return f.ClusterReleaseConfig
}

// GetReleaseHistory returns the ReleaseHistory Facade interface
func (f *ControlPlaneFacade) GetReleaseHistory() ReleaseHistoryFacadeInterface {
	return f.ReleaseHistory
}

// GetGpuUsageWeeklyReport returns the GpuUsageWeeklyReport Facade interface
func (f *ControlPlaneFacade) GetGpuUsageWeeklyReport() GpuUsageWeeklyReportFacadeInterface {
	return f.GpuUsageWeeklyReport
}

// GetTraceLensSession returns the TraceLensSession Facade interface
func (f *ControlPlaneFacade) GetTraceLensSession() TraceLensSessionFacadeInterface {
	return f.TraceLensSession
}

// GetControlPlaneConfig returns the ControlPlaneConfig Facade interface
func (f *ControlPlaneFacade) GetControlPlaneConfig() ControlPlaneConfigFacadeInterface {
	return f.ControlPlaneConfig
}

// GetSkill returns the Skill Facade interface
func (f *ControlPlaneFacade) GetSkill() SkillFacadeInterface {
	return f.Skill
}

// GetSkillVersion returns the SkillVersion Facade interface
func (f *ControlPlaneFacade) GetSkillVersion() SkillVersionFacadeInterface {
	return f.SkillVersion
}

// GetSkillEmbedding returns the SkillEmbedding Facade interface
func (f *ControlPlaneFacade) GetSkillEmbedding() SkillEmbeddingFacadeInterface {
	return f.SkillEmbedding
}

// GetSkillExecution returns the SkillExecution Facade interface
func (f *ControlPlaneFacade) GetSkillExecution() SkillExecutionFacadeInterface {
	return f.SkillExecution
}

// GetSkillFeedback returns the SkillFeedback Facade interface
func (f *ControlPlaneFacade) GetSkillFeedback() SkillFeedbackFacadeInterface {
	return f.SkillFeedback
}

// GetSkillQualityStats returns the SkillQualityStats Facade interface
func (f *ControlPlaneFacade) GetSkillQualityStats() SkillQualityStatsFacadeInterface {
	return f.SkillQualityStats
}

// Global control plane facade instance
var (
	controlPlaneFacade     *ControlPlaneFacade
	controlPlaneFacadeOnce sync.Once
	controlPlaneDB         *gorm.DB
)

// InitControlPlaneFacade initializes the global control plane facade
func InitControlPlaneFacade(db *gorm.DB) {
	controlPlaneFacadeOnce.Do(func() {
		controlPlaneDB = db
		controlPlaneFacade = NewControlPlaneFacade(db)
	})
}

// GetControlPlaneFacade returns the global control plane facade instance
func GetControlPlaneFacade() *ControlPlaneFacade {
	if controlPlaneFacade == nil {
		panic("control plane facade not initialized, please call InitControlPlaneFacade first")
	}
	return controlPlaneFacade
}

// GetControlPlaneDB returns the control plane database connection
func GetControlPlaneDB() *gorm.DB {
	if controlPlaneDB == nil {
		panic("control plane database not initialized")
	}
	return controlPlaneDB
}
