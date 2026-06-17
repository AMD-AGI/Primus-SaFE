/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func newClusterReconciler(t *testing.T) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	return &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
}

// ---- model_controller pure functions ----

func TestIsS3ImportModel(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ModelS3ImportLabel: v1.TrueStr}}}
	assert.True(t, isS3ImportModel(m))
	assert.False(t, isS3ImportModel(&v1.Model{}))
	assert.False(t, isS3ImportModel(nil))
}

func TestBuildHTTPURLFromS3URI(t *testing.T) {
	m := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{v1.ModelS3SourceEndpointAnn: "minio:9000"}},
		Spec:       v1.ModelSpec{Source: v1.ModelSource{URL: "s3://bucket/prefix"}},
	}
	url, err := buildHTTPURLFromS3URI(m)
	assert.NoError(t, err)
	assert.Equal(t, "https://minio:9000/bucket/prefix/", url)

	// Not an s3 URI.
	_, err = buildHTTPURLFromS3URI(&v1.Model{Spec: v1.ModelSpec{Source: v1.ModelSource{URL: "http://x"}}})
	assert.Error(t, err)
}

func TestContainsString(t *testing.T) {
	assert.True(t, containsString([]string{"a", "b"}, "a"))
	assert.False(t, containsString([]string{"a"}, "z"))
}

func TestConstructDownloadJobErrors(t *testing.T) {
	r := newMockModelReconciler(nil)
	// Empty source URL -> error.
	_, err := r.constructDownloadJob(&v1.Model{})
	assert.Error(t, err)
}

// ---- cluster_controller pure functions ----

func TestEndpointSubsetEqual(t *testing.T) {
	a := corev1.EndpointSubset{
		Addresses: []corev1.EndpointAddress{{IP: "1.1.1.1"}},
		Ports:     []corev1.EndpointPort{{Port: 80}},
	}
	b := a.DeepCopy()
	assert.True(t, endpointSubsetEqual(a, *b))

	diff := corev1.EndpointSubset{Addresses: []corev1.EndpointAddress{{IP: "2.2.2.2"}}, Ports: []corev1.EndpointPort{{Port: 80}}}
	assert.False(t, endpointSubsetEqual(a, diff))
}

func TestEndpointsSubsetsChanged(t *testing.T) {
	a := []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "1.1.1.1"}}}}
	assert.False(t, endpointsSubsetsChanged(a, a))
	assert.True(t, endpointsSubsetsChanged(a, nil))
}

func TestIsClusterSourceEndpoints(t *testing.T) {
	r := newClusterReconciler(t)
	ep := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace}}
	assert.True(t, r.isClusterSourceEndpoints(ep))
	// Forward EP -> false.
	fwd := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1-forward", Namespace: common.PrimusSafeNamespace}}
	assert.False(t, r.isClusterSourceEndpoints(fwd))
	// Wrong namespace -> false.
	other := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "other"}}
	assert.False(t, r.isClusterSourceEndpoints(other))
}

func TestBuildDNSServerBlock(t *testing.T) {
	block := buildDNSServerBlock("foo.local", "10.0.0.1")
	assert.Contains(t, block, "foo.local")
	assert.Contains(t, block, "10.0.0.1")
}

func TestGenerateForwardName(t *testing.T) {
	assert.Equal(t, "c1-forward", generateForwardName("c1"))
}

func TestGenAllPriorityClass(t *testing.T) {
	classes := genAllPriorityClass("c1")
	assert.Len(t, classes, 3)
}

func TestGetControlPlaneIPNoCluster(t *testing.T) {
	r := newClusterReconciler(t)
	_, err := r.getControlPlaneIP(context.Background())
	assert.Error(t, err)
}

// ---- cluster_contoller_plane pure functions ----

func TestGetComponentName(t *testing.T) {
	assert.Equal(t, "comp", getComponentName("comp.suffix"))
	assert.Equal(t, "comp", getComponentName("comp"))
}

func TestShouldFetchKubeConfig(t *testing.T) {
	r := newClusterReconciler(t)
	// Created phase, no data -> true.
	c := &v1.Cluster{}
	c.Status.ControlPlaneStatus.Phase = v1.CreatedPhase
	assert.True(t, r.shouldFetchKubeConfig(c))

	// Ready phase -> false.
	c2 := &v1.Cluster{}
	c2.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	assert.False(t, r.shouldFetchKubeConfig(c2))
}

func TestAddOwnerReferences(t *testing.T) {
	r := newClusterReconciler(t)
	pod := &corev1.Pod{}
	hosts := &HostTemplateContent{Controllers: []*v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
	}}
	r.addOwnerReferences(pod, hosts)
	assert.Len(t, pod.OwnerReferences, 1)
}

func TestGenerateSSHSecretExisting(t *testing.T) {
	scheme, _ := genMockScheme()
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	// Secret already exists -> no error, no creation.
	assert.NoError(t, r.generateSSHSecret(context.Background(), cluster))
}
