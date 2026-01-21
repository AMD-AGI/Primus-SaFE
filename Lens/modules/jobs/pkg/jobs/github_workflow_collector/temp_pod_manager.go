// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package github_workflow_collector

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	// TempPodPrefix is the prefix for temporary pod names
	TempPodPrefix = "lens-pvc-reader-"
	// TempPodImage is the image used for temporary pods
	TempPodImage = "busybox:latest"
	// TempPodTimeout is the timeout for waiting for pod to be ready
	TempPodTimeout = 2 * time.Minute
	// TempPodTTL is the time-to-live for temporary pods
	TempPodTTL = 10 * time.Minute
	// TempPodLabel is the label used to identify temporary pods
	TempPodLabel = "primus-lens.amd.com/temp-pvc-reader"
)

// AutoscalingRunnerSet GVR
var arsGVR = schema.GroupVersionResource{
	Group:    "actions.github.com",
	Version:  "v1alpha1",
	Resource: "autoscalingrunnersets",
}

// TempPodManager manages temporary pods for reading PVC files
type TempPodManager struct {
	k8sClient     kubernetes.Interface
	dynamicClient dynamic.Interface
}

// NewTempPodManager creates a new TempPodManager
func NewTempPodManager() *TempPodManager {
	clients := clientsets.GetClusterManager().GetCurrentClusterClients()
	if clients == nil || clients.K8SClientSet == nil {
		log.Warn("TempPodManager: cluster clients not available")
		return nil
	}

	return &TempPodManager{
		k8sClient:     clients.K8SClientSet.Clientsets,
		dynamicClient: clients.K8SClientSet.Dynamic,
	}
}

// VolumeInfo contains volume configuration extracted from AutoscalingRunnerSet
type VolumeInfo struct {
	Volumes      []corev1.Volume
	VolumeMounts []corev1.VolumeMount
}

// GetVolumeInfoFromARS gets volume configuration from AutoscalingRunnerSet template
func (m *TempPodManager) GetVolumeInfoFromARS(ctx context.Context, namespace, name string) (*VolumeInfo, error) {
	if m.dynamicClient == nil {
		return nil, fmt.Errorf("dynamic client not available")
	}

	// Get AutoscalingRunnerSet
	ars, err := m.dynamicClient.Resource(arsGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get AutoscalingRunnerSet %s/%s: %w", namespace, name, err)
	}

	// Extract template spec from ARS
	// AutoscalingRunnerSet has spec.template.spec which contains volumes and containers
	templateSpec, found, err := unstructured.NestedMap(ars.Object, "spec", "template", "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("failed to get template spec from ARS: spec.template.spec not found")
	}

	// Parse volumes
	volumesRaw, _, _ := unstructured.NestedSlice(templateSpec, "volumes")
	volumes, err := parseVolumes(volumesRaw)
	if err != nil {
		log.Warnf("TempPodManager: failed to parse volumes: %v", err)
	}

	// Parse volume mounts from first container
	containersRaw, _, _ := unstructured.NestedSlice(templateSpec, "containers")
	volumeMounts, err := parseVolumeMounts(containersRaw)
	if err != nil {
		log.Warnf("TempPodManager: failed to parse volume mounts: %v", err)
	}

	// Filter to only PVC-backed volumes
	pvcVolumes, pvcMounts := filterPVCVolumes(volumes, volumeMounts)

	if len(pvcVolumes) == 0 {
		return nil, fmt.Errorf("no PVC-backed volumes found in AutoscalingRunnerSet template")
	}

	log.Infof("TempPodManager: found %d PVC volumes in ARS %s/%s", len(pvcVolumes), namespace, name)

	return &VolumeInfo{
		Volumes:      pvcVolumes,
		VolumeMounts: pvcMounts,
	}, nil
}

// CreateTempPod creates a temporary pod to read PVC files
func (m *TempPodManager) CreateTempPod(ctx context.Context, config *model.GithubWorkflowConfigs, runID int64, volumeInfo *VolumeInfo) (*PodInfo, error) {
	if m.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not available")
	}

	podName := fmt.Sprintf("%s%d-%d", TempPodPrefix, config.ID, runID)
	namespace := config.RunnerSetNamespace

	// Check if pod already exists
	existingPod, err := m.k8sClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err == nil && existingPod != nil {
		// Pod exists, wait for it to be ready
		log.Infof("TempPodManager: temp pod %s/%s already exists, waiting for ready", namespace, podName)
		if err := m.waitForPodReady(ctx, namespace, podName); err != nil {
			return nil, fmt.Errorf("existing pod not ready: %w", err)
		}
		return m.buildPodInfoFromPod(existingPod)
	}

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check existing pod: %w", err)
	}

	// Build pod spec
	pod := m.buildTempPodSpec(podName, namespace, config.ID, runID, volumeInfo)

	// Create the pod
	_, err = m.k8sClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Race condition - another process created it
			log.Infof("TempPodManager: temp pod %s/%s created by another process", namespace, podName)
		} else {
			return nil, fmt.Errorf("failed to create temp pod: %w", err)
		}
	}

	log.Infof("TempPodManager: created temp pod %s/%s", namespace, podName)

	// Wait for pod to be ready
	if err := m.waitForPodReady(ctx, namespace, podName); err != nil {
		// Clean up failed pod
		_ = m.DeleteTempPod(ctx, namespace, podName)
		return nil, fmt.Errorf("temp pod not ready: %w", err)
	}

	// Re-fetch pod to get updated status (including NodeName after scheduling)
	readyPod, err := m.k8sClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		_ = m.DeleteTempPod(ctx, namespace, podName)
		return nil, fmt.Errorf("failed to get ready pod: %w", err)
	}

	return m.buildPodInfoFromPod(readyPod)
}

// DeleteTempPod deletes a temporary pod
func (m *TempPodManager) DeleteTempPod(ctx context.Context, namespace, name string) error {
	if m.k8sClient == nil {
		return fmt.Errorf("kubernetes client not available")
	}

	gracePeriod := int64(0) // Delete immediately
	err := m.k8sClient.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete temp pod %s/%s: %w", namespace, name, err)
	}

	log.Infof("TempPodManager: deleted temp pod %s/%s", namespace, name)
	return nil
}

// buildTempPodSpec builds a temporary pod spec for reading PVC files
func (m *TempPodManager) buildTempPodSpec(name, namespace string, configID, runID int64, volumeInfo *VolumeInfo) *corev1.Pod {
	labels := map[string]string{
		TempPodLabel:                    "true",
		"primus-lens.amd.com/config-id": fmt.Sprintf("%d", configID),
		"primus-lens.amd.com/run-id":    fmt.Sprintf("%d", runID),
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"primus-lens.amd.com/purpose":    "pvc-file-reader",
				"primus-lens.amd.com/created-at": time.Now().Format(time.RFC3339),
				"primus-lens.amd.com/expires-at": time.Now().Add(TempPodTTL).Format(time.RFC3339),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "reader",
					Image:   TempPodImage,
					Command: []string{"sleep", "3600"}, // Sleep for 1 hour (will be deleted earlier)
					VolumeMounts: volumeInfo.VolumeMounts,
				},
			},
			Volumes: volumeInfo.Volumes,
			// Only tolerate specific taints, NOT NotReady/Unreachable nodes
			// This ensures the pod won't be scheduled to unhealthy nodes
		},
	}
}

// waitForPodReady waits for a pod to be ready
func (m *TempPodManager) waitForPodReady(ctx context.Context, namespace, name string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, TempPodTimeout, true, func(ctx context.Context) (bool, error) {
		pod, err := m.k8sClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil // Keep waiting
			}
			return false, err
		}

		switch pod.Status.Phase {
		case corev1.PodRunning:
			return true, nil
		case corev1.PodSucceeded, corev1.PodFailed:
			return false, fmt.Errorf("pod terminated with phase %s", pod.Status.Phase)
		default:
			return false, nil // Keep waiting
		}
	})
}

// buildPodInfoFromPod builds PodInfo from a Kubernetes Pod
func (m *TempPodManager) buildPodInfoFromPod(pod *corev1.Pod) (*PodInfo, error) {
	if pod.Spec.NodeName == "" {
		return nil, fmt.Errorf("pod not scheduled to a node yet")
	}

	containerName := ""
	if len(pod.Spec.Containers) > 0 {
		containerName = pod.Spec.Containers[0].Name
	}

	return &PodInfo{
		UID:           string(pod.UID),
		Name:          pod.Name,
		Namespace:     pod.Namespace,
		NodeName:      pod.Spec.NodeName,
		ContainerName: containerName,
	}, nil
}

// parseVolumes parses volumes from unstructured data
func parseVolumes(volumesRaw []interface{}) ([]corev1.Volume, error) {
	if len(volumesRaw) == 0 {
		return nil, nil
	}

	data, err := json.Marshal(volumesRaw)
	if err != nil {
		return nil, err
	}

	var volumes []corev1.Volume
	if err := json.Unmarshal(data, &volumes); err != nil {
		return nil, err
	}

	return volumes, nil
}

// parseVolumeMounts parses volume mounts from containers spec
func parseVolumeMounts(containersRaw []interface{}) ([]corev1.VolumeMount, error) {
	if len(containersRaw) == 0 {
		return nil, nil
	}

	// Get the first container
	firstContainer, ok := containersRaw[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid container format")
	}

	volumeMountsRaw, found := firstContainer["volumeMounts"]
	if !found {
		return nil, nil
	}

	volumeMountsList, ok := volumeMountsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid volumeMounts format")
	}

	data, err := json.Marshal(volumeMountsList)
	if err != nil {
		return nil, err
	}

	var volumeMounts []corev1.VolumeMount
	if err := json.Unmarshal(data, &volumeMounts); err != nil {
		return nil, err
	}

	return volumeMounts, nil
}

// filterPVCVolumes filters volumes to only include PVC-backed volumes
func filterPVCVolumes(volumes []corev1.Volume, mounts []corev1.VolumeMount) ([]corev1.Volume, []corev1.VolumeMount) {
	pvcVolumeNames := make(map[string]bool)
	var pvcVolumes []corev1.Volume

	for _, vol := range volumes {
		// Include PVC volumes and any volume that might contain results
		if vol.PersistentVolumeClaim != nil {
			pvcVolumeNames[vol.Name] = true
			pvcVolumes = append(pvcVolumes, vol)
		}
	}

	var pvcMounts []corev1.VolumeMount
	for _, mount := range mounts {
		if pvcVolumeNames[mount.Name] {
			pvcMounts = append(pvcMounts, mount)
		}
	}

	return pvcVolumes, pvcMounts
}

// CleanupExpiredTempPods cleans up temporary pods that have expired
func (m *TempPodManager) CleanupExpiredTempPods(ctx context.Context) error {
	if m.k8sClient == nil {
		return fmt.Errorf("kubernetes client not available")
	}

	// List all temp pods
	labelSelector := fmt.Sprintf("%s=true", TempPodLabel)
	pods, err := m.k8sClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list temp pods: %w", err)
	}

	now := time.Now()
	for _, pod := range pods.Items {
		expiresAtStr := pod.Annotations["primus-lens.amd.com/expires-at"]
		if expiresAtStr == "" {
			// No expiry annotation, delete if older than TTL
			if now.Sub(pod.CreationTimestamp.Time) > TempPodTTL {
				log.Infof("TempPodManager: cleaning up old temp pod %s/%s", pod.Namespace, pod.Name)
				_ = m.DeleteTempPod(ctx, pod.Namespace, pod.Name)
			}
			continue
		}

		expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
		if err != nil {
			continue
		}

		if now.After(expiresAt) {
			log.Infof("TempPodManager: cleaning up expired temp pod %s/%s", pod.Namespace, pod.Name)
			_ = m.DeleteTempPod(ctx, pod.Namespace, pod.Name)
		}
	}

	return nil
}

