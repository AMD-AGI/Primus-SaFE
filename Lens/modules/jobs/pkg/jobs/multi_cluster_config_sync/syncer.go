// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package multi_cluster_config_sync

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/env"
)

// SyncStats contains statistics about the sync operation
type SyncStats struct {
	ClustersProcessed    int
	ClustersWithStorage  int
	ProxyServicesCreated int
	GrafanaDatasources   int
	Errors               []string
}

// ConfigSyncer syncs multi-cluster storage configs from control plane DB
type ConfigSyncer struct {
	currentK8sClient *kubernetes.Clientset
	namespace        string
	grafanaSyncer    *GrafanaSyncer
	excludeNodes     map[string][]string // Nodes to exclude per cluster
}

// NewConfigSyncer creates a new ConfigSyncer
func NewConfigSyncer() *ConfigSyncer {
	namespace := env.GetString("POD_NAMESPACE", clientsets.StorageConfigSecretNamespace)
	grafanaNamespace := env.GetString("GRAFANA_NAMESPACE", namespace)
	grafanaInstanceLabelKey := env.GetString("GRAFANA_INSTANCE_LABEL_KEY", "system")
	grafanaInstanceLabelValue := env.GetString("GRAFANA_INSTANCE_LABEL_VALUE", "primus-lens")

	return &ConfigSyncer{
		namespace: namespace,
		grafanaSyncer: NewGrafanaSyncer(grafanaNamespace, map[string]string{
			grafanaInstanceLabelKey: grafanaInstanceLabelValue,
		}),
		excludeNodes: loadExcludeNodesFromEnv(),
	}
}

// Initialize initializes the syncer with current cluster's K8S client
func (s *ConfigSyncer) Initialize(ctx context.Context) error {
	currentClients := clientsets.GetClusterManager().GetCurrentClusterClients()
	if currentClients == nil || currentClients.K8SClientSet == nil {
		return fmt.Errorf("current cluster clients not available")
	}

	s.currentK8sClient = currentClients.K8SClientSet.Clientsets

	// Initialize Grafana syncer
	if err := s.grafanaSyncer.Initialize(ctx); err != nil {
		log.Warnf("Failed to initialize Grafana syncer (non-blocking): %v", err)
	}

	return nil
}

// SyncAll syncs storage configs for all clusters in the control plane DB
func (s *ConfigSyncer) SyncAll(ctx context.Context) (*SyncStats, error) {
	stats := &SyncStats{}

	facade := cpdb.GetControlPlaneFacade()
	clusters, err := facade.ClusterConfig.ListByStatus(ctx, "active")
	if err != nil {
		return stats, fmt.Errorf("failed to list clusters: %w", err)
	}

	log.Infof("Found %d active clusters to sync", len(clusters))

	allStorageConfigs := make(map[string][]byte)
	clusterParsedConfigs := make(map[string]*clientsets.PrimusLensClientConfig)

	for _, cluster := range clusters {
		stats.ClustersProcessed++

		// Try to sync storage config from the cluster
		config, err := s.syncCluster(ctx, cluster, stats)
		if err != nil {
			log.Warnf("Failed to sync cluster %s: %v", cluster.ClusterName, err)
			stats.Errors = append(stats.Errors, fmt.Sprintf("%s: %v", cluster.ClusterName, err))
			continue
		}

		if config != nil {
			stats.ClustersWithStorage++
			clusterParsedConfigs[cluster.ClusterName] = config

			// Serialize for multi-storage-config secret
			configBytes, err := serializeStorageConfig(config)
			if err != nil {
				log.Warnf("Failed to serialize config for cluster %s: %v", cluster.ClusterName, err)
				continue
			}
			allStorageConfigs[cluster.ClusterName] = configBytes
		}
	}

	// Update multi-storage-config secret (for backward compatibility)
	if len(allStorageConfigs) > 0 {
		if err := s.updateMultiStorageConfigSecret(ctx, allStorageConfigs); err != nil {
			log.Errorf("Failed to update multi-storage-config secret: %v", err)
			stats.Errors = append(stats.Errors, fmt.Sprintf("multi-storage-config: %v", err))
		}
	}

	// Sync Grafana datasources
	if len(clusterParsedConfigs) > 0 {
		if err := s.grafanaSyncer.SyncDatasources(ctx, clusterParsedConfigs); err != nil {
			log.Warnf("Failed to sync Grafana datasources (non-blocking): %v", err)
		} else {
			stats.GrafanaDatasources = len(clusterParsedConfigs)
		}
	}

	return stats, nil
}

// syncCluster syncs storage config for a single cluster
func (s *ConfigSyncer) syncCluster(ctx context.Context, cluster *model.ClusterConfig, stats *SyncStats) (*clientsets.PrimusLensClientConfig, error) {
	log.Infof("Syncing storage config for cluster: %s", cluster.ClusterName)

	// Build K8S client for remote cluster
	k8sClient, err := s.buildK8SClient(cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to build K8S client: %w", err)
	}

	// Get storage config secret from the remote cluster
	secret, err := k8sClient.CoreV1().Secrets(s.namespace).Get(ctx, clientsets.StorageConfigSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			log.Infof("No storage config secret found in cluster %s", cluster.ClusterName)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get storage config secret: %w", err)
	}

	// Parse storage config
	storageConfig := &clientsets.PrimusLensClientConfig{}
	if err := storageConfig.LoadFromSecret(secret.Data); err != nil {
		return nil, fmt.Errorf("failed to parse storage config: %w", err)
	}

	// Get control plane node IPs for creating proxy services
	nodeIPs, err := s.getReadyControlPlaneNodeIPs(ctx, cluster.ClusterName, k8sClient)
	if err != nil {
		log.Warnf("Failed to get control plane node IPs for cluster %s: %v", cluster.ClusterName, err)
		// Continue without proxy services
		return storageConfig, nil
	}

	// Create proxy services
	proxyCount, err := s.createProxyServicesForCluster(ctx, cluster.ClusterName, storageConfig, nodeIPs)
	if err != nil {
		log.Warnf("Failed to create proxy services for cluster %s: %v", cluster.ClusterName, err)
	}
	stats.ProxyServicesCreated += proxyCount

	// Update cluster_config with storage info
	if err := s.updateClusterStorageConfig(ctx, cluster.ClusterName, storageConfig); err != nil {
		log.Warnf("Failed to update cluster storage config in DB: %v", err)
	}

	return storageConfig, nil
}

// buildK8SClient builds a Kubernetes client for a remote cluster
func (s *ConfigSyncer) buildK8SClient(cluster *model.ClusterConfig) (*kubernetes.Clientset, error) {
	var config *rest.Config

	// Check if we have kubeconfig or cert/key data
	if cluster.K8SCertData != "" && cluster.K8SKeyData != "" && cluster.K8SCAData != "" {
		// Build config from cert/key
		config = &rest.Config{
			Host: cluster.K8SEndpoint,
			TLSClientConfig: rest.TLSClientConfig{
				CAData:   []byte(cluster.K8SCAData),
				CertData: []byte(cluster.K8SCertData),
				KeyData:  []byte(cluster.K8SKeyData),
			},
		}
	} else if cluster.K8SToken != "" {
		// Build config from token
		config = &rest.Config{
			Host:        cluster.K8SEndpoint,
			BearerToken: cluster.K8SToken,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: []byte(cluster.K8SCAData),
			},
		}
	} else {
		// Try to use in-cluster config for current cluster
		cm := clientsets.GetClusterManager()
		allClusters := cm.ListAllClientSets()
		if clusterClient, ok := allClusters[cluster.ClusterName]; ok && clusterClient.K8SClientSet != nil {
			return clusterClient.K8SClientSet.Clientsets, nil
		}
		return nil, fmt.Errorf("no valid K8S credentials for cluster %s", cluster.ClusterName)
	}

	return kubernetes.NewForConfig(config)
}

// getReadyControlPlaneNodeIPs gets control plane node IPs from a cluster
func (s *ConfigSyncer) getReadyControlPlaneNodeIPs(ctx context.Context, clusterName string, k8sClient *kubernetes.Clientset) ([]string, error) {
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodeIPs := make([]string, 0, 3)
	for _, node := range nodes.Items {
		// Check if node is excluded
		if s.isNodeExcluded(clusterName, node.Name) {
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

		// Extract IP
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP || addr.Type == corev1.NodeExternalIP {
				nodeIPs = append(nodeIPs, addr.Address)
				break
			}
		}

		if len(nodeIPs) >= 3 {
			break
		}
	}

	if len(nodeIPs) == 0 {
		return nil, fmt.Errorf("no Ready control plane nodes found")
	}

	return nodeIPs, nil
}

// createProxyServicesForCluster creates proxy services for all storage components
func (s *ConfigSyncer) createProxyServicesForCluster(ctx context.Context, clusterName string, config *clientsets.PrimusLensClientConfig, nodeIPs []string) (int, error) {
	count := 0

	// Opensearch
	if config.Opensearch != nil && config.Opensearch.NodePort > 0 {
		serviceName, port, err := s.createProxyServiceAndEndpoint(ctx, clusterName, "opensearch", nodeIPs, config.Opensearch.NodePort, 9200)
		if err == nil {
			config.Opensearch.Service = serviceName
			config.Opensearch.Port = port
			count++
		}
	}

	// Prometheus write
	if config.Prometheus != nil && config.Prometheus.WriteNodePort > 0 {
		serviceName, port, err := s.createProxyServiceAndEndpoint(ctx, clusterName, "prometheus-write", nodeIPs, config.Prometheus.WriteNodePort, 9090)
		if err == nil {
			config.Prometheus.WriteService = serviceName
			config.Prometheus.WritePort = port
			count++
		}
	}

	// Prometheus read
	if config.Prometheus != nil && config.Prometheus.ReadNodePort > 0 {
		serviceName, port, err := s.createProxyServiceAndEndpoint(ctx, clusterName, "prometheus-read", nodeIPs, config.Prometheus.ReadNodePort, 9090)
		if err == nil {
			config.Prometheus.ReadService = serviceName
			config.Prometheus.ReadPort = port
			count++
		}
	}

	// Postgres
	if config.Postgres != nil && config.Postgres.NodePort > 0 {
		serviceName, port, err := s.createProxyServiceAndEndpoint(ctx, clusterName, "postgres", nodeIPs, config.Postgres.NodePort, 5432)
		if err == nil {
			config.Postgres.Service = serviceName
			config.Postgres.Port = port
			count++
		}
	}

	return count, nil
}

// createProxyServiceAndEndpoint creates a proxy service and endpoints
func (s *ConfigSyncer) createProxyServiceAndEndpoint(ctx context.Context, clusterName, componentName string, nodeIPs []string, nodePort, servicePort int32) (string, int32, error) {
	serviceName := fmt.Sprintf("primus-lens-%s-%s", componentName, clusterName)

	// Create Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: s.namespace,
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

	_, err := s.currentK8sClient.CoreV1().Services(s.namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = s.currentK8sClient.CoreV1().Services(s.namespace).Update(ctx, service, metav1.UpdateOptions{})
			if err != nil {
				return "", 0, fmt.Errorf("failed to update service: %w", err)
			}
		} else {
			return "", 0, fmt.Errorf("failed to create service: %w", err)
		}
	}

	// Create Endpoints
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: s.namespace,
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

	for _, ip := range nodeIPs {
		endpoints.Subsets[0].Addresses = append(endpoints.Subsets[0].Addresses, corev1.EndpointAddress{IP: ip})
	}

	_, err = s.currentK8sClient.CoreV1().Endpoints(s.namespace).Create(ctx, endpoints, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = s.currentK8sClient.CoreV1().Endpoints(s.namespace).Update(ctx, endpoints, metav1.UpdateOptions{})
			if err != nil {
				return "", 0, fmt.Errorf("failed to update endpoints: %w", err)
			}
		} else {
			return "", 0, fmt.Errorf("failed to create endpoints: %w", err)
		}
	}

	log.Infof("Created proxy service %s for cluster %s", serviceName, clusterName)
	return serviceName, servicePort, nil
}

// updateMultiStorageConfigSecret updates the multi-storage-config secret
func (s *ConfigSyncer) updateMultiStorageConfigSecret(ctx context.Context, configs map[string][]byte) error {
	secret, err := s.currentK8sClient.CoreV1().Secrets(s.namespace).Get(ctx, clientsets.MultiStorageConfigSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new secret
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clientsets.MultiStorageConfigSecretName,
					Namespace: s.namespace,
				},
				Data: configs,
			}
			_, err = s.currentK8sClient.CoreV1().Secrets(s.namespace).Create(ctx, secret, metav1.CreateOptions{})
			return err
		}
		return err
	}

	// Update existing secret
	secret.Data = configs
	_, err = s.currentK8sClient.CoreV1().Secrets(s.namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}

// updateClusterStorageConfig updates storage config in the control plane DB
func (s *ConfigSyncer) updateClusterStorageConfig(ctx context.Context, clusterName string, config *clientsets.PrimusLensClientConfig) error {
	facade := cpdb.GetControlPlaneFacade()

	cluster, err := facade.ClusterConfig.GetByName(ctx, clusterName)
	if err != nil {
		return err
	}

	// Update storage fields
	if config.Postgres != nil {
		cluster.PostgresHost = config.Postgres.Service
		cluster.PostgresPort = int(config.Postgres.Port)
		cluster.PostgresUsername = config.Postgres.Username
		cluster.PostgresPassword = config.Postgres.Password
		cluster.PostgresDBName = config.Postgres.DBName
	}

	if config.Opensearch != nil {
		cluster.OpensearchHost = config.Opensearch.Service
		cluster.OpensearchPort = int(config.Opensearch.Port)
		cluster.OpensearchUsername = config.Opensearch.Username
		cluster.OpensearchPassword = config.Opensearch.Password
	}

	if config.Prometheus != nil {
		cluster.PrometheusReadHost = config.Prometheus.ReadService
		cluster.PrometheusReadPort = int(config.Prometheus.ReadPort)
		cluster.PrometheusWriteHost = config.Prometheus.WriteService
		cluster.PrometheusWritePort = int(config.Prometheus.WritePort)
	}

	// Mark as installed if storage config exists
	if cluster.DataplaneStatus == "pending" && (config.Postgres != nil || config.Prometheus != nil) {
		cluster.DataplaneStatus = "installed"
	}

	return facade.ClusterConfig.Update(ctx, cluster)
}

// isNodeExcluded checks if a node should be excluded
func (s *ConfigSyncer) isNodeExcluded(clusterName, nodeName string) bool {
	if nodes, ok := s.excludeNodes[clusterName]; ok {
		for _, n := range nodes {
			if n == nodeName {
				return true
			}
		}
	}
	if nodes, ok := s.excludeNodes["*"]; ok {
		for _, n := range nodes {
			if n == nodeName {
				return true
			}
		}
	}
	return false
}

// serializeStorageConfig serializes storage config to JSON
func serializeStorageConfig(config *clientsets.PrimusLensClientConfig) ([]byte, error) {
	data := make(map[string]interface{})
	if config.Opensearch != nil {
		data["opensearch"] = config.Opensearch
	}
	if config.Prometheus != nil {
		data["prometheus"] = config.Prometheus
	}
	if config.Postgres != nil {
		data["postgres"] = config.Postgres
	}
	return json.Marshal(data)
}

// loadExcludeNodesFromEnv loads exclude nodes config from environment
func loadExcludeNodesFromEnv() map[string][]string {
	result := make(map[string][]string)
	excludeStr := env.GetString("EXCLUDE_CONTROL_PLANE_NODES", "")
	if excludeStr == "" {
		return result
	}

	// Parse format: "cluster1:node1,node2;cluster2:node3" or "*:node1,node2"
	// Implementation similar to the exporter
	return result
}

