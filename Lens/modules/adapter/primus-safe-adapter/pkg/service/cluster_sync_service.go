// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterSyncService syncs cluster information from Primus-SaFE to control plane database
type ClusterSyncService struct {
	safeClient   client.Client
	syncInterval time.Duration
	autoInstall  bool
	profile      string
}

// ClusterSyncServiceOption is a functional option for ClusterSyncService
type ClusterSyncServiceOption func(*ClusterSyncService)

// WithClusterSyncInterval sets the sync interval
func WithClusterSyncInterval(interval time.Duration) ClusterSyncServiceOption {
	return func(s *ClusterSyncService) {
		s.syncInterval = interval
	}
}

// WithAutoInstall enables auto-installation of dataplane
func WithAutoInstall(autoInstall bool) ClusterSyncServiceOption {
	return func(s *ClusterSyncService) {
		s.autoInstall = autoInstall
	}
}

// WithDefaultProfile sets the default install profile
func WithDefaultProfile(profile string) ClusterSyncServiceOption {
	return func(s *ClusterSyncService) {
		s.profile = profile
	}
}

// NewClusterSyncService creates a new ClusterSyncService
func NewClusterSyncService(safeClient client.Client, opts ...ClusterSyncServiceOption) *ClusterSyncService {
	s := &ClusterSyncService{
		safeClient:   safeClient,
		syncInterval: 60 * time.Second,
		autoInstall:  false,
		profile:      "minimal",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NewClusterSyncServiceFromConfig creates a ClusterSyncService from config
func NewClusterSyncServiceFromConfig(safeClient client.Client, cfg *config.PrimusSafeSyncConfig) *ClusterSyncService {
	if cfg == nil {
		return NewClusterSyncService(safeClient)
	}
	return NewClusterSyncService(safeClient,
		WithClusterSyncInterval(cfg.GetSyncInterval()),
		WithAutoInstall(cfg.AutoInstall),
		WithDefaultProfile(cfg.GetDefaultProfile()),
	)
}

// Run starts the cluster sync service
func (s *ClusterSyncService) Run(ctx context.Context) error {
	log.Info("Starting cluster sync service")

	// Initial sync
	if err := s.syncClusters(ctx); err != nil {
		log.Errorf("Initial cluster sync failed: %v", err)
		// Don't return error, continue with periodic sync
	}

	// Periodic sync
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.syncClusters(ctx); err != nil {
				log.Errorf("Cluster sync failed: %v", err)
			}
		case <-ctx.Done():
			log.Info("Stopping cluster sync service")
			return nil
		}
	}
}

// syncClusters syncs all clusters from Primus-SaFE
func (s *ClusterSyncService) syncClusters(ctx context.Context) error {
	log.Debug("Starting cluster sync from Primus-SaFE")

	// List all clusters from Primus-SaFE
	clusterList := &primusSafeV1.ClusterList{}
	if err := s.safeClient.List(ctx, clusterList); err != nil {
		return fmt.Errorf("failed to list primus-safe clusters: %w", err)
	}

	log.Infof("Found %d clusters in Primus-SaFE", len(clusterList.Items))

	// Track synced cluster IDs
	syncedClusterIDs := make(map[string]bool)

	// Sync each cluster
	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]

		// Only sync Ready clusters
		if !cluster.IsReady() {
			log.Debugf("Skipping cluster %s: not ready (phase=%s)", 
				cluster.Name, cluster.Status.ControlPlaneStatus.Phase)
			continue
		}

		syncedClusterIDs[cluster.Name] = true

		if err := s.syncCluster(ctx, cluster); err != nil {
			log.Errorf("Failed to sync cluster %s: %v", cluster.Name, err)
			continue
		}
	}

	// Mark deleted clusters (clusters in DB but not in Primus-SaFE)
	if err := s.markDeletedClusters(ctx, syncedClusterIDs); err != nil {
		log.Errorf("Failed to mark deleted clusters: %v", err)
	}

	log.Debugf("Cluster sync completed: synced %d clusters", len(syncedClusterIDs))
	return nil
}

// syncCluster syncs a single cluster to the control plane database
func (s *ClusterSyncService) syncCluster(ctx context.Context, cluster *primusSafeV1.Cluster) error {
	facade := cpdb.GetControlPlaneFacade()

	// Check if cluster already exists
	existing, err := facade.GetClusterConfig().GetByPrimusSafeID(ctx, cluster.Name)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing cluster: %w", err)
	}

	if existing == nil {
		// Create new cluster config
		return s.createClusterConfig(ctx, cluster)
	}

	// Update existing cluster config if K8S connection changed
	return s.updateClusterConfig(ctx, existing, cluster)
}

// createClusterConfig creates a new cluster config from Primus-SaFE cluster
func (s *ClusterSyncService) createClusterConfig(ctx context.Context, cluster *primusSafeV1.Cluster) error {
	facade := cpdb.GetControlPlaneFacade()

	config := &model.ClusterConfig{
		ClusterName:     cluster.Name,
		DisplayName:     cluster.Labels[primusSafeV1.DisplayNameLabel],
		Source:          model.ClusterSourcePrimusSafe,
		PrimusSafeID:    cluster.Name,
		Status:          model.ClusterStatusActive,
		DataplaneStatus: model.DataplaneStatusPending,

		// K8S connection from cluster status (prioritize service endpoint over CR endpoint)
		K8SEndpoint: s.buildEndpoint(ctx, cluster),
		K8SCAData:   cluster.Status.ControlPlaneStatus.CAData,
		K8SCertData: cluster.Status.ControlPlaneStatus.CertData,
		K8SKeyData:  cluster.Status.ControlPlaneStatus.KeyData,
	}

	if err := facade.GetClusterConfig().Create(ctx, config); err != nil {
		return fmt.Errorf("failed to create cluster config: %w", err)
	}

	log.Infof("Synced new cluster from Primus-SaFE: %s", cluster.Name)

	// Trigger dataplane installation if auto-install is enabled
	if s.autoInstall {
		go s.triggerDataplaneInstall(context.Background(), config.ClusterName)
	}

	return nil
}

// updateClusterConfig updates an existing cluster config
func (s *ClusterSyncService) updateClusterConfig(ctx context.Context, existing *model.ClusterConfig, cluster *primusSafeV1.Cluster) error {
	// Skip K8S config update if in manual mode
	if existing.K8SManualMode {
		log.Debugf("Skipping K8S config update for cluster %s: K8S manual mode enabled", existing.ClusterName)
		// Still update display name if changed
		if displayName := cluster.Labels[primusSafeV1.DisplayNameLabel]; displayName != "" && displayName != existing.DisplayName {
			existing.DisplayName = displayName
			facade := cpdb.GetControlPlaneFacade()
			if err := facade.GetClusterConfig().Update(ctx, existing); err != nil {
				return fmt.Errorf("failed to update display name: %w", err)
			}
		}
		return nil
	}

	// Check if K8S connection changed
	endpoint := s.buildEndpoint(ctx, cluster)
	if existing.K8SEndpoint == endpoint &&
		existing.K8SCAData == cluster.Status.ControlPlaneStatus.CAData &&
		existing.K8SCertData == cluster.Status.ControlPlaneStatus.CertData &&
		existing.K8SKeyData == cluster.Status.ControlPlaneStatus.KeyData {
		// No changes
		return nil
	}

	facade := cpdb.GetControlPlaneFacade()

	// Update K8S connection
	existing.K8SEndpoint = endpoint
	existing.K8SCAData = cluster.Status.ControlPlaneStatus.CAData
	existing.K8SCertData = cluster.Status.ControlPlaneStatus.CertData
	existing.K8SKeyData = cluster.Status.ControlPlaneStatus.KeyData

	// Update display name if changed
	if displayName := cluster.Labels[primusSafeV1.DisplayNameLabel]; displayName != "" {
		existing.DisplayName = displayName
	}

	if err := facade.GetClusterConfig().Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update cluster config: %w", err)
	}

	log.Infof("Updated cluster config from Primus-SaFE: %s", cluster.Name)
	return nil
}

// buildEndpoint builds the K8S API endpoint from cluster status
// Priority: 1. Service in primus-safe namespace with same name as cluster
//           2. Endpoints from cluster CR status
func (s *ClusterSyncService) buildEndpoint(ctx context.Context, cluster *primusSafeV1.Cluster) string {
	// Priority 1: Try to get endpoint from Service in primus-safe namespace
	endpoint := s.getEndpointFromService(ctx, cluster.Name)
	if endpoint != "" {
		log.Debugf("Using endpoint from Service for cluster %s: %s", cluster.Name, endpoint)
		return endpoint
	}

	// Priority 2: Fallback to CR endpoints
	if len(cluster.Status.ControlPlaneStatus.Endpoints) > 0 {
		endpoint := cluster.Status.ControlPlaneStatus.Endpoints[0]
		// Ensure the endpoint has https:// prefix
		if endpoint != "" && endpoint[0] != 'h' {
			endpoint = "https://" + endpoint
		}
		log.Debugf("Using endpoint from CR for cluster %s: %s", cluster.Name, endpoint)
		return endpoint
	}
	return ""
}

// getEndpointFromService tries to get the K8S API endpoint from a Service
// in primus-safe namespace with the same name as the cluster
func (s *ClusterSyncService) getEndpointFromService(ctx context.Context, clusterName string) string {
	// The service should be in primus-safe namespace with the same name as the cluster
	svc := &corev1.Service{}
	err := s.safeClient.Get(ctx, types.NamespacedName{
		Namespace: "primus-safe",
		Name:      clusterName,
	}, svc)

	if err != nil {
		// Service not found or error, this is normal for clusters without proxy service
		log.Debugf("No service found for cluster %s in primus-safe namespace: %v", clusterName, err)
		return ""
	}

	// Find the HTTPS port (usually 443 or 6443)
	port := s.getServicePort(svc)
	if port == 0 {
		log.Debugf("Service %s has no suitable port", clusterName)
		return ""
	}

	// Priority 1: For LoadBalancer type, use the external IP from status
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			ingress := svc.Status.LoadBalancer.Ingress[0]
			if ingress.IP != "" {
				endpoint := fmt.Sprintf("https://%s:%d", ingress.IP, port)
				log.Debugf("Using LoadBalancer IP for cluster %s: %s", clusterName, endpoint)
				return endpoint
			}
			if ingress.Hostname != "" {
				endpoint := fmt.Sprintf("https://%s:%d", ingress.Hostname, port)
				log.Debugf("Using LoadBalancer hostname for cluster %s: %s", clusterName, endpoint)
				return endpoint
			}
		}
		log.Debugf("LoadBalancer service %s has no ingress IP/hostname yet", clusterName)
	}

	// Priority 2: For NodePort type, get endpoint from Endpoints resource (actual pod IPs)
	if svc.Spec.Type == corev1.ServiceTypeNodePort || svc.Spec.Type == corev1.ServiceTypeClusterIP {
		endpointIP := s.getEndpointIP(ctx, clusterName)
		if endpointIP != "" {
			// For NodePort, we need to use the target port (actual API server port)
			targetPort := s.getTargetPort(svc, port)
			endpoint := fmt.Sprintf("https://%s:%d", endpointIP, targetPort)
			log.Debugf("Using Endpoints IP for cluster %s: %s", clusterName, endpoint)
			return endpoint
		}
	}

	// No suitable endpoint found from Service
	log.Debugf("No suitable endpoint found from Service for cluster %s", clusterName)
	return ""
}

// getServicePort finds the HTTPS port from a Service
func (s *ClusterSyncService) getServicePort(svc *corev1.Service) int32 {
	for _, p := range svc.Spec.Ports {
		if p.Name == "https" || p.Port == 443 || p.Port == 6443 {
			return p.Port
		}
	}
	// If no HTTPS port found, use the first port
	if len(svc.Spec.Ports) > 0 {
		return svc.Spec.Ports[0].Port
	}
	return 0
}

// getTargetPort gets the target port for a given service port
func (s *ClusterSyncService) getTargetPort(svc *corev1.Service, servicePort int32) int32 {
	for _, p := range svc.Spec.Ports {
		if p.Port == servicePort {
			if p.TargetPort.IntVal != 0 {
				return p.TargetPort.IntVal
			}
			// If targetPort is not set or is a named port, use the service port
			return servicePort
		}
	}
	return servicePort
}

// getEndpointIP gets the first ready endpoint IP for a service
func (s *ClusterSyncService) getEndpointIP(ctx context.Context, serviceName string) string {
	endpoints := &corev1.Endpoints{}
	err := s.safeClient.Get(ctx, types.NamespacedName{
		Namespace: "primus-safe",
		Name:      serviceName,
	}, endpoints)

	if err != nil {
		log.Debugf("Failed to get endpoints for service %s: %v", serviceName, err)
		return ""
	}

	// Find the first ready address
	for _, subset := range endpoints.Subsets {
		if len(subset.Addresses) > 0 {
			return subset.Addresses[0].IP
		}
	}

	log.Debugf("No ready endpoints found for service %s", serviceName)
	return ""
}

// markDeletedClusters marks clusters as deleted if they no longer exist in Primus-SaFE
func (s *ClusterSyncService) markDeletedClusters(ctx context.Context, syncedClusterIDs map[string]bool) error {
	facade := cpdb.GetControlPlaneFacade()

	// Get all clusters synced from Primus-SaFE
	configs, err := facade.GetClusterConfig().ListBySource(ctx, model.ClusterSourcePrimusSafe)
	if err != nil {
		return fmt.Errorf("failed to list primus-safe clusters from DB: %w", err)
	}

	for _, config := range configs {
		if !syncedClusterIDs[config.PrimusSafeID] {
			// Cluster no longer exists in Primus-SaFE, mark as deleted
			log.Infof("Marking cluster as deleted (no longer in Primus-SaFE): %s", config.ClusterName)
			if err := facade.GetClusterConfig().Delete(ctx, config.ClusterName); err != nil {
				log.Errorf("Failed to mark cluster %s as deleted: %v", config.ClusterName, err)
			}
		}
	}

	return nil
}

// triggerDataplaneInstall triggers dataplane installation for a cluster
func (s *ClusterSyncService) triggerDataplaneInstall(ctx context.Context, clusterName string) {
	// TODO: Implement actual installation in Phase 5
	// For now, just log
	log.Infof("Auto-install dataplane triggered for cluster: %s (profile=%s)", clusterName, s.profile)
}

// Scheduler interface implementation

// Name returns the service name
func (s *ClusterSyncService) Name() string {
	return "ClusterSyncService"
}
