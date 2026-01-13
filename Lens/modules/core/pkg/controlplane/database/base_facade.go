// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/dal"
	"gorm.io/gorm"
)

// BaseFacade is the base structure for all Control Plane Facades
// Unlike Data Plane BaseFacade, this doesn't support cluster switching
// as Control Plane always uses a single database
type BaseFacade struct{}

// getDB retrieves the Control Plane database connection
func (f *BaseFacade) getDB() *gorm.DB {
	return clientsets.GetClusterManager().GetControlPlaneDB()
}

// getDAL retrieves the DAL instance for Control Plane
func (f *BaseFacade) getDAL() *dal.Query {
	db := f.getDB()
	return dal.Use(db)
}
