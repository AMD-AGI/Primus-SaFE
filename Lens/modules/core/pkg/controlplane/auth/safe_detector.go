// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SafeDetector detects if Primus-SaFE is available
type SafeDetector struct {
	k8sClient  client.Client
	httpClient *http.Client
}

// SafeDetectionResult contains the result of SaFE detection
type SafeDetectionResult struct {
	AdapterDeployed      bool `json:"adapterDeployed"`
	SafeNamespaceExists  bool `json:"safeNamespaceExists"`
	SafeAPIReachable     bool `json:"safeApiReachable"`
	TokensAvailable      bool `json:"tokensAvailable"`
	ShouldEnableSafeMode bool `json:"shouldEnableSafeMode"`
}

// NewSafeDetector creates a new SafeDetector
func NewSafeDetector(k8sClient client.Client) *SafeDetector {
	return &SafeDetector{
		k8sClient: k8sClient,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// NewSafeDetectorWithoutK8s creates a SafeDetector without K8s client
// This is useful when running outside of a Kubernetes cluster
func NewSafeDetectorWithoutK8s() *SafeDetector {
	return &SafeDetector{
		k8sClient: nil,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// DetectSaFE checks if SaFE integration should be enabled
func (d *SafeDetector) DetectSaFE(ctx context.Context) (*SafeDetectionResult, error) {
	result := &SafeDetectionResult{}

	// 1. Check if safe-adapter is deployed (if K8s client is available)
	if d.k8sClient != nil {
		result.AdapterDeployed = d.checkAdapterDeployment(ctx)
		result.SafeNamespaceExists = d.checkSafeNamespace(ctx)
	}

	// 2. Check if primus-safe-apiserver is reachable
	result.SafeAPIReachable = d.checkSafeAPIServer(ctx)

	// 3. Check if SaFE tokens exist in Lens DB
	result.TokensAvailable = d.checkTokensInLensDB(ctx)

	// Determine if SaFE mode should be enabled
	// Enable if:
	// - Adapter is deployed AND Safe namespace exists AND (API reachable OR tokens available)
	// - OR tokens are already available (adapter might have synced before)
	if d.k8sClient != nil {
		result.ShouldEnableSafeMode = result.AdapterDeployed &&
			result.SafeNamespaceExists &&
			(result.SafeAPIReachable || result.TokensAvailable)
	} else {
		// Without K8s client, just check if tokens are available
		result.ShouldEnableSafeMode = result.TokensAvailable || result.SafeAPIReachable
	}

	log.Debugf("SaFE detection result: adapter=%v, namespace=%v, api=%v, tokens=%v, enable=%v",
		result.AdapterDeployed,
		result.SafeNamespaceExists,
		result.SafeAPIReachable,
		result.TokensAvailable,
		result.ShouldEnableSafeMode,
	)

	return result, nil
}

// checkAdapterDeployment checks if primus-safe-adapter deployment exists
func (d *SafeDetector) checkAdapterDeployment(ctx context.Context) bool {
	if d.k8sClient == nil {
		return false
	}

	deployment := &appsv1.Deployment{}
	err := d.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "primus-lens",
		Name:      "primus-safe-adapter",
	}, deployment)

	if err != nil {
		log.Debugf("Failed to get primus-safe-adapter deployment: %v", err)
		return false
	}

	return deployment.Status.ReadyReplicas > 0
}

// checkSafeNamespace checks if primus-safe namespace exists
func (d *SafeDetector) checkSafeNamespace(ctx context.Context) bool {
	if d.k8sClient == nil {
		return false
	}

	ns := &corev1.Namespace{}
	err := d.k8sClient.Get(ctx, types.NamespacedName{Name: "primus-safe"}, ns)

	if err != nil {
		log.Debugf("Failed to get primus-safe namespace: %v", err)
		return false
	}

	return true
}

// checkSafeAPIServer checks if primus-safe-apiserver is reachable
func (d *SafeDetector) checkSafeAPIServer(ctx context.Context) bool {
	// Try to reach the SaFE API server health endpoint
	// The URL depends on the deployment configuration
	urls := []string{
		"http://primus-safe-apiserver.primus-safe.svc:8080/healthz",
		"http://primus-safe-apiserver.primus-safe.svc.cluster.local:8080/healthz",
	}

	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}

		resp, err := d.httpClient.Do(req)
		if err != nil {
			log.Debugf("Failed to reach SaFE API at %s: %v", url, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Debugf("SaFE API reachable at %s", url)
			return true
		}
	}

	return false
}

// checkTokensInLensDB checks if synced tokens exist in Lens DB
func (d *SafeDetector) checkTokensInLensDB(ctx context.Context) bool {
	// Get Control Plane DB
	cm := clientsets.GetClusterManager()
	if !cm.IsControlPlaneEnabled() {
		return false
	}

	db := cm.GetControlPlaneDB()

	var count int64
	err := db.WithContext(ctx).
		Model(&model.LensSessions{}).
		Where("sync_source = ?", string(SyncSourceSafe)).
		Count(&count).Error

	if err != nil {
		log.Debugf("Failed to check tokens in Lens DB: %v", err)
		return false
	}

	return count > 0
}
