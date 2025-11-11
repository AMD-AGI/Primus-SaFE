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

	if f.clusterName == "" {
		db := sql.GetDefaultDB()
		return db
	}

	// Get the database of the specified cluster through ClusterManager
	cm := clientsets.GetClusterManager()
	clientSet, err := cm.GetClientSetByClusterName(f.clusterName)
	if err != nil {
		log.Errorf("getDB: error getting client set by cluster name '%s': %v", f.clusterName, err)
		// If retrieval fails, return the default database
		db := sql.GetDefaultDB()
		log.Errorf("getDB: falling back to default DB: %p", db)
		return db
	}

	if clientSet.StorageClientSet == nil {
		log.Errorf("getDB: cluster '%s' has no Storage configuration", f.clusterName)
		// If the cluster has no Storage configuration, return the default database
		db := sql.GetDefaultDB()
		log.Errorf("getDB: falling back to default DB: %p", db)
		return db
	}
	db := clientSet.StorageClientSet.DB
	return db
}

// getDAL retrieves the DAL instance
func (f *BaseFacade) getDAL() *dal.Query {
	db := f.getDB()
	query := dal.Use(db)
	return query
}

// withCluster returns a new Facade instance using the specified cluster
func (f *BaseFacade) withCluster(clusterName string) BaseFacade {
	return BaseFacade{
		clusterName: clusterName,
	}
}
