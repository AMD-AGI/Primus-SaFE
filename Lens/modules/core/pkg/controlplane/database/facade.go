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
	// Skills Repository
	Skill             SkillFacadeInterface
	SkillVersion      SkillVersionFacadeInterface
	SkillEmbedding    SkillEmbeddingFacadeInterface
	SkillExecution    SkillExecutionFacadeInterface
	SkillFeedback     SkillFeedbackFacadeInterface
	SkillQualityStats SkillQualityStatsFacadeInterface
	// Skillset
	Skillset      SkillsetFacadeInterface
	SkillsetSkill SkillsetSkillFacadeInterface
}

// NewControlPlaneFacade creates a new ControlPlaneFacade instance
func NewControlPlaneFacade(db *gorm.DB) *ControlPlaneFacade {
	return &ControlPlaneFacade{
		// Skills Repository
		Skill:             NewSkillFacade(db),
		SkillVersion:      NewSkillVersionFacade(db),
		SkillEmbedding:    NewSkillEmbeddingFacade(db),
		SkillExecution:    NewSkillExecutionFacade(db),
		SkillFeedback:     NewSkillFeedbackFacade(db),
		SkillQualityStats: NewSkillQualityStatsFacade(db),
		// Skillset
		Skillset:      NewSkillsetFacade(db),
		SkillsetSkill: NewSkillsetSkillFacade(db),
	}
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

// GetSkillset returns the Skillset Facade interface
func (f *ControlPlaneFacade) GetSkillset() SkillsetFacadeInterface {
	return f.Skillset
}

// GetSkillsetSkill returns the SkillsetSkill Facade interface
func (f *ControlPlaneFacade) GetSkillsetSkill() SkillsetSkillFacadeInterface {
	return f.SkillsetSkill
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
