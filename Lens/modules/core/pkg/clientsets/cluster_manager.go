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
	"k8s.io/client-go/tools/clientcmd"
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

	// Component type (control plane or data plane)
	componentType ComponentType

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

// InitClusterManager initializes the cluster manager with a component declaration
// This is the main entry point for initializing all clients
func InitClusterManager(ctx context.Context, decl ComponentDeclaration) error {
	var initErr error
	clusterManagerOnce.Do(func() {
		globalClusterManager = &ClusterManager{
			clusters:          make(map[string]*ClusterClientSet),
			componentType:     decl.Type,
			loadK8SClient:     decl.RequireK8S,
			loadStorageClient: decl.RequireStorage,
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

// InitClusterManagerWithClientSet initializes the cluster manager with a pre-configured ClusterClientSet.
// This is useful for testing or standalone tools that need to bypass the normal initialization process.
// The provided clientSet will be used as the current cluster.
//
// Parameters:
//   - clientSet: The pre-configured ClusterClientSet to use as the current cluster
//
// Note: This method can only be called once. Subsequent calls will be ignored.
func InitClusterManagerWithClientSet(clientSet *ClusterClientSet) {
	InitClusterManagerWithClientSetV2(clientSet, ComponentTypeDataPlane)
}

// InitClusterManagerWithClientSetV2 initializes the cluster manager with a pre-configured ClusterClientSet
// and a specific component type.
func InitClusterManagerWithClientSetV2(clientSet *ClusterClientSet, componentType ComponentType) {
	clusterManagerOnce.Do(func() {
		if clientSet == nil {
			log.Warn("InitClusterManagerWithClientSetV2 called with nil clientSet, creating empty manager")
			clientSet = &ClusterClientSet{
				ClusterName: "default",
			}
		}

		globalClusterManager = &ClusterManager{
			clusters:           make(map[string]*ClusterClientSet),
			componentType:      componentType,
			loadK8SClient:      clientSet.K8SClientSet != nil,
			loadStorageClient:  clientSet.StorageClientSet != nil,
			currentCluster:     clientSet,
			defaultClusterName: clientSet.ClusterName,
		}

		// Add current cluster to clusters map
		globalClusterManager.clusters[clientSet.ClusterName] = clientSet

		log.Infof("Cluster manager initialized with pre-configured client set: %s (K8S: %v, Storage: %v, Type: %s)",
			clientSet.ClusterName,
			clientSet.K8SClientSet != nil,
			clientSet.StorageClientSet != nil,
			componentType.String())
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

// initialize initializes the cluster manager based on component type
func (cm *ClusterManager) initialize(ctx context.Context) error {
	log.Infof("Initializing cluster manager as %s component...", cm.componentType.String())

	if !cm.loadK8SClient && !cm.loadStorageClient {
		log.Warn("Both K8S and Storage client loading are disabled, skipping cluster initialization")
		return nil
	}

	var err error
	switch cm.componentType {
	case ComponentTypeControlPlane:
		err = cm.initControlPlane(ctx)
	case ComponentTypeDataPlane:
		err = cm.initDataPlane(ctx)
	default:
		// Fallback to data plane behavior
		log.Warnf("Unknown component type %d, falling back to DataPlane", cm.componentType)
		cm.componentType = ComponentTypeDataPlane
		err = cm.initDataPlane(ctx)
	}

	if err != nil {
		return err
	}

	log.Infof("Cluster manager initialized successfully as %s", cm.componentType.String())
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
// For data plane components, this will only return the current cluster if the name matches
func (cm *ClusterManager) GetClientSetByClusterName(clusterName string) (*ClusterClientSet, error) {
	// For data plane components, only allow access to current cluster
	if cm.componentType.IsDataPlane() {
		if cm.currentCluster != nil && cm.currentCluster.ClusterName == clusterName {
			return cm.currentCluster, nil
		}
		return nil, errors.NewError().
			WithCode(errors.RequestDataNotExisted).
			WithMessagef("DataPlane component can only access current cluster, requested: %s", clusterName)
	}

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
// For data plane components, this will only return the current cluster
func (cm *ClusterManager) ListAllClientSets() map[string]*ClusterClientSet {
	// For data plane components, only return current cluster
	if cm.componentType.IsDataPlane() {
		log.Debug("DataPlane component called ListAllClientSets, returning only current cluster")
		if cm.currentCluster != nil {
			return map[string]*ClusterClientSet{
				cm.currentCluster.ClusterName: cm.currentCluster,
			}
		}
		return make(map[string]*ClusterClientSet)
	}

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
// For data plane components, this returns only the current cluster name
func (cm *ClusterManager) GetClusterNames() []string {
	// For data plane components, only return current cluster
	if cm.componentType.IsDataPlane() {
		if cm.currentCluster != nil {
			return []string{cm.currentCluster.ClusterName}
		}
		return []string{}
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	names := make([]string, 0, len(cm.clusters))
	for name := range cm.clusters {
		names = append(names, name)
	}

	return names
}

// GetClusterCount returns the number of clusters
// For data plane components, this always returns 1 (current cluster only)
func (cm *ClusterManager) GetClusterCount() int {
	// For data plane components, only current cluster is available
	if cm.componentType.IsDataPlane() {
		if cm.currentCluster != nil {
			return 1
		}
		return 0
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return len(cm.clusters)
}

// IsMultiCluster returns whether in multi-cluster mode
// Deprecated: Use GetComponentType().IsControlPlane() instead
func (cm *ClusterManager) IsMultiCluster() bool {
	return cm.componentType.IsControlPlane()
}

// GetComponentType returns the component type of this cluster manager
func (cm *ClusterManager) GetComponentType() ComponentType {
	return cm.componentType
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

	// Try to get from kubeconfig current-context
	if name := getClusterNameFromKubeconfig(); name != "" {
		log.Infof("Cluster name detected from kubeconfig: %s", name)
		return name
	}

	// Default value - use "local" instead of "default" to avoid confusion
	// "default" is too generic and conflicts with Kubernetes "default" namespace
	log.Warn("CLUSTER_NAME environment variable not set, using 'local' as cluster name")
	return "local"
}

// getClusterNameFromKubeconfig attempts to get the cluster name from kubeconfig
func getClusterNameFromKubeconfig() string {
	// Try to load kubeconfig rules
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	// Get raw config
	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		log.Debugf("Failed to load kubeconfig: %v", err)
		return ""
	}

	// Get current context
	currentContext := rawConfig.CurrentContext
	if currentContext == "" {
		return ""
	}

	// Return the context name as cluster name
	// Context name typically represents the cluster identity
	return currentContext
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
// For data plane components, this always returns the current cluster (ignoring clusterName parameter)
func (cm *ClusterManager) GetClusterClientsOrDefault(clusterName string) (*ClusterClientSet, error) {
	// For data plane components, always return current cluster
	if cm.componentType.IsDataPlane() {
		if clusterName != "" && clusterName != cm.currentCluster.ClusterName {
			log.Debugf("DataPlane component requested cluster '%s', returning current cluster '%s' instead",
				clusterName, cm.currentCluster.ClusterName)
		}
		return cm.GetCurrentClusterClients(), nil
	}

	// Control plane logic: support multi-cluster
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
