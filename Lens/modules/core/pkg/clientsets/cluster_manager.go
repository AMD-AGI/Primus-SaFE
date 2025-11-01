package clientsets

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
)

// ClusterClientSet contains all clients for a single cluster
type ClusterClientSet struct {
	ClusterName      string
	K8SClientSet     *K8SClientSet
	StorageClientSet *StorageClientSet
}

// ClusterManager manages clients for all clusters
type ClusterManager struct {
	mu sync.RWMutex

	// Client for the current (local) cluster
	currentCluster *ClusterClientSet

	// Map of all cluster clients (clusterName -> ClusterClientSet)
	// In multi-cluster mode, this includes the current cluster and all remote clusters
	clusters map[string]*ClusterClientSet

	// Whether in multi-cluster mode
	multiCluster bool
}

var (
	globalClusterManager *ClusterManager
	clusterManagerOnce   sync.Once
)

// InitClusterManager initializes the cluster manager and all client sets
// This is the main entry point for initializing all clients
func InitClusterManager(ctx context.Context, multiCluster bool) error {
	var initErr error
	clusterManagerOnce.Do(func() {
		globalClusterManager = &ClusterManager{
			clusters:     make(map[string]*ClusterClientSet),
			multiCluster: multiCluster,
		}
		initErr = globalClusterManager.initialize(ctx)
	})
	return initErr
}

// GetClusterManager returns the global cluster manager instance
func GetClusterManager() *ClusterManager {
	if globalClusterManager == nil {
		panic("cluster manager not initialized, please call InitClusterManager first")
	}
	return globalClusterManager
}

// initialize initializes the cluster manager
func (cm *ClusterManager) initialize(ctx context.Context) error {
	// Initialize K8S client sets first
	if err := cm.initializeK8SClients(ctx); err != nil {
		return err
	}

	// Initialize Storage client sets
	if err := cm.initializeStorageClients(ctx); err != nil {
		return err
	}

	// Initialize current cluster
	if err := cm.initializeCurrentCluster(); err != nil {
		return err
	}

	// If in multi-cluster mode, initialize all clusters
	if cm.multiCluster {
		if err := cm.loadAllClusters(ctx); err != nil {
			log.Warnf("Failed to load multi-cluster clients: %v", err)
			// Don't return error as multi-cluster config may not be ready yet
		}

		// Start periodic sync
		go cm.startPeriodicSync(ctx)
	}

	log.Info("Cluster manager initialized successfully")
	return nil
}

// initializeK8SClients initializes K8S clients for current and multi-cluster
func (cm *ClusterManager) initializeK8SClients(ctx context.Context) error {
	// Initialize current cluster K8S client
	if err := initCurrentClusterK8SClientSet(ctx); err != nil {
		return err
	}

	// If in multi-cluster mode, initialize multi-cluster K8S clients
	if cm.multiCluster {
		if err := loadMultiClusterK8SClientSet(ctx); err != nil {
			log.Warnf("Failed to load multi-cluster K8S clients: %v", err)
			// Don't return error as multi-cluster config may not be ready yet
		}
		// Start periodic sync for K8S clients
		go doLoadMultiClusterK8SClientSet(ctx)
	}

	log.Info("K8S clients initialized successfully")
	return nil
}

// initializeStorageClients initializes Storage clients for current and multi-cluster
func (cm *ClusterManager) initializeStorageClients(ctx context.Context) error {
	var err error
	if !cm.multiCluster {
		err = loadCurrentClusterStorageClients(ctx)
	} else {
		err = loadMultiClusterStorageClients(ctx)
		if err != nil {
			log.Warnf("Failed to load multi-cluster storage clients: %v", err)
			// Don't return error as multi-cluster config may not be ready yet
			err = nil
		}
		// Start periodic sync for storage clients
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := loadMultiClusterStorageClients(ctx); err != nil {
						log.Errorf("Failed to reload multi-cluster storage clients: %v", err)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if err != nil {
		return err
	}

	log.Info("Storage clients initialized successfully")
	return nil
}

// initializeCurrentCluster initializes the current cluster's clients
func (cm *ClusterManager) initializeCurrentCluster() error {
	// Get clients from already initialized global variables
	k8sClient := getCurrentClusterK8SClientSet()
	storageClient := getCurrentClusterStorageClientSet()

	// Try to get cluster name from environment variable or config
	clusterName := getCurrentClusterName()

	cm.currentCluster = &ClusterClientSet{
		ClusterName:      clusterName,
		K8SClientSet:     k8sClient,
		StorageClientSet: storageClient,
	}

	// Also add current cluster to clusters map
	cm.clusters[clusterName] = cm.currentCluster

	log.Infof("Initialized current cluster: %s", clusterName)
	return nil
}

// loadAllClusters loads clients for all clusters (multi-cluster mode)
func (cm *ClusterManager) loadAllClusters(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Get all K8S clients
	k8sClients := getAllClusterK8SClients()

	// Create new cluster map
	newClusters := make(map[string]*ClusterClientSet)

	// Keep current cluster
	if cm.currentCluster != nil {
		newClusters[cm.currentCluster.ClusterName] = cm.currentCluster
	}

	// Create ClusterClientSet for each remote cluster
	for clusterName, k8sClient := range k8sClients {
		// Skip if it's the current cluster (already added)
		if clusterName == cm.currentCluster.ClusterName {
			continue
		}

		// Try to get storage client for this cluster
		storageClient, err := getStorageClientSetByClusterName(clusterName)
		if err != nil {
			log.Warnf("Failed to get storage client for cluster %s: %v", clusterName, err)
			// Create cluster object even without storage client (storage config may not be ready yet)
			storageClient = nil
		}

		newClusters[clusterName] = &ClusterClientSet{
			ClusterName:      clusterName,
			K8SClientSet:     k8sClient,
			StorageClientSet: storageClient,
		}

		log.Infof("Loaded cluster: %s", clusterName)
	}

	cm.clusters = newClusters
	log.Infof("Total clusters loaded: %d", len(cm.clusters))
	return nil
}

// startPeriodicSync starts periodic synchronization (multi-cluster mode)
func (cm *ClusterManager) startPeriodicSync(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := cm.loadAllClusters(ctx); err != nil {
				log.Errorf("Failed to sync clusters: %v", err)
			} else {
				log.Debug("Clusters synced successfully")
			}
		case <-ctx.Done():
			log.Info("Stopping cluster manager periodic sync")
			return
		}
	}
}

// GetCurrentClusterClients returns the current cluster's clients (commonly used in data plane)
func (cm *ClusterManager) GetCurrentClusterClients() *ClusterClientSet {
	if cm.currentCluster == nil {
		panic("current cluster not initialized")
	}
	return cm.currentCluster
}

// GetClientSetByClusterName returns clients for a specific cluster by name (commonly used in control plane)
func (cm *ClusterManager) GetClientSetByClusterName(clusterName string) (*ClusterClientSet, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	clientSet, exists := cm.clusters[clusterName]
	if !exists {
		return nil, errors.NewError().
			WithCode(errors.RequestDataNotExisted).
			WithMessagef("ClientSet for cluster %s not found", clusterName)
	}

	return clientSet, nil
}

// ListAllClientSets returns all cluster clients (commonly used in control plane)
func (cm *ClusterManager) ListAllClientSets() map[string]*ClusterClientSet {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Return a copy to avoid concurrent modifications
	result := make(map[string]*ClusterClientSet, len(cm.clusters))
	for name, clientSet := range cm.clusters {
		result[name] = clientSet
	}

	return result
}

// GetClusterNames returns a list of all cluster names
func (cm *ClusterManager) GetClusterNames() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	names := make([]string, 0, len(cm.clusters))
	for name := range cm.clusters {
		names = append(names, name)
	}

	return names
}

// GetClusterCount returns the number of clusters
func (cm *ClusterManager) GetClusterCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return len(cm.clusters)
}

// IsMultiCluster returns whether in multi-cluster mode
func (cm *ClusterManager) IsMultiCluster() bool {
	return cm.multiCluster
}

// GetCurrentClusterName returns the current cluster name
func (cm *ClusterManager) GetCurrentClusterName() string {
	if cm.currentCluster == nil {
		return ""
	}
	return cm.currentCluster.ClusterName
}

// HasCluster checks if a cluster with the given name exists
func (cm *ClusterManager) HasCluster(clusterName string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	_, exists := cm.clusters[clusterName]
	return exists
}

// getCurrentClusterName gets the current cluster name from environment variables or other sources
func getCurrentClusterName() string {
	// First try to get from environment variable
	if name := os.Getenv("CLUSTER_NAME"); name != "" {
		return name
	}

	// Try to get from K8S config
	// This can be extended based on actual requirements

	// Default value
	return "default"
}
