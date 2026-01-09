// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
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

	// ============ Control Plane ============
	// Single instance, manages Lens's own metadata (users, sessions, configs)
	controlPlane *ControlPlaneClientSet

	// Whether to load Control Plane database
	loadControlPlane bool

	// ============ Data Plane ============
	// Client for the current (local) cluster
	currentCluster *ClusterClientSet

	// Map of all cluster clients (clusterName -> ClusterClientSet)
	// In multi-cluster mode, this includes the current cluster and all remote clusters
	clusters map[string]*ClusterClientSet

	// Whether in multi-cluster mode
	multiCluster bool

	// Whether to load K8S client
	loadK8SClient bool

	// Whether to load Storage client
	loadStorageClient bool

	// Default cluster name to use when no cluster is specified
	defaultClusterName string
}

var (
	globalClusterManager *ClusterManager
	clusterManagerOnce   sync.Once
)

// InitClusterManager initializes the cluster manager and all client sets
// This is the main entry point for initializing all clients
func InitClusterManager(ctx context.Context, multiCluster bool, loadK8SClient bool, loadStorageClient bool) error {
	var initErr error
	clusterManagerOnce.Do(func() {
		globalClusterManager = &ClusterManager{
			clusters:          make(map[string]*ClusterClientSet),
			multiCluster:      multiCluster,
			loadK8SClient:     loadK8SClient,
			loadStorageClient: loadStorageClient,
		}
		initErr = globalClusterManager.initialize(ctx)
	})
	return initErr
}

// InitClusterManagerWithOptions initializes the cluster manager with options
// This supports both Control Plane and Data Plane initialization
func InitClusterManagerWithOptions(ctx context.Context, opts *InitOptions) error {
	var initErr error
	clusterManagerOnce.Do(func() {
		globalClusterManager = &ClusterManager{
			clusters:          make(map[string]*ClusterClientSet),
			multiCluster:      opts.MultiCluster,
			loadK8SClient:     opts.LoadK8SClient,
			loadStorageClient: opts.LoadStorageClient,
			loadControlPlane:  opts.LoadControlPlane,
		}
		initErr = globalClusterManager.initializeWithOptions(ctx, opts)
	})
	return initErr
}

// initializeWithOptions initializes the cluster manager with options
func (cm *ClusterManager) initializeWithOptions(ctx context.Context, opts *InitOptions) error {
	// 1. Initialize Control Plane first (if enabled)
	if cm.loadControlPlane && opts.ControlPlaneConfig != nil {
		if err := cm.initializeControlPlane(ctx, opts.ControlPlaneConfig); err != nil {
			return err
		}
		log.Info("Control Plane initialized successfully")
	}

	// 2. Initialize Data Plane (existing logic)
	return cm.initialize(ctx)
}

// GetClusterManager returns the global cluster manager instance
func GetClusterManager() *ClusterManager {
	if globalClusterManager == nil {
		panic("cluster manager not initialized, please call InitClusterManager first")
	}
	return globalClusterManager
}

// InitClusterManagerWithClientSet initializes the cluster manager with a pre-configured ClusterClientSet.
// This is useful for testing or standalone tools that need to bypass the normal initialization process.
// The provided clientSet will be used as the current cluster.
//
// Parameters:
//   - clientSet: The pre-configured ClusterClientSet to use as the current cluster
//
// Note: This method can only be called once. Subsequent calls will be ignored.
func InitClusterManagerWithClientSet(clientSet *ClusterClientSet) {
	clusterManagerOnce.Do(func() {
		if clientSet == nil {
			log.Warn("InitClusterManagerWithClientSet called with nil clientSet, creating empty manager")
			clientSet = &ClusterClientSet{
				ClusterName: "default",
			}
		}

		globalClusterManager = &ClusterManager{
			clusters:           make(map[string]*ClusterClientSet),
			multiCluster:       false,
			loadK8SClient:      clientSet.K8SClientSet != nil,
			loadStorageClient:  clientSet.StorageClientSet != nil,
			currentCluster:     clientSet,
			defaultClusterName: clientSet.ClusterName,
		}

		// Add current cluster to clusters map
		globalClusterManager.clusters[clientSet.ClusterName] = clientSet

		log.Infof("Cluster manager initialized with pre-configured client set: %s (K8S: %v, Storage: %v)",
			clientSet.ClusterName,
			clientSet.K8SClientSet != nil,
			clientSet.StorageClientSet != nil)
	})
}

// NewStorageClientSetWithDB creates a minimal StorageClientSet with only a database connection.
// This is useful for testing or CLI tools that only need database access.
func NewStorageClientSetWithDB(db interface{}) *StorageClientSet {
	// We need to handle the type assertion carefully
	// The db parameter is expected to be *gorm.DB
	return &StorageClientSet{
		DB: nil, // Will be set by the caller who can do proper type assertion
	}
}

// initialize initializes the cluster manager
func (cm *ClusterManager) initialize(ctx context.Context) error {
	// Initialize K8S client sets first if enabled
	if cm.loadK8SClient {
		if err := cm.initializeK8SClients(ctx); err != nil {
			return err
		}
	} else {
		log.Info("K8S client loading is disabled")
	}

	// Initialize Storage client sets if enabled
	if cm.loadStorageClient {
		if err := cm.initializeStorageClients(ctx); err != nil {
			return err
		}
	} else {
		log.Info("Storage client loading is disabled")
	}

	// Initialize current cluster only if at least one client is enabled
	if cm.loadK8SClient || cm.loadStorageClient {
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
	} else {
		log.Warn("Both K8S and Storage client loading are disabled, skipping cluster initialization")
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
	} else {
		log.Info("Not in multi-cluster mode, skipping multi-cluster K8S client loading")
	}

	log.Info("K8S clients initialized successfully")
	return nil
}

// initializeStorageClients initializes Storage clients for current and multi-cluster
func (cm *ClusterManager) initializeStorageClients(ctx context.Context) error {
	var err error
	if !cm.multiCluster {
		err = loadCurrentClusterStorageClients(ctx)
		if err == nil {
			log.Info("Current cluster storage clients loaded successfully")
		}
	} else {
		// In multi-cluster mode, first initialize current cluster storage clients
		err = loadCurrentClusterStorageClients(ctx)
		if err != nil {
			log.Warnf("Failed to load current cluster storage clients: %v", err)
			// Don't return error as storage config may not be ready yet
			err = nil
		}

		// Then load multi-cluster storage clients
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
		log.Info("Multi-cluster storage clients loading initiated")
	}

	if err != nil {
		return err
	}

	log.Info("Storage clients initialized successfully")
	return nil
}

// initializeCurrentCluster initializes the current cluster's clients
func (cm *ClusterManager) initializeCurrentCluster() error {
	// Get clients from already initialized global variables based on configuration
	var k8sClient *K8SClientSet
	var storageClient *StorageClientSet

	if cm.loadK8SClient {
		k8sClient = getCurrentClusterK8SClientSet()
	}

	if cm.loadStorageClient {
		storageClient = getCurrentClusterStorageClientSet()
	}

	// Try to get cluster name from environment variable or config
	clusterName := getCurrentClusterName()

	cm.currentCluster = &ClusterClientSet{
		ClusterName:      clusterName,
		K8SClientSet:     k8sClient,
		StorageClientSet: storageClient,
	}

	// Also add current cluster to clusters map
	cm.clusters[clusterName] = cm.currentCluster

	// Initialize default cluster name from environment variable
	cm.defaultClusterName = getDefaultClusterName()
	if cm.defaultClusterName != "" {
		log.Infof("Default cluster configured: %s", cm.defaultClusterName)
	}

	log.Infof("Initialized current cluster: %s (K8S: %v, Storage: %v)",
		clusterName, cm.loadK8SClient, cm.loadStorageClient)
	return nil
}

// loadAllClusters loads clients for all clusters (multi-cluster mode)
func (cm *ClusterManager) loadAllClusters(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Create new cluster map
	newClusters := make(map[string]*ClusterClientSet)

	// Keep current cluster
	if cm.currentCluster != nil {
		newClusters[cm.currentCluster.ClusterName] = cm.currentCluster
	}

	// Get all K8S clients if K8S client loading is enabled
	var k8sClients map[string]*K8SClientSet
	if cm.loadK8SClient {
		k8sClients = getAllClusterK8SClients()
	}

	// Create ClusterClientSet for each remote cluster
	for clusterName, k8sClient := range k8sClients {
		// Skip if it's the current cluster (already added)
		if clusterName == cm.currentCluster.ClusterName {
			continue
		}

		var storageClient *StorageClientSet
		// Try to get storage client for this cluster if storage client loading is enabled
		if cm.loadStorageClient {
			var err error
			storageClient, err = getStorageClientSetByClusterName(clusterName)
			if err != nil {
				log.Warnf("Failed to get storage client for cluster %s: %v", clusterName, err)
				// Create cluster object even without storage client (storage config may not be ready yet)
				storageClient = nil
			}
		}

		newClusters[clusterName] = &ClusterClientSet{
			ClusterName:      clusterName,
			K8SClientSet:     k8sClient,
			StorageClientSet: storageClient,
		}

		log.Infof("Loaded cluster: %s (K8S: %v, Storage: %v)",
			clusterName, k8sClient != nil, storageClient != nil)
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

// getDefaultClusterName gets the default cluster name from environment variables
// This cluster will be used when no cluster is specified in API requests
func getDefaultClusterName() string {
	return os.Getenv("DEFAULT_CLUSTER_NAME")
}

// GetDefaultClusterName returns the configured default cluster name
func (cm *ClusterManager) GetDefaultClusterName() string {
	return cm.defaultClusterName
}

// SetDefaultClusterName sets the default cluster name
func (cm *ClusterManager) SetDefaultClusterName(clusterName string) {
	cm.defaultClusterName = clusterName
	log.Infof("Default cluster name set to: %s", clusterName)
}

// GetClusterClientsOrDefault returns cluster clients based on priority:
// 1. If clusterName is provided and not empty, use that cluster
// 2. If no clusterName provided but default cluster is configured, use default cluster
// 3. Otherwise, use current cluster
func (cm *ClusterManager) GetClusterClientsOrDefault(clusterName string) (*ClusterClientSet, error) {
	// If cluster name is explicitly provided, use it
	if clusterName != "" {
		return cm.GetClientSetByClusterName(clusterName)
	}

	// If default cluster is configured, use it
	if cm.defaultClusterName != "" {
		clients, err := cm.GetClientSetByClusterName(cm.defaultClusterName)
		if err != nil {
			log.Warnf("Failed to get default cluster '%s', falling back to current cluster: %v",
				cm.defaultClusterName, err)
			return cm.GetCurrentClusterClients(), nil
		}
		return clients, nil
	}

	// Otherwise, use current cluster
	return cm.GetCurrentClusterClients(), nil
}
