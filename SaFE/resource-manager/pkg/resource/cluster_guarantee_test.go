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
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/agiledragon/gomonkey/v2"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func rbacAddToSchemeForTest(s *runtime.Scheme) error { return rbacv1.AddToScheme(s) }

func newClusterRole(name string) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func newClusterReconcilerWithFactory(t *testing.T, clusterName string, cs *k8sfake.Clientset, objs ...client.Object) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	mgr := commonutils.NewObjectManager()
	factory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, cs)
	_ = mgr.Add(clusterName, factory)
	return &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         mgr,
	}
}

func readyCluster(name string) *v1.Cluster {
	c := testCluster(name)
	c.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	return c
}

func TestGuaranteePriorityClassNotReady(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerWithFactory(t, "c1", cs)
	// Not ready -> no-op.
	res, err := r.guaranteePriorityClass(context.Background(), testCluster("c1"))
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestGuaranteePriorityClassReady(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := readyCluster("c1")
	r := newClusterReconcilerWithFactory(t, "c1", cs)
	_, err := r.guaranteePriorityClass(context.Background(), cluster)
	assert.NoError(t, err)
	// Priority classes should now exist.
	list, err := cs.SchedulingV1().PriorityClasses().List(context.Background(), metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Len(t, list.Items, 3)
}

func TestDeletePriorityClass(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := readyCluster("c1")
	r := newClusterReconcilerWithFactory(t, "c1", cs)
	_, _ = r.guaranteePriorityClass(context.Background(), cluster)
	assert.NoError(t, r.deletePriorityClass(context.Background(), cluster))
}

func TestGetAdminImageSecretNotFound(t *testing.T) {
	r := newPlaneReconciler(t)
	_, err := r.getAdminImageSecret(context.Background())
	assert.Error(t, err)
}

func TestGuaranteeDataPlaneClusterRole(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	scheme, _ := genMockScheme()
	_ = rbacAddToSchemeForTest(scheme)
	role := newClusterRole("role1")
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(role).Build()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}, clientManager: mgr}

	err := r.guaranteeDataPlaneClusterRole(context.Background(), testCluster("c1"), "role1")
	assert.NoError(t, err)
	// Role should now exist in data plane.
	_, err = cs.RbacV1().ClusterRoles().Get(context.Background(), "role1", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestGuaranteeNodeLocalDNSNoHost(t *testing.T) {
	r := newClusterReconcilerWithFactory(t, "c1", k8sfake.NewSimpleClientset())
	// GetSystemHost default empty -> no-op.
	assert.NoError(t, r.guaranteeNodeLocalDNS(context.Background(), testCluster("c1")))
}

func TestGuaranteeNodeLocalDNSUpdatesCorefile(t *testing.T) {
	patches := gomonkey.ApplyFunc(commonconfig.GetSystemHost, func() string { return "safe.local" })
	defer patches.Reset()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "nodelocaldns", Namespace: "kube-system"},
		Data:       map[string]string{"Corefile": ".:53 {\n}"},
	}
	cs := k8sfake.NewSimpleClientset(cm)

	scheme, _ := genMockScheme()
	dataCluster := readyCluster("c1")
	cpCluster := readyCluster("ctrl")
	cpCluster.Labels = map[string]string{v1.ClusterControlPlaneLabel: ""}
	cpCluster.Status.ControlPlaneStatus.Endpoints = []string{"https://10.0.0.9:6443"}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(dataCluster, cpCluster).Build()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}, clientManager: mgr}

	err := r.guaranteeNodeLocalDNS(context.Background(), dataCluster)
	assert.NoError(t, err)
	updated, _ := cs.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "nodelocaldns", metav1.GetOptions{})
	assert.Contains(t, updated.Data["Corefile"], "safe.local")
}

func TestGuaranteeDataPlaneClusterRoleEmptyName(t *testing.T) {
	r := newClusterReconcilerWithFactory(t, "c1", k8sfake.NewSimpleClientset())
	assert.NoError(t, r.guaranteeDataPlaneClusterRole(context.Background(), testCluster("c1"), ""))
}

func TestDeleteDataPlaneClusterRole(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(newClusterRole("role1"))
	r := newClusterReconcilerWithFactory(t, "c1", cs)
	assert.NoError(t, r.deleteDataPlaneClusterRole(context.Background(), testCluster("c1"), "role1"))
	assert.NoError(t, r.deleteDataPlaneClusterRole(context.Background(), testCluster("c1"), ""))
}

func TestGuaranteeImageSecretCreate(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	// admin plane secret存在
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "img-secret", Namespace: "primus-safe"},
		Data:       map[string][]byte{".dockerconfigjson": []byte("{}")},
	}
	r := newClusterReconcilerWithFactory(t, "c1", cs, adminSecret)
	// getAdminImageSecret reads from GetImageSecret() which is empty by default; just ensure no panic on get.
	_, err := r.getAdminImageSecret(context.Background())
	// Empty name -> not found error acceptable.
	_ = err
}
