// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/k8sUtil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubeSystemNamespace   = "kube-system"
	labelK8sApp           = "k8s-app"
	labelCoreDNS          = "kube-dns"
	labelNodeLocalDNS     = "node-local-dns"
	labelPrimusSafeAppName  = "primus-safe-app-name"
	labelPrimusLensAppName  = "primus-lens-app-name"
)

// KubeSystemProbePodItem represents one pod in kube-system probe result.
type KubeSystemProbePodItem struct {
	Name  string `json:"name"`
	Node  string `json:"node"`
	Ready bool   `json:"ready"`
}

// KubeSystemProbeComponentItem represents one kube-system component (e.g. coredns, node-local-dns).
type KubeSystemProbeComponentItem struct {
	Name        string                    `json:"name"`
	DisplayName string                    `json:"displayName"`
	Kind        string                    `json:"kind"`
	Desired     int32                     `json:"desired"`
	Ready       int32                     `json:"ready"`
	Healthy     bool                      `json:"healthy"`
	Pods        []KubeSystemProbePodItem  `json:"pods,omitempty"`
}

// PlatformComponentPodItem represents one pod in platform component probe result.
type PlatformComponentPodItem struct {
	Name  string `json:"name"`
	Node  string `json:"node"`
	Ready bool   `json:"ready"`
}

// PlatformComponentItem represents one platform component (Primus-SaFE or Primus-Lens).
type PlatformComponentItem struct {
	AppName   string                     `json:"appName"`
	Namespace string                     `json:"namespace"`
	Total     int                        `json:"total"`
	Ready     int                        `json:"ready"`
	Healthy   bool                       `json:"healthy"`
	Pods      []PlatformComponentPodItem `json:"pods,omitempty"`
}

func probeCoreDNS(ctx context.Context, c client.Client, clusterName string) (KubeSystemProbeComponentItem, error) {
	item := KubeSystemProbeComponentItem{
		Name: "coredns", DisplayName: "CoreDNS", Kind: "Deployment",
		Desired: 0, Ready: 0, Healthy: false, Pods: nil,
	}
	var deployList appsv1.DeploymentList
	if err := c.List(ctx, &deployList, client.InNamespace(kubeSystemNamespace), client.MatchingLabels{labelK8sApp: labelCoreDNS}); err != nil || len(deployList.Items) == 0 {
		return item, err
	}
	deploy := &deployList.Items[0]
	if deploy.Spec.Replicas != nil {
		item.Desired = *deploy.Spec.Replicas
	}
	var podList corev1.PodList
	if err := c.List(ctx, &podList, client.InNamespace(kubeSystemNamespace), client.MatchingLabels{labelK8sApp: labelCoreDNS}); err != nil {
		return item, err
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		ready := k8sUtil.IsPodRunning(pod)
		if ready {
			item.Ready++
		}
		item.Pods = append(item.Pods, KubeSystemProbePodItem{Name: pod.Name, Node: pod.Spec.NodeName, Ready: ready})
	}
	item.Healthy = item.Desired > 0 && item.Ready == item.Desired
	return item, nil
}

func probeNodeLocalDNS(ctx context.Context, c client.Client, clusterName string) (KubeSystemProbeComponentItem, error) {
	item := KubeSystemProbeComponentItem{
		Name: "node-local-dns", DisplayName: "NodeLocal DNS", Kind: "DaemonSet",
		Desired: 0, Ready: 0, Healthy: false, Pods: nil,
	}
	var dsList appsv1.DaemonSetList
	if err := c.List(ctx, &dsList, client.InNamespace(kubeSystemNamespace), client.MatchingLabels{labelK8sApp: labelNodeLocalDNS}); err != nil || len(dsList.Items) == 0 {
		return item, err
	}
	ds := &dsList.Items[0]
	item.Desired = ds.Status.DesiredNumberScheduled
	var podList corev1.PodList
	if err := c.List(ctx, &podList, client.InNamespace(kubeSystemNamespace), client.MatchingLabels{labelK8sApp: labelNodeLocalDNS}); err != nil {
		return item, err
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		ready := k8sUtil.IsPodRunning(pod)
		if ready {
			item.Ready++
		}
		item.Pods = append(item.Pods, KubeSystemProbePodItem{Name: pod.Name, Node: pod.Spec.NodeName, Ready: ready})
	}
	item.Healthy = item.Desired > 0 && item.Ready == item.Desired
	return item, nil
}

type componentKey struct {
	namespace string
	appName   string
}

func listComponentsByLabel(ctx context.Context, c client.Client, labelKey string) ([]PlatformComponentItem, error) {
	var podList corev1.PodList
	if err := c.List(ctx, &podList); err != nil {
		return nil, err
	}
	var filtered []corev1.Pod
	for i := range podList.Items {
		if appName := podList.Items[i].Labels[labelKey]; appName != "" {
			filtered = append(filtered, podList.Items[i])
		}
	}
	return groupPodsToComponents(filtered, labelKey), nil
}

func groupPodsToComponents(pods []corev1.Pod, labelKey string) []PlatformComponentItem {
	group := make(map[componentKey][]corev1.Pod)
	for i := range pods {
		pod := &pods[i]
		appName := pod.Labels[labelKey]
		if appName == "" {
			continue
		}
		k := componentKey{namespace: pod.Namespace, appName: appName}
		group[k] = append(group[k], *pod)
	}
	out := make([]PlatformComponentItem, 0, len(group))
	for k, list := range group {
		var total, ready int
		var podItems []PlatformComponentPodItem
		for i := range list {
			pod := &list[i]
			total++
			r := k8sUtil.IsPodRunning(pod)
			if r {
				ready++
			}
			podItems = append(podItems, PlatformComponentPodItem{Name: pod.Name, Node: pod.Spec.NodeName, Ready: r})
		}
		healthy := ready >= 1
		out = append(out, PlatformComponentItem{
			AppName: k.appName, Namespace: k.namespace,
			Total: total, Ready: ready, Healthy: healthy, Pods: podItems,
		})
	}
	return out
}
