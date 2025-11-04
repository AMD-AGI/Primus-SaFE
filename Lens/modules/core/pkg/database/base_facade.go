package database

import (
	"runtime/debug"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/dal"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/sql"
	"gorm.io/gorm"
)

// BaseFacade is the base structure for all Facades, providing DB access capability
type BaseFacade struct {
	clusterName string // Empty string means using the current cluster
}

// getDB retrieves the corresponding database connection based on clusterName
func (f *BaseFacade) getDB() *gorm.DB {
	defer func() {
		log.Infof("getDB: clusterName: %s, call stack:\n%s", f.clusterName, string(debug.Stack()))
	}()
	if f.clusterName == "" {
		log.Infof("getDB: using default database")
		// Use the default database of the current cluster
		return sql.GetDefaultDB()
	}

	// Get the database of the specified cluster through ClusterManager
	cm := clientsets.GetClusterManager()
	clientSet, err := cm.GetClientSetByClusterName(f.clusterName)
	if err != nil {
		log.Errorf("getDB: error getting client set by cluster name: %v", err)
		// If retrieval fails, return the default database
		return sql.GetDefaultDB()
	}

	if clientSet.StorageClientSet == nil {
		log.Errorf("getDB: cluster has no Storage configuration")
		// If the cluster has no Storage configuration, return the default database
		return sql.GetDefaultDB()
	}
	log.Infof("getDB: client cluster name: %s, database address: %p", clientSet.ClusterName, clientSet.StorageClientSet.DB)
	db := clientSet.StorageClientSet.DB
	return db
}

// getDAL retrieves the DAL instance
func (f *BaseFacade) getDAL() *dal.Query {
	return dal.Use(f.getDB())
}

// withCluster returns a new Facade instance using the specified cluster
func (f *BaseFacade) withCluster(clusterName string) BaseFacade {
	return BaseFacade{
		clusterName: clusterName,
	}
}
