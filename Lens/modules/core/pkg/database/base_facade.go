package database

import (
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
	log.Infof("getDB called: clusterName: %s", f.clusterName)

	if f.clusterName == "" {
		log.Infof("getDB: using default database (empty clusterName)")
		db := sql.GetDefaultDB()
		log.Infof("getDB: returning default DB: %p", db)
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
	log.Infof("getDB: successfully got client set for cluster '%s', database address: %+v",
		clientSet.ClusterName, clientSet.StorageClientSet.Config.Postgres)
	db := clientSet.StorageClientSet.DB
	log.Infof("getDB: returning cluster DB: %p for cluster '%s'", db, f.clusterName)
	return db
}

// getDAL retrieves the DAL instance
func (f *BaseFacade) getDAL() *dal.Query {
	db := f.getDB()
	log.Infof("getDAL: creating DAL with DB: %p for cluster: %s", db, f.clusterName)
	query := dal.Use(db)
	log.Infof("getDAL: created Query: %p", query)
	return query
}

// withCluster returns a new Facade instance using the specified cluster
func (f *BaseFacade) withCluster(clusterName string) BaseFacade {
	return BaseFacade{
		clusterName: clusterName,
	}
}
