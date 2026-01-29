// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/dal"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"
	"gorm.io/gorm"
)

// BaseFacade is the base structure for all Facades, providing DB access capability
type BaseFacade struct {
	clusterName string // Empty string means using the current cluster
}

// getDB retrieves the corresponding database connection based on clusterName
func (f *BaseFacade) getDB() *gorm.DB {
	clusterName := f.clusterName

	// If clusterName is empty, try to get the current cluster name from ClusterManager
	if clusterName == "" {
		cm := clientsets.GetClusterManager()
		if cm != nil {
			clusterName = cm.GetCurrentClusterName()
		}
	}

	// If still empty or "local", try to get from ClusterManager's current cluster
	if clusterName == "" || clusterName == "local" {
		cm := clientsets.GetClusterManager()
		if cm != nil {
			currentCluster := cm.GetCurrentClusterClients()
			if currentCluster != nil && currentCluster.StorageClientSet != nil && currentCluster.StorageClientSet.DB != nil {
				return currentCluster.StorageClientSet.DB
			}
		}
		// Fallback to sql.GetDefaultDB() for backward compatibility
		db := sql.GetDefaultDB()
		if db != nil {
			return db
		}
		// Last resort: try to get from sql pool with current cluster name
		if clusterName != "" {
			db = sql.GetDB(clusterName)
			if db != nil {
				return db
			}
		}
		return nil
	}

	// Get the database of the specified cluster through ClusterManager
	cm := clientsets.GetClusterManager()
	clientSet, err := cm.GetClientSetByClusterName(clusterName)
	if err != nil {
		log.Errorf("getDB: error getting client set by cluster name '%s': %v", clusterName, err)
		// If retrieval fails, try sql pool directly
		db := sql.GetDB(clusterName)
		if db != nil {
			return db
		}
		// Fallback to default database
		return sql.GetDefaultDB()
	}

	if clientSet.StorageClientSet == nil || clientSet.StorageClientSet.DB == nil {
		log.Errorf("getDB: cluster '%s' has no Storage configuration", clusterName)
		// Try sql pool directly
		db := sql.GetDB(clusterName)
		if db != nil {
			return db
		}
		// Fallback to default database
		return sql.GetDefaultDB()
	}
	return clientSet.StorageClientSet.DB
}

// getDAL retrieves the DAL instance
func (f *BaseFacade) getDAL() *dal.Query {
	db := f.getDB()
	if db == nil {
		log.Errorf("getDAL: database connection is nil for cluster '%s'", f.clusterName)
		return nil
	}
	query := dal.Use(db)
	return query
}

// withCluster returns a new Facade instance using the specified cluster
func (f *BaseFacade) withCluster(clusterName string) BaseFacade {
	return BaseFacade{
		clusterName: clusterName,
	}
}
