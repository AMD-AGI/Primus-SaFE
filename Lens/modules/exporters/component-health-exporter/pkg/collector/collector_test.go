// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestComponentHealthCollector_Collect_WithFakeClient(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	// Deployment with primus-lens-app-name, healthy (2/2)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "primus-lens", Labels: map[string]string{labelPrimusLensAppName: "api"}},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr(int32(2))},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 2},
	}
	objs := []client.Object{deploy}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

	k8sSet := &clientsets.K8SClientSet{ControllerRuntimeClient: fakeClient}
	clusterSet := &clientsets.ClusterClientSet{ClusterName: "test-cluster", K8SClientSet: k8sSet}
	clientsets.InitClusterManagerWithClientSet(clusterSet)
	defer func() {
		// Reset global state for other tests (clusterManagerOnce cannot be reset easily; test runs in isolation)
	}()

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(NewComponentHealthCollector()))
	metrics, err := reg.Gather()
	require.NoError(t, err)

	var healthyFound bool
	var desiredFound, readyFound bool
	for _, mf := range metrics {
		if mf.GetName() != "primus_component_healthy" && mf.GetName() != "primus_component_replicas_desired" && mf.GetName() != "primus_component_replicas_ready" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "app_name" && l.GetValue() == "api" {
					if mf.GetName() == "primus_component_healthy" {
						healthyFound = true
						assert.Equal(t, 1.0, m.GetGauge().GetValue(), "api should be healthy 2/2")
					}
					if mf.GetName() == "primus_component_replicas_desired" {
						desiredFound = true
						assert.Equal(t, 2.0, m.GetGauge().GetValue())
					}
					if mf.GetName() == "primus_component_replicas_ready" {
						readyFound = true
						assert.Equal(t, 2.0, m.GetGauge().GetValue())
					}
					break
				}
			}
		}
	}
	assert.True(t, healthyFound, "primus_component_healthy for api should be present")
	assert.True(t, desiredFound, "primus_component_replicas_desired for api should be present")
	assert.True(t, readyFound, "primus_component_replicas_ready for api should be present")
}

func TestComponentHealthCollector_Describe(t *testing.T) {
	c := NewComponentHealthCollector()
	ch := make(chan *prometheus.Desc, 4)
	c.Describe(ch)
	close(ch)
	var count int
	for range ch {
		count++
	}
	assert.GreaterOrEqual(t, count, 3, "Describe should emit at least 3 descs")
}

func ptr(i int32) *int32 { return &i }
