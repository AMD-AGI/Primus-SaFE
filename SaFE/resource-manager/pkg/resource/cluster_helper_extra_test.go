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
	"k8s.io/utils/pointer"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestGetKubeSprayCreateCMD(t *testing.T) {
	cmd := getKubeSprayCreateCMD("root", "-e x=y")
	assert.Contains(t, cmd, "cluster.yml")
	assert.Contains(t, cmd, "-e x=y")
}

func TestGetKubeSprayHostsCMD(t *testing.T) {
	assert.Contains(t, getKubeSprayHostsCMD("root"), "ansible all")
	nonRoot := getKubeSprayHostsCMD("user1")
	assert.Contains(t, nonRoot, "useradd")
	assert.Contains(t, nonRoot, "user1")
}

func TestGetKubeSprayEnv(t *testing.T) {
	cluster := &v1.Cluster{}
	cluster.Spec.ControlPlane.KubeVersion = pointer.String("1.28")
	cluster.Spec.ControlPlane.KubeProxyMode = pointer.String("ipvs")
	env := getKubeSprayEnv(cluster)
	assert.Contains(t, env, "kube_version=1.28")
	assert.Contains(t, env, "conntrack_modules")
	assert.Contains(t, env, "auto_renew_certificates=true")
}

func TestGetKubeSprayResetCMD(t *testing.T) {
	assert.Contains(t, getKubeSprayResetCMD("root", ""), "reset.yml")
}

func TestGetKubesprayImage(t *testing.T) {
	assert.Equal(t, DefaultKubeSprayImage, getKubesprayImage(&v1.Cluster{}))
	cluster := &v1.Cluster{}
	cluster.Spec.ControlPlane.KubeSprayImage = pointer.String("custom:1")
	assert.Equal(t, "custom:1", getKubesprayImage(cluster))
}

func TestGetNodeLabelsString(t *testing.T) {
	_, ok := getNodeLabelsString(&v1.Node{})
	assert.False(t, ok)

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"k": "v"}}}
	s, ok := getNodeLabelsString(node)
	assert.True(t, ok)
	assert.Contains(t, s, "\"k\":\"v\"")
}

func TestCreateKubernetesClusterOwnerReference(t *testing.T) {
	cluster := &v1.Cluster{}
	cluster.APIVersion = "amd.com/v1"
	cluster.Kind = "Cluster"
	cluster.Name = "c1"
	ref := createKubernetesClusterOwnerReference(cluster)
	assert.Equal(t, "c1", ref.Name)
	assert.True(t, *ref.Controller)
}

func TestGuaranteeControllerPlane(t *testing.T) {
	// Pending phase -> true.
	c := &v1.Cluster{}
	c.Status.ControlPlaneStatus.Phase = v1.PendingPhase
	assert.True(t, guaranteeControllerPlane(c))

	// Ready phase -> false.
	c2 := &v1.Cluster{}
	c2.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	assert.False(t, guaranteeControllerPlane(c2))
}

func TestClusterGetNode(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	r := &ClusterBaseReconciler{Client: cl}
	got, err := r.getNode(context.Background(), "n1")
	assert.NoError(t, err)
	assert.Equal(t, "n1", got.Name)

	_, err = r.getNode(context.Background(), "missing")
	assert.Error(t, err)
}

func TestClusterGetUsername(t *testing.T) {
	scheme, _ := genMockScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{"username": []byte("admin")},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	r := &ClusterBaseReconciler{Client: cl}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	node := &v1.Node{}
	username, err := r.getUsername(context.Background(), node, cluster)
	assert.NoError(t, err)
	assert.Equal(t, "admin", username)
}
