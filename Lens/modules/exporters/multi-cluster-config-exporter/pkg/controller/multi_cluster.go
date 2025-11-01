package controller

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// MultiClusterStorageConfigListener watches for changes in multi-cluster config secrets
// When config changes are detected, it reinitializes K8S clientset and StorageClientSet
type MultiClusterStorageConfigListener struct {
	ctx            context.Context
	cancel         context.CancelFunc
	syncTaskCancel context.CancelFunc // Used to cancel sync task
	syncInterval   time.Duration      // Sync interval
}

// NewMultiClusterStorageConfigListener creates a new multi-cluster storage config listener
func NewMultiClusterStorageConfigListener(ctx context.Context) *MultiClusterStorageConfigListener {
	childCtx, cancel := context.WithCancel(ctx)
	return &MultiClusterStorageConfigListener{
		ctx:          childCtx,
		cancel:       cancel,
		syncInterval: 30 * time.Second, // Default sync every 30 seconds
	}
}

// Start starts the listener and begins watching K8S secret changes
func (m *MultiClusterStorageConfigListener) Start() error {
	log.Info("Starting multi-cluster storage config listener")

	// Start watching multi-k8s-config secret
	go m.watchK8SConfigSecret()

	return nil
}

// Stop stops the listener
func (m *MultiClusterStorageConfigListener) Stop() {
	log.Info("Stopping multi-cluster storage config listener")

	// Stop sync task
	if m.syncTaskCancel != nil {
		m.syncTaskCancel()
		m.syncTaskCancel = nil
	}

	// Stop the entire listener
	if m.cancel != nil {
		m.cancel()
	}
}

// watchK8SConfigSecret watches for changes in multi-cluster K8S config secret
func (m *MultiClusterStorageConfigListener) watchK8SConfigSecret() {
	for {
		select {
		case <-m.ctx.Done():
			log.Info("K8S config secret watcher stopped")
			return
		default:
			if err := m.doWatchK8SConfigSecret(); err != nil {
				log.Errorf("Error watching K8S config secret: %v, retrying in 10 seconds...", err)
				time.Sleep(10 * time.Second)
			}
		}
	}
}

// doWatchK8SConfigSecret executes the watch for K8S config secret
func (m *MultiClusterStorageConfigListener) doWatchK8SConfigSecret() error {
	clientSet := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet

	watcher, err := clientSet.Clientsets.CoreV1().Secrets(clientsets.StorageConfigSecretNamespace).Watch(
		m.ctx,
		metav1.ListOptions{
			FieldSelector: "metadata.name=" + clientsets.MultiK8SConfigSecretName,
		},
	)
	if err != nil {
		return err
	}
	defer watcher.Stop()

	log.Infof("Started watching K8S config secret: %s/%s",
		clientsets.StorageConfigSecretNamespace,
		clientsets.MultiK8SConfigSecretName)

	for {
		select {
		case <-m.ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				log.Warn("K8S config secret watcher channel closed, restarting...")
				return nil // Returning nil will trigger reconnection
			}

			if err := m.handleK8SConfigSecretEvent(event); err != nil {
				log.Errorf("Failed to handle K8S config secret event: %v", err)
			}
		}
	}
}

// handleK8SConfigSecretEvent handles K8S config secret events
func (m *MultiClusterStorageConfigListener) handleK8SConfigSecretEvent(event watch.Event) error {
	switch event.Type {
	case watch.Added, watch.Modified:
		secret, ok := event.Object.(*corev1.Secret)
		if !ok {
			log.Error("Failed to cast event object to Secret")
			return nil
		}

		log.Infof("Detected K8S config secret change (event: %s), K8S clientsets will be reloaded automatically", event.Type)
		log.Infof("Detected %d cluster configs", len(secret.Data))

		// Start or restart scheduled sync task to collect storage configs from all clusters periodically
		m.startStorageConfigSyncTask()

	case watch.Deleted:
		log.Warn("K8S config secret deleted, this may cause errors")
		// Stop sync task
		if m.syncTaskCancel != nil {
			m.syncTaskCancel()
			m.syncTaskCancel = nil
		}
	case watch.Error:
		log.Error("Received error event from K8S config secret watcher")
	}

	return nil
}

// startStorageConfigSyncTask starts a scheduled sync task to collect storage configs from K8S clusters periodically
func (m *MultiClusterStorageConfigListener) startStorageConfigSyncTask() {
	// Stop existing sync task if it's running
	if m.syncTaskCancel != nil {
		log.Info("Stopping existing storage config sync task...")
		m.syncTaskCancel()
		m.syncTaskCancel = nil
	}

	// Create new context for sync task
	syncCtx, syncCancel := context.WithCancel(m.ctx)
	m.syncTaskCancel = syncCancel

	log.Infof("Starting storage config sync task with interval: %v", m.syncInterval)

	// Start sync task
	go m.runStorageConfigSyncTask(syncCtx)
}

// runStorageConfigSyncTask runs the scheduled sync task
func (m *MultiClusterStorageConfigListener) runStorageConfigSyncTask(ctx context.Context) {
	// Execute sync immediately
	if err := m.syncStorageConfigsFromAllClusters(); err != nil {
		log.Errorf("Failed to sync storage configs: %v", err)
	}

	// Create ticker
	ticker := time.NewTicker(m.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Storage config sync task stopped")
			return
		case <-ticker.C:
			log.Info("Running scheduled storage config sync...")
			if err := m.syncStorageConfigsFromAllClusters(); err != nil {
				log.Errorf("Failed to sync storage configs: %v", err)
			}
		}
	}
}

// syncStorageConfigsFromAllClusters collects storage configs from all K8S clusters and aggregates them to the current cluster
func (m *MultiClusterStorageConfigListener) syncStorageConfigsFromAllClusters() error {
	log.Info("Syncing storage configs from all clusters...")

	// 1. Get all cluster clients through ClusterManager
	cm := clientsets.GetClusterManager()
	allClusters := cm.ListAllClientSets()
	if len(allClusters) == 0 {
		log.Warn("No K8S cluster clients available")
		return nil
	}

	// 2. Collect storage configs from each cluster
	allStorageConfigs := make(map[string][]byte)
	log.Infof("All clusters: %v", allClusters)
	for clusterName, cluster := range allClusters {
		k8sClient := cluster.K8SClientSet
		log.Infof("Fetching storage config from cluster: %s", clusterName)

		// Get storage config secret from the cluster
		secret, err := k8sClient.Clientsets.CoreV1().Secrets(clientsets.StorageConfigSecretNamespace).Get(
			m.ctx,
			clientsets.StorageConfigSecretName,
			metav1.GetOptions{},
		)
		if err != nil {
			log.Warnf("Failed to get storage config secret from cluster %s: %v", clusterName, err)
			continue
		}

		// Serialize the entire secret.Data to JSON as the cluster's config
		configBytes, err := json.Marshal(secret.Data)
		if err != nil {
			log.Errorf("Failed to marshal storage config for cluster %s: %v", clusterName, err)
			continue
		}

		allStorageConfigs[clusterName] = configBytes
		log.Infof("Successfully fetched storage config from cluster: %s", clusterName)
	}

	// 3. Update the aggregated configs to the current cluster's multi-storage-config secret
	if len(allStorageConfigs) > 0 {
		if err := m.updateMultiStorageConfigSecret(allStorageConfigs); err != nil {
			log.Errorf("Failed to update multi-storage-config secret: %v", err)
			return err
		}
		log.Infof("Successfully synced storage configs from %d clusters", len(allStorageConfigs))
	} else {
		log.Warn("No storage configs collected from any cluster")
	}

	return nil
}

// updateMultiStorageConfigSecret updates the multi-storage-config secret in the current cluster
func (m *MultiClusterStorageConfigListener) updateMultiStorageConfigSecret(configs map[string][]byte) error {
	currentClientSet := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet

	// Get existing secret, create if it doesn't exist
	secret, err := currentClientSet.Clientsets.CoreV1().Secrets(clientsets.StorageConfigSecretNamespace).Get(
		m.ctx,
		clientsets.MultiStorageConfigSecretName,
		metav1.GetOptions{},
	)

	if err != nil {
		// Secret doesn't exist, create a new one
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientsets.MultiStorageConfigSecretName,
				Namespace: clientsets.StorageConfigSecretNamespace,
			},
			Data: configs,
		}

		_, err = currentClientSet.Clientsets.CoreV1().Secrets(clientsets.StorageConfigSecretNamespace).Create(
			m.ctx,
			secret,
			metav1.CreateOptions{},
		)
		if err != nil {
			return err
		}
		log.Infof("Created multi-storage-config secret with %d cluster configs", len(configs))
	} else {
		// Secret already exists, update data
		secret.Data = configs

		_, err = currentClientSet.Clientsets.CoreV1().Secrets(clientsets.StorageConfigSecretNamespace).Update(
			m.ctx,
			secret,
			metav1.UpdateOptions{},
		)
		if err != nil {
			return err
		}
		log.Infof("Updated multi-storage-config secret with %d cluster configs", len(configs))
	}

	return nil
}
