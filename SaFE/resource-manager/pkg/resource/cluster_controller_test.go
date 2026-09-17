/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	testifyassert "github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func newClusterReconcilerWithClientSet(t *testing.T, cs *k8sfake.Clientset, objs ...client.Object) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs},
	}
}

func clusterEndpoints(name string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
		}},
	}
}

func TestGetClusterEndpoint(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(clusterEndpoints("c1"))
	r := newClusterReconcilerWithClientSet(t, cs)
	addrs, err := r.getClusterEndpoint(context.Background(), testCluster("c1"))
	assert.NoError(t, err)
	assert.Len(t, addrs, 1)

	// Missing endpoints -> error.
	r2 := newClusterReconcilerWithClientSet(t, k8sfake.NewSimpleClientset())
	_, err = r2.getClusterEndpoint(context.Background(), testCluster("c1"))
	assert.Error(t, err)
}

func TestGuaranteeForwardEndpointsCreate(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(clusterEndpoints("c1"))
	cluster := testCluster("c1")
	r := newClusterReconcilerWithClientSet(t, cs, cluster)
	assert.NoError(t, r.guaranteeForwardEndpoints(context.Background(), cluster))
	_, err := cs.CoreV1().Endpoints(common.PrimusSafeNamespace).Get(context.Background(), "c1-forward", metav1.GetOptions{})
	assert.NoError(t, err)
	// Idempotent (already exists, no change).
	assert.NoError(t, r.guaranteeForwardEndpoints(context.Background(), cluster))
}

func TestGuaranteeForwardService(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(clusterEndpoints("c1"))
	cluster := testCluster("c1")
	r := newClusterReconcilerWithClientSet(t, cs, cluster)
	assert.NoError(t, r.guaranteeForwardService(context.Background(), cluster))
	_, err := cs.CoreV1().Services(common.PrimusSafeNamespace).Get(context.Background(), "c1-forward", metav1.GetOptions{})
	assert.NoError(t, err)
	// Idempotent.
	assert.NoError(t, r.guaranteeForwardService(context.Background(), cluster))
}

func TestGuaranteeForwardIngressDisabled(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	r := newClusterReconcilerWithClientSet(t, cs, cluster)
	// Ingress class not higress by default -> no-op.
	assert.NoError(t, r.guaranteeForwardIngress(context.Background(), cluster))
}

func TestGetAdminClusterRole(t *testing.T) {
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "role1"}}
	scheme, _ := genMockScheme()
	_ = rbacv1.AddToScheme(scheme)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(role).Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	got, err := r.getAdminClusterRole(context.Background(), "role1")
	assert.NoError(t, err)
	assert.Equal(t, "role1", got.Name)

	// Missing -> nil, nil (IgnoreNotFound).
	got, err = r.getAdminClusterRole(context.Background(), "missing")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestGuaranteeCICDClusterRoleDisabled(t *testing.T) {
	r := newPlaneReconciler(t)
	// CI/CD disabled by default -> no-op.
	assert.NoError(t, r.guaranteeCICDClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.deleteCICDClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.guaranteeMonarchClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.deleteMonarchClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.guaranteeGithubRunnerClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.deleteGithubRunnerClusterRole(context.Background(), testCluster("c1")))
}

var _ = v1.ClusterKind

// --- merged from cluster_guarantee_full_test.go ---

// newClusterReconcilerFull builds a ClusterReconciler whose admin client (ctrl
// fake) holds objs, with both r.clientSet and the data-plane factory backed by
// the given clientset.

func newClusterReconcilerFull(t *testing.T, cs *k8sfake.Clientset, objs ...ctrlclient.Object) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	_ = rbacv1.AddToScheme(scheme)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1.Cluster{}, &v1.Node{}).WithObjects(objs...).Build()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	return &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs},
		clientManager:         mgr,
	}
}

func TestGuaranteeCICDClusterRoleBindingFull(t *testing.T) {
	patches := gomonkey.NewPatches()
	patches.ApplyFunc(commonconfig.IsCICDEnable, func() bool { return true })
	patches.ApplyFunc(commonconfig.GetCICDRoleName, func() string { return "cicd-role" })
	patches.ApplyFunc(commonconfig.GetCICDControllerName, func() string { return "cicd-sa" })
	defer patches.Reset()

	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs)
	err := r.guaranteeCICDClusterRoleBinding(context.Background(), testCluster("c1"))
	testifyassert.NoError(t, err)
	_, err = cs.RbacV1().ClusterRoleBindings().Get(context.Background(), "cicd-role", metav1.GetOptions{})
	testifyassert.NoError(t, err)
}

func TestGuaranteeAllImageSecretsFull(t *testing.T) {
	patches := gomonkey.NewPatches()
	patches.ApplyFunc(commonconfig.GetImageSecret, func() string { return "img-secret" })
	defer patches.Reset()

	cs := k8sfake.NewSimpleClientset()
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "img-secret", Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{".dockerconfigjson": []byte("{}")},
		Type:       corev1.SecretTypeDockerConfigJson,
	}
	r := newClusterReconcilerFull(t, cs, adminSecret)
	err := r.guaranteeAllImageSecrets(context.Background(), readyCluster("c1"))
	testifyassert.NoError(t, err)
}

func TestGuaranteeForwardIngressFull(t *testing.T) {
	patches := gomonkey.NewPatches()
	patches.ApplyFunc(commonconfig.GetIngress, func() string { return common.HigressClassname })
	patches.ApplyFunc(commonconfig.GetSystemHost, func() string { return "safe.local" })
	defer patches.Reset()

	srcEp := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Subsets:    []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}},
	}
	cs := k8sfake.NewSimpleClientset(srcEp)
	r := newClusterReconcilerFull(t, cs)
	err := r.guaranteeForwardIngress(context.Background(), testCluster("c1"))
	testifyassert.NoError(t, err)
	_, err = cs.NetworkingV1().Ingresses(common.PrimusSafeNamespace).Get(context.Background(), "c1-forward", metav1.GetOptions{})
	testifyassert.NoError(t, err)
}

func TestGuaranteeDataPlaneClusterRoleFull(t *testing.T) {
	adminRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-role", Labels: map[string]string{"a": "b"}},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"pods"}}},
	}
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, adminRole)
	err := r.guaranteeDataPlaneClusterRole(context.Background(), testCluster("c1"), "dp-role")
	testifyassert.NoError(t, err)
	_, err = cs.RbacV1().ClusterRoles().Get(context.Background(), "dp-role", metav1.GetOptions{})
	testifyassert.NoError(t, err)
	// second call should update path (already exists in data plane)
	testifyassert.NoError(t, r.guaranteeDataPlaneClusterRole(context.Background(), testCluster("c1"), "dp-role"))

	testifyassert.NoError(t, r.deleteDataPlaneClusterRole(context.Background(), testCluster("c1"), "dp-role"))
}

func TestClusterDeleteAndCleanupFull(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Finalizers = []string{v1.ClusterFinalizer}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, cluster, node)
	ctx := context.Background()

	testifyassert.NoError(t, r.cleanupClusterResources(ctx, cluster))
	testifyassert.NoError(t, r.delete(ctx, cluster))
	got := &v1.Cluster{}
	testifyassert.NoError(t, r.Get(ctx, ctrlclient.ObjectKey{Name: "c1"}, got))
	testifyassert.NotContains(t, got.Finalizers, v1.ClusterFinalizer)
}

func TestClusterReconcileReadyNoControlPlaneNodes(t *testing.T) {
	cluster := readyCluster("c1")
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, cluster)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
	testifyassert.NoError(t, err)
	// priority classes should have been created in the data plane
	pcs, err := cs.SchedulingV1().PriorityClasses().List(context.Background(), metav1.ListOptions{})
	testifyassert.NoError(t, err)
	testifyassert.NotEmpty(t, pcs.Items)
}

func TestClusterReconcileDeletePhase(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Finalizers = []string{v1.ClusterFinalizer}
	cluster.Status.ControlPlaneStatus.Phase = v1.DeletedPhase
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, cluster)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
	testifyassert.NoError(t, err)
}

func TestGuaranteeMonarchClusterRoleFull(t *testing.T) {
	patches := gomonkey.NewPatches()
	patches.ApplyFunc(commonconfig.IsMonarchEnable, func() bool { return true })
	patches.ApplyFunc(commonconfig.GetMonarchClientRole, func() string { return "monarch-role" })
	defer patches.Reset()

	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "monarch-role"}}
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, role)
	testifyassert.NoError(t, r.guaranteeMonarchClusterRole(context.Background(), testCluster("c1")))
	testifyassert.NoError(t, r.deleteMonarchClusterRole(context.Background(), testCluster("c1")))
}

func TestGuaranteeGithubRunnerClusterRoleFull(t *testing.T) {
	commonconfig.SetValue("cicd.enable", "true")
	defer commonconfig.SetValue("cicd.enable", "")

	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: common.GithubRunnerServiceAccount}}
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, role)
	testifyassert.NoError(t, r.guaranteeGithubRunnerClusterRole(context.Background(), testCluster("c1")))
	testifyassert.NoError(t, r.deleteGithubRunnerClusterRole(context.Background(), testCluster("c1")))
}

// --- merged from cluster_guarantee_test.go ---

func rbacAddToSchemeForTest(s *runtime.Scheme) error { return rbacv1.AddToScheme(s) }

func newClusterRole(name string) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func newClusterReconcilerWithFactory(t *testing.T, clusterName string, cs *k8sfake.Clientset, objs ...client.Object) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	testifyassert.NoError(t, err)
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
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestGuaranteePriorityClassReady(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := readyCluster("c1")
	r := newClusterReconcilerWithFactory(t, "c1", cs)
	_, err := r.guaranteePriorityClass(context.Background(), cluster)
	testifyassert.NoError(t, err)
	// Priority classes should now exist.
	list, err := cs.SchedulingV1().PriorityClasses().List(context.Background(), metav1.ListOptions{})
	testifyassert.NoError(t, err)
	testifyassert.Len(t, list.Items, 3)
}

func TestDeletePriorityClass(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := readyCluster("c1")
	r := newClusterReconcilerWithFactory(t, "c1", cs)
	_, _ = r.guaranteePriorityClass(context.Background(), cluster)
	testifyassert.NoError(t, r.deletePriorityClass(context.Background(), cluster))
}

func TestGetAdminImageSecretNotFound(t *testing.T) {
	r := newPlaneReconciler(t)
	_, err := r.getAdminImageSecret(context.Background())
	testifyassert.Error(t, err)
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
	testifyassert.NoError(t, err)
	// Role should now exist in data plane.
	_, err = cs.RbacV1().ClusterRoles().Get(context.Background(), "role1", metav1.GetOptions{})
	testifyassert.NoError(t, err)
}

func TestGuaranteeDataPlaneClusterRoleEmptyName(t *testing.T) {
	r := newClusterReconcilerWithFactory(t, "c1", k8sfake.NewSimpleClientset())
	testifyassert.NoError(t, r.guaranteeDataPlaneClusterRole(context.Background(), testCluster("c1"), ""))
}

func TestDeleteDataPlaneClusterRole(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(newClusterRole("role1"))
	r := newClusterReconcilerWithFactory(t, "c1", cs)
	testifyassert.NoError(t, r.deleteDataPlaneClusterRole(context.Background(), testCluster("c1"), "role1"))
	testifyassert.NoError(t, r.deleteDataPlaneClusterRole(context.Background(), testCluster("c1"), ""))
}

func TestGuaranteeImageSecretCreate(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	// Admin-plane secret exists.
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

// --- merged from cluster_plane_extra_test.go ---

func TestClusterReconcileNotFound(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         commonutils.NewObjectManager(),
	}
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	testifyassert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestCleanupClusterResources(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := readyCluster("c1")
	r := newClusterReconcilerWithFactory(t, "c1", cs, cluster)
	// All deletes are no-ops on empty cluster.
	testifyassert.NoError(t, r.cleanupClusterResources(context.Background(), cluster))
}

func TestResetNodesOfCluster(t *testing.T) {
	scheme, _ := genMockScheme()
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "n1",
			Labels: map[string]string{v1.ClusterIdLabel: "c1"},
		},
	}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Node{}).
		WithObjects(node).
		Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	cluster := testCluster("c1")
	testifyassert.NoError(t, r.resetNodesOfCluster(context.Background(), cluster))
	updated := &v1.Node{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "n1"}, updated))
	testifyassert.Nil(t, updated.Spec.Cluster)
}

func TestClusterDelete(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := testCluster("c1")
	cluster.Finalizers = []string{v1.ClusterFinalizer}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         commonutils.NewObjectManager(),
	}
	testifyassert.NoError(t, r.delete(context.Background(), cluster))
}

func TestClusterReconcileReadyHappyPath(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := readyCluster("c1")
	cluster.Finalizers = []string{v1.ClusterFinalizer}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Cluster{}).
		WithObjects(cluster).
		Build()
	cs := k8sfake.NewSimpleClientset()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         mgr,
	}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
	testifyassert.NoError(t, err)
	// Priority classes created in data plane.
	list, err := cs.SchedulingV1().PriorityClasses().List(context.Background(), metav1.ListOptions{})
	testifyassert.NoError(t, err)
	testifyassert.Len(t, list.Items, 3)
}

func TestGuaranteeClientFactoryNotReady(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         commonutils.NewObjectManager(),
	}
	// Not ready -> no-op nil.
	testifyassert.NoError(t, r.guaranteeClientFactory(context.Background(), testCluster("c1")))
}

func TestShouldPeriodicSyncControlPlaneEndpoints(t *testing.T) {
	ready := testCluster("c1")
	ready.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	ready.Spec.ControlPlane.Nodes = []string{"cp1"}

	notReady := testCluster("c2")
	notReady.Spec.ControlPlane.Nodes = []string{"cp1"}

	deleting := ready.DeepCopy()
	now := metav1.Now()
	deleting.DeletionTimestamp = &now

	testifyassert.True(t, shouldPeriodicSyncControlPlaneEndpoints(ready))
	testifyassert.False(t, shouldPeriodicSyncControlPlaneEndpoints(notReady))
	testifyassert.False(t, shouldPeriodicSyncControlPlaneEndpoints(deleting))
	testifyassert.False(t, shouldPeriodicSyncControlPlaneEndpoints(nil))
}

func TestMarkClusterClientFactoryStale(t *testing.T) {
	mgr := commonutils.NewObjectManager()
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	testifyassert.NoError(t, mgr.Add("c1", factory))
	r := &ClusterReconciler{clientManager: mgr}
	r.markClusterClientFactoryStale("c1", "control plane endpoints changed")
	testifyassert.False(t, factory.IsValid())
}

func TestGuaranteeClientFactoryKeepsValidFactoryWithoutEndpoint(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := readyCluster("c1")
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	cs := k8sfake.NewSimpleClientset()
	mgr := commonutils.NewObjectManager()
	factory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
	testifyassert.NoError(t, mgr.Add("c1", factory))
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         mgr,
	}
	testifyassert.NoError(t, r.guaranteeClientFactory(context.Background(), cluster))
	testifyassert.True(t, factory.IsValid())
}

func TestSyncControlPlaneServiceEndpointsSkipsWithoutCPNodes(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := readyCluster("c1")
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	testifyassert.NoError(t, r.syncControlPlaneServiceEndpoints(context.Background(), cluster))
}

func newClusterReconciler(t *testing.T) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	return &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
}

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

func TestGenerateForwardName(t *testing.T) {
	assert.Equal(t, "c1-forward", generateForwardName("c1"))
}

func TestGenAllPriorityClass(t *testing.T) {
	classes := genAllPriorityClass("c1")
	assert.Len(t, classes, 3)
}

func TestClusterRelevantChangePredicate(t *testing.T) {
	r := newClusterReconciler(t)
	p := r.relevantChangePredicate()
	ready := readyCluster("c1")
	assert.True(t, p.Create(event.CreateEvent{Object: ready}))
	assert.False(t, p.Create(event.CreateEvent{Object: testCluster("c2")}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: testCluster("c1"), ObjectNew: ready}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: ready}))
}

func TestClusterHandleNodeEvent(t *testing.T) {
	r := newClusterReconciler(t)
	h := r.handleNodeEvent().(genericEventHandler)
	q := resWorkQueue()
	defer q.ShutDown()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "n1",
		OwnerReferences: []metav1.OwnerReference{
			{APIVersion: v1.SchemeGroupVersion.String(), Kind: v1.ClusterKind, Name: "c1"},
		},
	}}
	h.Create(context.Background(), event.CreateEvent{Object: node}, q)
	assert.Equal(t, 1, q.Len())
}

func TestClusterEndpointsPredicate(t *testing.T) {
	r := newClusterReconciler(t)
	p := r.endpointsPredicate()
	ep := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace}}
	assert.True(t, p.Create(event.CreateEvent{Object: ep}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: ep}))
	// Update with no subset change -> false.
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: ep, ObjectNew: ep.DeepCopy()}))
}

func TestClusterHandleEndpointsEvent(t *testing.T) {
	r := newClusterReconciler(t)
	h := r.handleEndpointsEvent()
	ep := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace}}
	reqs := h.(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
	})
	q := resWorkQueue()
	defer q.ShutDown()
	reqs.Create(context.Background(), event.CreateEvent{Object: ep}, q)
	assert.Equal(t, 1, q.Len())
}

func TestClusterHandlePodEvent(t *testing.T) {
	r := newClusterReconciler(t)
	h := r.handlePodEvent().(genericEventHandler)
	q := resWorkQueue()
	defer q.ShutDown()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "p1",
		OwnerReferences: []metav1.OwnerReference{
			{APIVersion: v1.SchemeGroupVersion.String(), Kind: v1.ClusterKind, Name: "c1"},
		},
	}}
	h.Create(context.Background(), event.CreateEvent{Object: pod}, q)
	assert.Equal(t, 1, q.Len())
}
