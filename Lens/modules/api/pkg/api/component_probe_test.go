// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGroupPodsToComponents(t *testing.T) {
	runningPod := func(name, namespace, appLabel, node string) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{labelPrimusLensAppName: appLabel}},
			Spec:       corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		}
	}
	notRunningPod := func(name, namespace, appLabel string) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{labelPrimusLensAppName: appLabel}},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		}
	}

	tests := []struct {
		name     string
		pods     []corev1.Pod
		labelKey string
		wantLen  int
		want     map[string]struct{ total, ready int; healthy bool }
	}{
		{
			name:     "empty pods",
			pods:     nil,
			labelKey: labelPrimusLensAppName,
			wantLen:  0,
			want:     nil,
		},
		{
			name: "single component one running pod",
			pods: []corev1.Pod{
				runningPod("api-0", "primus-lens", "api", "node1"),
			},
			labelKey: labelPrimusLensAppName,
			wantLen:  1,
			want:     map[string]struct{ total, ready int; healthy bool }{"api": {1, 1, true}},
		},
		{
			name: "single component one not running pod",
			pods: []corev1.Pod{
				notRunningPod("api-0", "primus-lens", "api"),
			},
			labelKey: labelPrimusLensAppName,
			wantLen:  1,
			want:     map[string]struct{ total, ready int; healthy bool }{"api": {1, 0, false}},
		},
		{
			name: "two components mixed ready",
			pods: []corev1.Pod{
				runningPod("api-0", "ns1", "api", "n1"),
				notRunningPod("api-1", "ns1", "api"),
				runningPod("jobs-0", "ns1", "jobs", "n2"),
			},
			labelKey: labelPrimusLensAppName,
			wantLen:  2,
			want: map[string]struct{ total, ready int; healthy bool }{
				"api":  {2, 1, true},
				"jobs": {1, 1, true},
			},
		},
		{
			name: "pods without label are skipped",
			pods: []corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "no-label", Namespace: "ns", Labels: nil}},
				runningPod("api-0", "ns", "api", "n1"),
			},
			labelKey: labelPrimusLensAppName,
			wantLen:  1,
			want:     map[string]struct{ total, ready int; healthy bool }{"api": {1, 1, true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupPodsToComponents(tt.pods, tt.labelKey)
			require.Len(t, got, tt.wantLen)
			for _, c := range got {
				w, ok := tt.want[c.AppName]
				require.True(t, ok, "unexpected component %s", c.AppName)
				assert.Equal(t, w.total, c.Total, "Total for %s", c.AppName)
				assert.Equal(t, w.ready, c.Ready, "Ready for %s", c.AppName)
				assert.Equal(t, w.healthy, c.Healthy, "Healthy for %s", c.AppName)
			}
		})
	}
}

func TestProbeCoreDNS(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	corednsRunningPod := func(name, node string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: kubeSystemNamespace,
				Labels:    map[string]string{labelK8sApp: labelCoreDNS},
			},
			Spec: corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		}
	}

	t.Run("no deployment returns zero item and no error", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		item, err := probeCoreDNS(context.Background(), c, "cluster1")
		require.NoError(t, err)
		assert.False(t, item.Healthy)
		assert.Equal(t, "coredns", item.Name)
		assert.Equal(t, int32(0), item.Desired)
	})

	t.Run("deployment and pods healthy", func(t *testing.T) {
		replicas := int32(2)
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: kubeSystemNamespace,
				Labels:    map[string]string{labelK8sApp: labelCoreDNS},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{labelK8sApp: labelCoreDNS}},
			},
		}
		objs := []client.Object{dep, corednsRunningPod("coredns-1", "node1"), corednsRunningPod("coredns-2", "node2")}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
		item, err := probeCoreDNS(context.Background(), c, "cluster1")
		require.NoError(t, err)
		assert.True(t, item.Healthy)
		assert.Equal(t, int32(2), item.Desired)
		assert.Equal(t, int32(2), item.Ready)
		assert.Len(t, item.Pods, 2)
	})

	t.Run("deployment with one not ready", func(t *testing.T) {
		replicas := int32(2)
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "coredns",
				Namespace: kubeSystemNamespace,
				Labels:    map[string]string{labelK8sApp: labelCoreDNS},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{labelK8sApp: labelCoreDNS}},
			},
		}
		pending := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "coredns-1", Namespace: kubeSystemNamespace, Labels: map[string]string{labelK8sApp: labelCoreDNS}},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		}
		objs := []client.Object{dep, pending, corednsRunningPod("coredns-2", "node2")}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
		item, err := probeCoreDNS(context.Background(), c, "cluster1")
		require.NoError(t, err)
		assert.False(t, item.Healthy)
		assert.Equal(t, int32(2), item.Desired)
		assert.Equal(t, int32(1), item.Ready)
	})
}

func TestProbeNodeLocalDNS(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	nodelocalRunningPod := func(name, node string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: kubeSystemNamespace,
				Labels:    map[string]string{labelK8sApp: labelNodeLocalDNS},
			},
			Spec: corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		}
	}

	t.Run("no daemonset returns zero item and no error", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		item, err := probeNodeLocalDNS(context.Background(), c, "cluster1")
		require.NoError(t, err)
		assert.False(t, item.Healthy)
		assert.Equal(t, "node-local-dns", item.Name)
		assert.Equal(t, int32(0), item.Desired)
	})

	t.Run("daemonset and pods healthy", func(t *testing.T) {
		ds := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-local-dns",
				Namespace: kubeSystemNamespace,
				Labels:    map[string]string{labelK8sApp: labelNodeLocalDNS},
			},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{labelK8sApp: labelNodeLocalDNS}},
			},
			Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 2},
		}
		objs := []client.Object{ds, nodelocalRunningPod("nodelocal-1", "node1"), nodelocalRunningPod("nodelocal-2", "node2")}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
		item, err := probeNodeLocalDNS(context.Background(), c, "cluster1")
		require.NoError(t, err)
		assert.True(t, item.Healthy)
		assert.Equal(t, int32(2), item.Desired)
		assert.Equal(t, int32(2), item.Ready)
		assert.Len(t, item.Pods, 2)
	})
}

func TestListComponentsByLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	runningPod := func(name, ns, appName, node string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    map[string]string{labelPrimusLensAppName: appName},
			},
			Spec: corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		}
	}

	t.Run("empty cluster", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		list, err := listComponentsByLabel(context.Background(), c, labelPrimusLensAppName)
		require.NoError(t, err)
		assert.Empty(t, list)
	})

	t.Run("pods with primus-lens-app-name grouped", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(
				runningPod("api-0", "primus-lens", "api", "n1"),
				runningPod("jobs-0", "primus-lens", "jobs", "n2"),
			).
			Build()
		list, err := listComponentsByLabel(context.Background(), c, labelPrimusLensAppName)
		require.NoError(t, err)
		require.Len(t, list, 2)
		byApp := make(map[string]PlatformComponentItem)
		for _, item := range list {
			byApp[item.AppName] = item
		}
		assert.Equal(t, 1, byApp["api"].Total)
		assert.Equal(t, 1, byApp["api"].Ready)
		assert.True(t, byApp["api"].Healthy)
		assert.Equal(t, 1, byApp["jobs"].Total)
		assert.True(t, byApp["jobs"].Healthy)
	})
}
