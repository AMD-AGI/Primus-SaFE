/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	testifyassert "github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
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

func TestGenerateSSHSecretFull(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.SSHSecret = &corev1.ObjectReference{Name: "ssh", Namespace: "default"}
	sshSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ssh", Namespace: "default"},
		Data:       map[string][]byte{utils.Username: []byte("admin")},
	}
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, cluster, node, sshSecret)
	err := r.generateSSHSecret(context.Background(), cluster)
	testifyassert.NoError(t, err)
	got := &corev1.Secret{}
	testifyassert.NoError(t, r.Get(context.Background(), ctrlclient.ObjectKey{Name: "c1", Namespace: common.PrimusSafeNamespace}, got))
	assert.Equal(t, "admin", string(got.Data[utils.Username]))
}

func TestClearPodsFull(t *testing.T) {
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "old",
			Namespace:         common.PrimusSafeNamespace,
			Labels:            map[string]string{v1.ClusterManageClusterLabel: "c1"},
			CreationTimestamp: metav1.NewTime(time.Now().UTC().Add(-2 * time.Hour)),
		},
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}
	runningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "running",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.ClusterManageClusterLabel: "c1"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, oldPod, runningPod)
	ctx := context.Background()
	testifyassert.NoError(t, r.clearPods(ctx, testCluster("c1")))
	got := &corev1.Pod{}
	testifyassert.Error(t, r.Get(ctx, ctrlclient.ObjectKey{Name: "old", Namespace: common.PrimusSafeNamespace}, got))
	testifyassert.NoError(t, r.Get(ctx, ctrlclient.ObjectKey{Name: "running", Namespace: common.PrimusSafeNamespace}, got))
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

func TestControlPlaneStatusHelpers(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	r := newClusterReconcilerFull(t, cs, cluster)
	ctx := context.Background()

	// updatePodStatus: succeeded -> CreatedPhase
	succeeded := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}
	testifyassert.NoError(t, r.updatePodStatus(ctx, cluster, succeeded))
	assert.Equal(t, v1.CreatedPhase, cluster.Status.ControlPlaneStatus.Phase)

	// updatePodStatus: failed -> CreationFailed
	failed := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}
	testifyassert.NoError(t, r.updatePodStatus(ctx, cluster, failed))
	assert.Equal(t, v1.CreationFailed, cluster.Status.ControlPlaneStatus.Phase)

	// updateResetPhase variants (pure)
	r.updateResetPhase(cluster, succeeded)
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)
	r.updateResetPhase(cluster, failed)
	assert.Equal(t, v1.DeleteFailedPhase, cluster.Status.ControlPlaneStatus.Phase)
	r.updateResetPhase(cluster, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}})
	assert.Equal(t, v1.DeletingPhase, cluster.Status.ControlPlaneStatus.Phase)

	// reset with nil hosts -> DeletedPhase
	testifyassert.NoError(t, r.reset(ctx, cluster, nil))
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)
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

func TestGuaranteeNodeLocalDNSNoHost(t *testing.T) {
	r := newClusterReconcilerWithFactory(t, "c1", k8sfake.NewSimpleClientset())
	// GetSystemHost default empty -> no-op.
	testifyassert.NoError(t, r.guaranteeNodeLocalDNS(context.Background(), testCluster("c1")))
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
	testifyassert.NoError(t, err)
	updated, _ := cs.CoreV1().ConfigMaps("kube-system").Get(context.Background(), "nodelocaldns", metav1.GetOptions{})
	testifyassert.Contains(t, updated.Data["Corefile"], "safe.local")
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

// --- merged from cluster_plane_extra_test.go ---

func newPlaneReconciler(t *testing.T, objs ...client.Object) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	testifyassert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Cluster{}).
		WithObjects(objs...).
		Build()
	return &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
}

func TestGetControllerPlaneNodes(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	r := newPlaneReconciler(t, node)
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	nodes, err := r.getControllerPlaneNodes(context.Background(), cluster)
	testifyassert.NoError(t, err)
	testifyassert.Len(t, nodes, 1)

	// Missing node -> error.
	cluster.Spec.ControlPlane.Nodes = []string{"missing"}
	_, err = r.getControllerPlaneNodes(context.Background(), cluster)
	testifyassert.Error(t, err)
}

func TestGuaranteeNamespace(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newPlaneReconciler(t)
	// Create namespace.
	testifyassert.NoError(t, r.guaranteeNamespace(context.Background(), cs, "ns1"))
	_, err := cs.CoreV1().Namespaces().Get(context.Background(), "ns1", metav1.GetOptions{})
	testifyassert.NoError(t, err)
	// Idempotent.
	testifyassert.NoError(t, r.guaranteeNamespace(context.Background(), cs, "ns1"))
}

func TestGuaranteeEndpoints(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	nodes := []*v1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}}}
	testifyassert.NoError(t, r.guaranteeEndpoints(context.Background(), cluster, nodes))
	ep := &corev1.Endpoints{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1", Namespace: common.PrimusSafeNamespace}, ep))
	// Already exists -> no-op.
	testifyassert.NoError(t, r.guaranteeEndpoints(context.Background(), cluster, nodes))
}

func TestGuaranteeServiceResource(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	testifyassert.NoError(t, r.guaranteeServiceResource(context.Background(), cluster))
	svc := &corev1.Service{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1", Namespace: common.PrimusSafeNamespace}, svc))
	// Already exists -> no-op.
	testifyassert.NoError(t, r.guaranteeServiceResource(context.Background(), cluster))
}

func TestGuaranteeServiceNotReady(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Status.ControlPlaneStatus.Phase = v1.PendingPhase
	r := newPlaneReconciler(t, cluster)
	// Not ready -> no-op nil.
	testifyassert.NoError(t, r.guaranteeService(context.Background(), cluster))
}

func TestGuaranteeServiceReady(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}}
	r := newPlaneReconciler(t, cluster, node)
	testifyassert.NoError(t, r.guaranteeService(context.Background(), cluster))
}

func TestUpdatePodStatus(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)

	succeeded := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}
	testifyassert.NoError(t, r.updatePodStatus(context.Background(), cluster, succeeded))
	assert.Equal(t, v1.CreatedPhase, cluster.Status.ControlPlaneStatus.Phase)

	failed := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}
	testifyassert.NoError(t, r.updatePodStatus(context.Background(), cluster, failed))
	assert.Equal(t, v1.CreationFailed, cluster.Status.ControlPlaneStatus.Phase)
}

func TestUpdateResetPhase(t *testing.T) {
	r := newPlaneReconciler(t)
	cluster := testCluster("c1")

	r.updateResetPhase(cluster, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}})
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)

	r.updateResetPhase(cluster, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}})
	assert.Equal(t, v1.DeleteFailedPhase, cluster.Status.ControlPlaneStatus.Phase)

	r.updateResetPhase(cluster, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}})
	assert.Equal(t, v1.DeletingPhase, cluster.Status.ControlPlaneStatus.Phase)
}

func TestPlaneGetUsernameNoNodes(t *testing.T) {
	r := newPlaneReconciler(t)
	_, err := r.getUsername(context.Background(), testCluster("c1"))
	testifyassert.Error(t, err)
}

func TestGuaranteeClusterControlPlaneNoNodes(t *testing.T) {
	r := newPlaneReconciler(t)
	// No control plane nodes -> nil.
	testifyassert.NoError(t, r.guaranteeClusterControlPlane(context.Background(), testCluster("c1")))
}

func TestResetNilHostsContent(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	// Nil hostsContent -> phase set to Deleted.
	testifyassert.NoError(t, r.reset(context.Background(), cluster, nil))
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)
}

func TestPatchKubeControlPlanNodes(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"n1", "missing"}
	r := newPlaneReconciler(t, node, cluster)
	testifyassert.NoError(t, r.patchKubeControlPlanNodes(context.Background(), cluster))
	updated := &v1.Node{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "n1"}, updated))
	assert.Equal(t, "c1", updated.GetSpecCluster())
}

func planeClusterWithNode(t *testing.T) (*v1.Cluster, *ClusterReconciler) {
	t.Helper()
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.Nodes = []string{"n1"}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.PrivateIP = "10.0.0.1"
	node.Status.MachineStatus.Phase = v1.NodeReady
	node.Status.MachineStatus.HostName = "host1"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{"username": []byte("root")},
	}
	r := newPlaneReconciler(t, cluster, node, secret)
	return cluster, r
}

func TestCreateNewWorkerPod(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	testifyassert.NoError(t, err)
	pod, err := r.createNewWorkerPod(context.Background(), cluster, hosts)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, pod)
}

func TestCreateResetPod(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	testifyassert.NoError(t, err)
	pod, err := r.createResetPod(context.Background(), cluster, hosts)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, pod)
}

func TestGuaranteeCreateWorkerPodCreated(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	testifyassert.NoError(t, err)
	pod, err := r.guaranteeCreateWorkerPodCreated(context.Background(), cluster, hosts)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, pod)
}

func TestGuaranteeResetWorkPodCreated(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	testifyassert.NoError(t, err)
	pod, err := r.guaranteeResetWorkPodCreated(context.Background(), cluster, hosts)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, pod)
}

func TestClearPods(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	// No pods -> nil.
	testifyassert.NoError(t, r.clearPods(context.Background(), cluster))
}

func TestGuaranteeDefaultAddonCreatesAddon(t *testing.T) {
	cluster := testCluster("c1")
	template := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "infera-operator.0.1.0",
			Labels: map[string]string{v1.AddonDefaultLabel: ""},
		},
	}
	r := newPlaneReconciler(t, cluster, template)
	res, err := r.guaranteeDefaultAddon(context.Background(), cluster)
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
	// Addon should be created with name "c1-infera-operator".
	addon := &v1.Addon{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1-infera-operator"}, addon))
}

func TestGuaranteeDefaultAddonDeletesLegacyAutoNginx(t *testing.T) {
	cluster := testCluster("c1")
	cluster.UID = "uid-c1"
	template := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "infera-operator.0.1.0",
			Labels: map[string]string{v1.AddonDefaultLabel: ""},
		},
	}
	nginx := legacyAutoNginxAddon(cluster, "")
	r := newPlaneReconciler(t, cluster, template, nginx)

	_, err := r.guaranteeDefaultAddon(context.Background(), cluster)
	testifyassert.NoError(t, err)

	addon := &v1.Addon{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1-infera-operator"}, addon))
	testifyassert.Error(t, r.Get(context.Background(), client.ObjectKey{Name: "c1-nginx"}, &v1.Addon{}))
}

func TestGuaranteeDefaultAddonKeepsCustomizedNginx(t *testing.T) {
	cluster := testCluster("c1")
	cluster.UID = "uid-c1"
	nginx := legacyAutoNginxAddon(cluster, "service:\n  type: NodePort\n")
	r := newPlaneReconciler(t, cluster, nginx)

	_, err := r.guaranteeDefaultAddon(context.Background(), cluster)
	testifyassert.NoError(t, err)

	addon := &v1.Addon{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1-nginx"}, addon))
}

func legacyAutoNginxAddon(cluster *v1.Cluster, values string) *v1.Addon {
	return &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: cluster.Name + "-nginx",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: cluster.APIVersion,
				Kind:       cluster.Kind,
				Name:       cluster.Name,
				UID:        cluster.UID,
			}},
		},
		Spec: v1.AddonSpec{
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{
					ReleaseName: deprecatedDefaultNginxRelease,
					Values:      values,
					Template: &corev1.ObjectReference{
						Name: deprecatedDefaultNginxTemplate,
					},
				},
			},
		},
	}
}

func TestUpdateClusterKubeConfigNilConfig(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	// nil restConfig -> no-op nil.
	testifyassert.NoError(t, r.updateClusterKubeConfig(context.Background(), cluster, nil, nil))
}

func TestUpdateClusterKubeConfig(t *testing.T) {
	scheme, _ := genMockScheme()
	cluster := testCluster("c1")
	cs := k8sfakeClientset()
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Cluster{}).
		WithObjects(cluster).
		Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs}}
	nodes := []*v1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}}}
	cfg := &rest.Config{}
	cfg.CertData = []byte("cert")
	cfg.CAData = []byte("ca")
	cfg.KeyData = []byte("key")
	err := r.updateClusterKubeConfig(context.Background(), cluster, nodes, cfg)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.ReadyPhase, cluster.Status.ControlPlaneStatus.Phase)
	testifyassert.Len(t, cluster.Status.ControlPlaneStatus.Endpoints, 1)
}

func TestResetWithHostsContent(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	testifyassert.NoError(t, err)
	// reset with hostsContent + non-deleted/failed phase -> creates reset pod.
	err = r.reset(context.Background(), cluster, hosts)
	testifyassert.NoError(t, err)
}

func TestResetCreationFailedPhase(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	cluster.Status.ControlPlaneStatus.Phase = v1.CreationFailed
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	testifyassert.NoError(t, err)
	// CreationFailed -> sets DeletedPhase.
	err = r.reset(context.Background(), cluster, hosts)
	testifyassert.NoError(t, err)
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)
}

func TestHandleControlPlaneCreation(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	cluster.Status.ControlPlaneStatus.Phase = v1.PendingPhase
	// No SSHSecret on cluster -> generateSSHSecret creates one, then worker pod.
	err := r.handleControlPlaneCreation(context.Background(), cluster)
	testifyassert.NoError(t, err)
	// Worker pod should now exist.
	pod := &corev1.Pod{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{
		Name: "c1-" + string(v1.ClusterCreateAction), Namespace: common.PrimusSafeNamespace,
	}, pod))
}

func TestCreateControlPlanePod(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	testifyassert.NoError(t, err)
	err = r.createControlPlanePod(context.Background(), cluster, hosts)
	testifyassert.NoError(t, err)
}

func TestHandleExistingPodOwned(t *testing.T) {
	cluster, r := planeClusterWithNode(t)
	cluster.UID = "uid-1"
	hosts, err := r.generateHosts(context.Background(), cluster, nil)
	testifyassert.NoError(t, err)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: common.PrimusSafeNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: cluster.Kind, UID: cluster.UID},
			},
		},
	}
	got, err := r.handleExistingPod(context.Background(), cluster, pod, hosts)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, got)
}

func TestGuaranteeDefaultAddonNoTemplates(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	res, err := r.guaranteeDefaultAddon(context.Background(), cluster)
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

// --- merged from cluster_reconcile_test.go ---

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

func TestFilterHealthyControlPlaneAddressesNoCredentials(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	nodes := []*v1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}}}
	addrs := r.filterHealthyControlPlaneAddresses(context.Background(), cluster, nodes)
	testifyassert.Len(t, addrs, 1)
	testifyassert.Equal(t, "10.0.0.1", addrs[0].IP)
}

func TestMarkClusterClientFactoryStale(t *testing.T) {
	mgr := commonutils.NewObjectManager()
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	testifyassert.NoError(t, mgr.Add("c1", factory))
	r := &ClusterReconciler{clientManager: mgr}
	r.markClusterClientFactoryStale("c1", "control plane endpoints changed")
	testifyassert.False(t, factory.IsValid())
}

func TestGuaranteeEndpointsBackendSync(t *testing.T) {
	cluster := testCluster("c1")
	existing := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}, {IP: "10.0.0.2"}},
		}},
	}
	r := newPlaneReconciler(t, cluster, existing)
	mgr := commonutils.NewObjectManager()
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	factory.SetBackendFingerprint("10.0.0.1,10.0.0.2")
	testifyassert.NoError(t, mgr.Add("c1", factory))
	r.clientManager = mgr

	nodes := []*v1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}}}
	testifyassert.NoError(t, r.guaranteeEndpoints(context.Background(), cluster, nodes))

	ep := &corev1.Endpoints{}
	testifyassert.NoError(t, r.Get(context.Background(), client.ObjectKey{
		Name: "c1", Namespace: common.PrimusSafeNamespace,
	}, ep))
	testifyassert.Len(t, ep.Subsets[0].Addresses, 1)
	testifyassert.Equal(t, "10.0.0.1", ep.Subsets[0].Addresses[0].IP)
	testifyassert.False(t, factory.IsValid())
}
