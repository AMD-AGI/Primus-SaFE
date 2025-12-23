package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/multi-cluster-config-exporter/pkg/grafana"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/env"
)

// MultiClusterStorageConfigListener watches for changes in multi-cluster config secrets
// When config changes are detected, it reinitializes K8S clientset and StorageClientSet
type MultiClusterStorageConfigListener struct {
	ctx              context.Context
	cancel           context.CancelFunc
	syncTaskCancel   context.CancelFunc        // Used to cancel sync task
	syncInterval     time.Duration             // Sync interval
	excludeNodes     map[string][]string       // Nodes to exclude per cluster: map[clusterName][]nodeName
	grafanaSyncer    *grafana.DatasourceSyncer // Grafana datasource syncer
	grafanaNamespace string                    // Namespace for Grafana datasources
}

// Environment variable names for exclude nodes configuration
const (
	// ExcludeControlPlaneNodesEnv is the environment variable name for exclude nodes configuration
	// Format: "node1,node2,node3" for global exclusion (applies to all clusters)
	// Or: "cluster1:node1,node2;cluster2:node3,node4" for per-cluster exclusion
	// Or: "*:node1,node2;cluster-a:node3" for mixed global and per-cluster exclusion
	// The "*" key means the exclusion applies to all clusters
	ExcludeControlPlaneNodesEnv = "EXCLUDE_CONTROL_PLANE_NODES"
)

// NewMultiClusterStorageConfigListener creates a new multi-cluster storage config listener
func NewMultiClusterStorageConfigListener(ctx context.Context) *MultiClusterStorageConfigListener {
	childCtx, cancel := context.WithCancel(ctx)

	// Get Grafana namespace from env or use default
	grafanaNamespace := env.GetString("GRAFANA_NAMESPACE", clientsets.StorageConfigSecretNamespace)

	// Grafana instance selector labels - default to primus-lens for primus-lens namespace
	grafanaInstanceLabelKey := env.GetString("GRAFANA_INSTANCE_LABEL_KEY", "system")
	grafanaInstanceLabelValue := env.GetString("GRAFANA_INSTANCE_LABEL_VALUE", "primus-lens")
	grafanaInstanceLabels := map[string]string{
		grafanaInstanceLabelKey: grafanaInstanceLabelValue,
	}

	listener := &MultiClusterStorageConfigListener{
		ctx:              childCtx,
		cancel:           cancel,
		syncInterval:     30 * time.Second,          // Default sync every 30 seconds
		excludeNodes:     make(map[string][]string), // Initialize empty exclude nodes map
		grafanaSyncer:    grafana.NewDatasourceSyncer(grafanaNamespace, grafanaInstanceLabels),
		grafanaNamespace: grafanaNamespace,
	}

	// Load exclude nodes from environment variable
	listener.loadExcludeNodesFromEnv()

	return listener
}

// loadExcludeNodesFromEnv loads exclude nodes configuration from environment variable
// Supported formats:
//   - Simple: "node1,node2,node3" - applies to all clusters (stored with "*" key)
//   - Per-cluster: "cluster1:node1,node2;cluster2:node3,node4"
//   - Mixed: "*:node1,node2;cluster-a:node3" - global and per-cluster
func (m *MultiClusterStorageConfigListener) loadExcludeNodesFromEnv() {
	excludeNodesStr := env.GetString(ExcludeControlPlaneNodesEnv, "")
	if excludeNodesStr == "" {
		log.Info("No exclude control plane nodes configured via environment variable")
		return
	}
	log.Infof("Loading exclude control plane nodes from env %s: %s", ExcludeControlPlaneNodesEnv, excludeNodesStr)

	// Check if it contains cluster-specific configuration (contains ":")
	if strings.Contains(excludeNodesStr, ":") {
		// Per-cluster format: "cluster1:node1,node2;cluster2:node3,node4"
		clusterConfigs := strings.Split(excludeNodesStr, ";")
		for _, clusterConfig := range clusterConfigs {
			clusterConfig = strings.TrimSpace(clusterConfig)
			if clusterConfig == "" {
				continue
			}

			parts := strings.SplitN(clusterConfig, ":", 2)
			if len(parts) != 2 {
				log.Warnf("Invalid exclude nodes format for cluster config: %s, expected 'cluster:node1,node2'", clusterConfig)
				continue
			}

			clusterName := strings.TrimSpace(parts[0])
			nodeNamesStr := strings.TrimSpace(parts[1])

			if clusterName == "" || nodeNamesStr == "" {
				log.Warnf("Invalid exclude nodes format: empty cluster name or node names in %s", clusterConfig)
				continue
			}

			nodeNames := parseNodeNames(nodeNamesStr)
			if len(nodeNames) > 0 {
				m.excludeNodes[clusterName] = nodeNames
				if clusterName == "*" {
					log.Infof("Loaded global exclude nodes (all clusters): %v", nodeNames)
				} else {
					log.Infof("Loaded exclude nodes for cluster %s: %v", clusterName, nodeNames)
				}
			}
		}
	} else {
		// Simple format: "node1,node2,node3" - applies to all clusters
		nodeNames := parseNodeNames(excludeNodesStr)
		if len(nodeNames) > 0 {
			m.excludeNodes["*"] = nodeNames
			log.Infof("Loaded global exclude nodes (all clusters): %v", nodeNames)
		}
	}
}

// parseNodeNames parses comma-separated node names and returns a cleaned slice
func parseNodeNames(nodeNamesStr string) []string {
	parts := strings.Split(nodeNamesStr, ",")
	nodeNames := make([]string, 0, len(parts))
	for _, name := range parts {
		name = strings.TrimSpace(name)
		if name != "" {
			nodeNames = append(nodeNames, name)
		}
	}
	return nodeNames
}

// SetExcludeNodes sets the nodes to exclude for a specific cluster
// clusterName: the name of the cluster
// nodeNames: list of node names to exclude when selecting control-plane nodes
func (m *MultiClusterStorageConfigListener) SetExcludeNodes(clusterName string, nodeNames []string) {
	m.excludeNodes[clusterName] = nodeNames
	log.Infof("Set exclude nodes for cluster %s: %v", clusterName, nodeNames)
}

// SetExcludeNodesForAllClusters sets the nodes to exclude for all clusters (using "*" as key)
// nodeNames: list of node names to exclude when selecting control-plane nodes from any cluster
func (m *MultiClusterStorageConfigListener) SetExcludeNodesForAllClusters(nodeNames []string) {
	m.excludeNodes["*"] = nodeNames
	log.Infof("Set global exclude nodes for all clusters: %v", nodeNames)
}

// isNodeExcluded checks if a node should be excluded for a given cluster
func (m *MultiClusterStorageConfigListener) isNodeExcluded(clusterName, nodeName string) bool {
	// Check cluster-specific exclusions
	if excludedNodes, ok := m.excludeNodes[clusterName]; ok {
		for _, excludedNode := range excludedNodes {
			if excludedNode == nodeName {
				return true
			}
		}
	}
	// Check global exclusions (using "*" as key)
	if excludedNodes, ok := m.excludeNodes["*"]; ok {
		for _, excludedNode := range excludedNodes {
			if excludedNode == nodeName {
				return true
			}
		}
	}
	return false
}

// Start starts the listener and begins watching K8S secret changes
func (m *MultiClusterStorageConfigListener) Start() error {
	log.Info("Starting multi-cluster storage config listener")

	// Initialize Grafana datasource syncer (non-blocking, errors are logged)
	if err := m.grafanaSyncer.Initialize(m.ctx); err != nil {
		log.Warnf("Failed to initialize Grafana datasource syncer (non-blocking): %v", err)
	}

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
	clusterParsedConfigs := make(map[string]*clientsets.PrimusLensClientConfig) // For Grafana sync
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
			log.Warnf("Please check: 1) Certificate validity, 2) RBAC permissions, 3) API server connectivity")
			log.Warnf("Cluster endpoint: %s", k8sClient.Config.Host)
			continue
		}

		// Parse the storage config
		storageConfig := &clientsets.PrimusLensClientConfig{}
		if err := storageConfig.LoadFromSecret(secret.Data); err != nil {
			log.Errorf("Failed to parse storage config for cluster %s: %v", clusterName, err)
			continue
		}

		// Get Ready control plane node IPs from the cluster
		nodeIPs, err := m.getReadyControlPlaneNodeIPs(clusterName, k8sClient)
		if err != nil {
			log.Errorf("Failed to get Ready control plane node IPs for cluster %s: %v", clusterName, err)
			continue
		}

		// Create proxy services and endpoints, and update the config
		if err := m.createProxyServicesForCluster(clusterName, storageConfig, nodeIPs); err != nil {
			log.Errorf("Failed to create proxy services for cluster %s: %v", clusterName, err)
			continue
		}

		// Store parsed config for Grafana sync
		clusterParsedConfigs[clusterName] = storageConfig

		// Marshal the updated config back to JSON
		// Need to convert back to secret.Data format (map[string][]byte)
		updatedData := make(map[string][]byte)
		if storageConfig.Opensearch != nil {
			opensearchBytes, err := json.Marshal(storageConfig.Opensearch)
			if err != nil {
				log.Errorf("Failed to marshal opensearch config for cluster %s: %v", clusterName, err)
				continue
			}
			updatedData["opensearch"] = opensearchBytes
		}
		if storageConfig.Prometheus != nil {
			prometheusBytes, err := json.Marshal(storageConfig.Prometheus)
			if err != nil {
				log.Errorf("Failed to marshal prometheus config for cluster %s: %v", clusterName, err)
				continue
			}
			updatedData["prometheus"] = prometheusBytes
		}
		if storageConfig.Postgres != nil {
			postgresBytes, err := json.Marshal(storageConfig.Postgres)
			if err != nil {
				log.Errorf("Failed to marshal postgres config for cluster %s: %v", clusterName, err)
				continue
			}
			updatedData["postgres"] = postgresBytes
		}

		// Serialize the updated config
		configBytes, err := json.Marshal(updatedData)
		if err != nil {
			log.Errorf("Failed to marshal storage config for cluster %s: %v", clusterName, err)
			continue
		}

		allStorageConfigs[clusterName] = configBytes
		log.Infof("Successfully fetched and updated storage config from cluster: %s", clusterName)
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

	// 4. Sync Grafana datasources (non-blocking, errors are logged but don't fail the main flow)
	if len(clusterParsedConfigs) > 0 {
		if err := m.grafanaSyncer.SyncDatasources(m.ctx, clusterParsedConfigs); err != nil {
			log.Warnf("Failed to sync Grafana datasources (non-blocking): %v", err)
		}
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

// getReadyControlPlaneNodeIPs gets up to 3 Ready control-plane node IPs from a cluster
func (m *MultiClusterStorageConfigListener) getReadyControlPlaneNodeIPs(clusterName string, k8sClient *clientsets.K8SClientSet) ([]string, error) {
	// List all nodes
	nodes, err := k8sClient.Clientsets.CoreV1().Nodes().List(
		m.ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("no nodes found in cluster %s", clusterName)
	}

	// Filter Ready control-plane nodes and extract IPs
	nodeIPs := make([]string, 0, 3)
	for _, node := range nodes.Items {
		// Check if node is excluded
		if m.isNodeExcluded(clusterName, node.Name) {
			log.Infof("Skipping excluded node %s for cluster %s", node.Name, clusterName)
			continue
		}

		// Only select control-plane nodes
		_, hasControlPlaneLabel := node.Labels["node-role.kubernetes.io/control-plane"]
		_, hasMasterLabel := node.Labels["node-role.kubernetes.io/master"]
		if !hasControlPlaneLabel && !hasMasterLabel {
			continue
		}

		// Check if node is Ready
		isReady := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				isReady = true
				break
			}
		}
		if !isReady {
			continue
		}

		// Extract IP from ready control plane node
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP || addr.Type == corev1.NodeExternalIP {
				nodeIPs = append(nodeIPs, addr.Address)
				log.Infof("Selected Ready control plane node %s with IP %s for cluster %s", node.Name, addr.Address, clusterName)
				break
			}
		}

		// Limit to 3 nodes
		if len(nodeIPs) >= 3 {
			break
		}
	}

	if len(nodeIPs) == 0 {
		return nil, fmt.Errorf("no Ready control plane nodes found in cluster %s", clusterName)
	}

	log.Infof("Found %d Ready control plane node IPs for cluster %s: %v", len(nodeIPs), clusterName, nodeIPs)
	return nodeIPs, nil
}

// createProxyServiceAndEndpoint creates a proxy service and endpoint for a remote cluster component
func (m *MultiClusterStorageConfigListener) createProxyServiceAndEndpoint(
	clusterName, componentName string,
	nodeIPs []string,
	nodePort int32,
	servicePort int32,
) (string, int32, error) {
	currentClientSet := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet
	namespace := clientsets.StorageConfigSecretNamespace

	// Generate service name: primus-lens-{component}-{cluster}
	serviceName := fmt.Sprintf("primus-lens-%s-%s", componentName, clusterName)

	// Create Service without selector
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "primus-lens",
				"component": componentName,
				"cluster":   clusterName,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       componentName,
					Protocol:   corev1.ProtocolTCP,
					Port:       servicePort,
					TargetPort: intstr.FromInt(int(nodePort)),
				},
			},
		},
	}

	// Try to create service, update if it already exists
	_, err := currentClientSet.Clientsets.CoreV1().Services(namespace).Create(
		m.ctx,
		service,
		metav1.CreateOptions{},
	)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Update existing service
			_, err = currentClientSet.Clientsets.CoreV1().Services(namespace).Update(
				m.ctx,
				service,
				metav1.UpdateOptions{},
			)
			if err != nil {
				return "", 0, fmt.Errorf("failed to update proxy service: %w", err)
			}
			log.Infof("Updated proxy service: %s/%s", namespace, serviceName)
		} else {
			return "", 0, fmt.Errorf("failed to create proxy service: %w", err)
		}
	} else {
		log.Infof("Created proxy service: %s/%s", namespace, serviceName)
	}

	// Create Endpoints
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: make([]corev1.EndpointAddress, 0, len(nodeIPs)),
				Ports: []corev1.EndpointPort{
					{
						Name:     componentName,
						Protocol: corev1.ProtocolTCP,
						Port:     nodePort,
					},
				},
			},
		},
	}

	// Add all node IPs as endpoints
	for _, ip := range nodeIPs {
		endpoints.Subsets[0].Addresses = append(endpoints.Subsets[0].Addresses, corev1.EndpointAddress{
			IP: ip,
		})
	}

	// Try to create endpoints, update if it already exists
	_, err = currentClientSet.Clientsets.CoreV1().Endpoints(namespace).Create(
		m.ctx,
		endpoints,
		metav1.CreateOptions{},
	)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Update existing endpoints
			_, err = currentClientSet.Clientsets.CoreV1().Endpoints(namespace).Update(
				m.ctx,
				endpoints,
				metav1.UpdateOptions{},
			)
			if err != nil {
				return "", 0, fmt.Errorf("failed to update proxy endpoints: %w", err)
			}
			log.Infof("Updated proxy endpoints: %s/%s", namespace, serviceName)
		} else {
			return "", 0, fmt.Errorf("failed to create proxy endpoints: %w", err)
		}
	} else {
		log.Infof("Created proxy endpoints: %s/%s with %d IPs", namespace, serviceName, len(nodeIPs))
	}

	return serviceName, servicePort, nil
}

// createProxyServicesForCluster creates proxy services for all storage components in a cluster
func (m *MultiClusterStorageConfigListener) createProxyServicesForCluster(
	clusterName string,
	config *clientsets.PrimusLensClientConfig,
	nodeIPs []string,
) error {
	// Handle Opensearch
	if config.Opensearch != nil && config.Opensearch.NodePort > 0 {
		serviceName, port, err := m.createProxyServiceAndEndpoint(
			clusterName,
			"opensearch",
			nodeIPs,
			config.Opensearch.NodePort,
			9200, // Standard opensearch port
		)
		if err != nil {
			log.Errorf("Failed to create proxy service for opensearch in cluster %s: %v", clusterName, err)
		} else {
			// Update config to use proxy service
			config.Opensearch.Service = serviceName
			config.Opensearch.Port = port
			log.Infof("Updated opensearch config for cluster %s: service=%s, port=%d", clusterName, serviceName, port)
		}
	}

	// Handle Prometheus
	if config.Prometheus != nil {
		// Prometheus write endpoint
		if config.Prometheus.WriteNodePort > 0 {
			serviceName, port, err := m.createProxyServiceAndEndpoint(
				clusterName,
				"prometheus-write",
				nodeIPs,
				config.Prometheus.WriteNodePort,
				9090, // Standard prometheus port
			)
			if err != nil {
				log.Errorf("Failed to create proxy service for prometheus-write in cluster %s: %v", clusterName, err)
			} else {
				config.Prometheus.WriteService = serviceName
				config.Prometheus.WritePort = port
				log.Infof("Updated prometheus write config for cluster %s: service=%s, port=%d", clusterName, serviceName, port)
			}
		}

		// Prometheus read endpoint
		if config.Prometheus.ReadNodePort > 0 {
			serviceName, port, err := m.createProxyServiceAndEndpoint(
				clusterName,
				"prometheus-read",
				nodeIPs,
				config.Prometheus.ReadNodePort,
				9090, // Standard prometheus port
			)
			if err != nil {
				log.Errorf("Failed to create proxy service for prometheus-read in cluster %s: %v", clusterName, err)
			} else {
				config.Prometheus.ReadService = serviceName
				config.Prometheus.ReadPort = port
				log.Infof("Updated prometheus read config for cluster %s: service=%s, port=%d", clusterName, serviceName, port)
			}
		}
	}

	// Handle Postgres
	if config.Postgres != nil && config.Postgres.NodePort > 0 {
		serviceName, port, err := m.createProxyServiceAndEndpoint(
			clusterName,
			"postgres",
			nodeIPs,
			config.Postgres.NodePort,
			5432, // Standard postgres port
		)
		if err != nil {
			log.Errorf("Failed to create proxy service for postgres in cluster %s: %v", clusterName, err)
		} else {
			config.Postgres.Service = serviceName
			config.Postgres.Port = port
			log.Infof("Updated postgres config for cluster %s: service=%s, port=%d", clusterName, serviceName, port)
		}
	}

	return nil
}
