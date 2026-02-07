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
	// Tools (unified skills + mcp)
	Tool *ToolFacade
}

// NewControlPlaneFacade creates a new ControlPlaneFacade instance
func NewControlPlaneFacade(db *gorm.DB) *ControlPlaneFacade {
	return &ControlPlaneFacade{
		Tool: NewToolFacade(db),
	}
}

// GetTool returns the Tool Facade
func (f *ControlPlaneFacade) GetTool() *ToolFacade {
	return f.Tool
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
