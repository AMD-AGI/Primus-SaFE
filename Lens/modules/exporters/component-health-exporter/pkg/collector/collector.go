// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"
	"os"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	namespaceKubeSystem       = "kube-system"
	labelK8sApp              = "k8s-app"
	labelCoreDNS             = "kube-dns"
	labelNodeLocalDNS        = "node-local-dns"
	labelPrimusLensAppName   = "primus-lens-app-name"
	labelPrimusSafeAppName   = "primus-safe-app-name"
	platformKubeSystem       = "kube_system"
	platformPrimusLens       = "primus_lens"
	platformPrimusSafe       = "primus_safe"
)

// ComponentHealthCollector implements prometheus.Collector; on each scrape it probes
// workload controllers (Deployment/DaemonSet/StatefulSet) and emits primus_component_*
// metrics. No goroutine: probe runs synchronously in Collect().
type ComponentHealthCollector struct {
	descHealthy       *prometheus.Desc
	descReplicasDesired *prometheus.Desc
	descReplicasReady   *prometheus.Desc
}

// NewComponentHealthCollector returns a new ComponentHealthCollector.
func NewComponentHealthCollector() *ComponentHealthCollector {
	return &ComponentHealthCollector{
		descHealthy: prometheus.NewDesc(
			"primus_component_healthy",
			"1 if component is healthy (desired > 0 && ready == desired), 0 otherwise",
			[]string{"platform", "app_name", "namespace", "kind", "cluster"},
			nil,
		),
		descReplicasDesired: prometheus.NewDesc(
			"primus_component_replicas_desired",
			"Desired number of replicas for the component",
			[]string{"platform", "app_name", "namespace", "kind", "cluster"},
			nil,
		),
		descReplicasReady: prometheus.NewDesc(
			"primus_component_replicas_ready",
			"Number of ready replicas for the component",
			[]string{"platform", "app_name", "namespace", "kind", "cluster"},
			nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *ComponentHealthCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.descHealthy
	ch <- c.descReplicasDesired
	ch <- c.descReplicasReady
}

// Collect implements prometheus.Collector. It runs the probe and emits metrics for
// components found in this scrape only (no stale series).
func (c *ComponentHealthCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault("")
	if err != nil || clients == nil || clients.K8SClientSet == nil || clients.K8SClientSet.ControllerRuntimeClient == nil {
		return
	}
	k8sClient := clients.K8SClientSet.ControllerRuntimeClient
	clusterName := getClusterName(clients.ClusterName)

	// Kube-system: CoreDNS (Deployment), NodeLocal DNS (DaemonSet)
	collectKubeSystem(ctx, ch, c, k8sClient, clusterName)

	// Platform: primus-lens-app-name and primus-safe-app-name (Deployment, DaemonSet, StatefulSet)
	collectPlatformByLabel(ctx, ch, c, k8sClient, clusterName, labelPrimusLensAppName, platformPrimusLens)
	collectPlatformByLabel(ctx, ch, c, k8sClient, clusterName, labelPrimusSafeAppName, platformPrimusSafe)
}

func getClusterName(fallback string) string {
	if s := os.Getenv("CLUSTER_NAME"); s != "" {
		return s
	}
	return fallback
}

func collectKubeSystem(ctx context.Context, ch chan<- prometheus.Metric, c *ComponentHealthCollector, k8sClient client.Client, clusterName string) {
	// CoreDNS
	var deployList appsv1.DeploymentList
	if err := k8sClient.List(ctx, &deployList, client.InNamespace(namespaceKubeSystem), client.MatchingLabels{labelK8sApp: labelCoreDNS}); err == nil && len(deployList.Items) > 0 {
		d := &deployList.Items[0]
		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		ready := d.Status.ReadyReplicas
		healthy := desired > 0 && ready == desired
		v := 0.0
		if healthy {
			v = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.descHealthy, prometheus.GaugeValue, v, platformKubeSystem, "coredns", namespaceKubeSystem, "Deployment", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasDesired, prometheus.GaugeValue, float64(desired), platformKubeSystem, "coredns", namespaceKubeSystem, "Deployment", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasReady, prometheus.GaugeValue, float64(ready), platformKubeSystem, "coredns", namespaceKubeSystem, "Deployment", clusterName)
	}

	// NodeLocal DNS
	var dsList appsv1.DaemonSetList
	if err := k8sClient.List(ctx, &dsList, client.InNamespace(namespaceKubeSystem), client.MatchingLabels{labelK8sApp: labelNodeLocalDNS}); err == nil && len(dsList.Items) > 0 {
		ds := &dsList.Items[0]
		desired := ds.Status.DesiredNumberScheduled
		ready := ds.Status.NumberReady
		healthy := desired > 0 && ready == desired
		v := 0.0
		if healthy {
			v = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.descHealthy, prometheus.GaugeValue, v, platformKubeSystem, "node-local-dns", namespaceKubeSystem, "DaemonSet", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasDesired, prometheus.GaugeValue, float64(desired), platformKubeSystem, "node-local-dns", namespaceKubeSystem, "DaemonSet", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasReady, prometheus.GaugeValue, float64(ready), platformKubeSystem, "node-local-dns", namespaceKubeSystem, "DaemonSet", clusterName)
	}
}

func collectPlatformByLabel(ctx context.Context, ch chan<- prometheus.Metric, c *ComponentHealthCollector, k8sClient client.Client, clusterName, labelKey, platform string) {
	opts := []client.ListOption{client.HasLabels{labelKey}}

	// Deployments
	var deployList appsv1.DeploymentList
	if err := k8sClient.List(ctx, &deployList, opts...); err != nil {
		log.Warnf("component-health-exporter: list Deployments with %s: %v", labelKey, err)
		return
	}
	for i := range deployList.Items {
		d := &deployList.Items[i]
		appName := d.Labels[labelKey]
		if appName == "" {
			continue
		}
		ns := d.Namespace
		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		ready := d.Status.ReadyReplicas
		healthy := desired > 0 && ready == desired
		v := 0.0
		if healthy {
			v = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.descHealthy, prometheus.GaugeValue, v, platform, appName, ns, "Deployment", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasDesired, prometheus.GaugeValue, float64(desired), platform, appName, ns, "Deployment", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasReady, prometheus.GaugeValue, float64(ready), platform, appName, ns, "Deployment", clusterName)
	}

	// DaemonSets
	var dsList appsv1.DaemonSetList
	if err := k8sClient.List(ctx, &dsList, opts...); err != nil {
		log.Warnf("component-health-exporter: list DaemonSets with %s: %v", labelKey, err)
		return
	}
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		appName := ds.Labels[labelKey]
		if appName == "" {
			continue
		}
		ns := ds.Namespace
		desired := ds.Status.DesiredNumberScheduled
		ready := ds.Status.NumberReady
		healthy := desired > 0 && ready == desired
		v := 0.0
		if healthy {
			v = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.descHealthy, prometheus.GaugeValue, v, platform, appName, ns, "DaemonSet", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasDesired, prometheus.GaugeValue, float64(desired), platform, appName, ns, "DaemonSet", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasReady, prometheus.GaugeValue, float64(ready), platform, appName, ns, "DaemonSet", clusterName)
	}

	// StatefulSets
	var stsList appsv1.StatefulSetList
	if err := k8sClient.List(ctx, &stsList, opts...); err != nil {
		log.Warnf("component-health-exporter: list StatefulSets with %s: %v", labelKey, err)
		return
	}
	for i := range stsList.Items {
		sts := &stsList.Items[i]
		appName := sts.Labels[labelKey]
		if appName == "" {
			continue
		}
		ns := sts.Namespace
		desired := int32(0)
		if sts.Spec.Replicas != nil {
			desired = *sts.Spec.Replicas
		}
		ready := sts.Status.ReadyReplicas
		healthy := desired > 0 && ready == desired
		v := 0.0
		if healthy {
			v = 1.0
		}
		ch <- prometheus.MustNewConstMetric(c.descHealthy, prometheus.GaugeValue, v, platform, appName, ns, "StatefulSet", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasDesired, prometheus.GaugeValue, float64(desired), platform, appName, ns, "StatefulSet", clusterName)
		ch <- prometheus.MustNewConstMetric(c.descReplicasReady, prometheus.GaugeValue, float64(ready), platform, appName, ns, "StatefulSet", clusterName)
	}
}

// Ensure we implement the interface (compile-time check).
var _ prometheus.Collector = (*ComponentHealthCollector)(nil)
