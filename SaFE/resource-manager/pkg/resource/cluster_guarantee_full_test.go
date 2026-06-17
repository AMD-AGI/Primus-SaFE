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
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

// newClusterReconcilerFull builds a ClusterReconciler whose admin client (ctrl
// fake) holds objs, with both r.clientSet and the data-plane factory backed by
// the given clientset.
func newClusterReconcilerFull(t *testing.T, cs *k8sfake.Clientset, objs ...ctrlclient.Object) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	_, err = cs.RbacV1().ClusterRoleBindings().Get(context.Background(), "cicd-role", metav1.GetOptions{})
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	_, err = cs.NetworkingV1().Ingresses(common.PrimusSafeNamespace).Get(context.Background(), "c1-forward", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestGuaranteeDataPlaneClusterRoleFull(t *testing.T) {
	adminRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-role", Labels: map[string]string{"a": "b"}},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, APIGroups: []string{""}, Resources: []string{"pods"}}},
	}
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, adminRole)
	err := r.guaranteeDataPlaneClusterRole(context.Background(), testCluster("c1"), "dp-role")
	assert.NoError(t, err)
	_, err = cs.RbacV1().ClusterRoles().Get(context.Background(), "dp-role", metav1.GetOptions{})
	assert.NoError(t, err)
	// second call should update path (already exists in data plane)
	assert.NoError(t, r.guaranteeDataPlaneClusterRole(context.Background(), testCluster("c1"), "dp-role"))

	assert.NoError(t, r.deleteDataPlaneClusterRole(context.Background(), testCluster("c1"), "dp-role"))
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

	assert.NoError(t, r.cleanupClusterResources(ctx, cluster))
	assert.NoError(t, r.delete(ctx, cluster))
	got := &v1.Cluster{}
	assert.NoError(t, r.Get(ctx, ctrlclient.ObjectKey{Name: "c1"}, got))
	assert.NotContains(t, got.Finalizers, v1.ClusterFinalizer)
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
	assert.NoError(t, err)
	got := &corev1.Secret{}
	assert.NoError(t, r.Get(context.Background(), ctrlclient.ObjectKey{Name: "c1", Namespace: common.PrimusSafeNamespace}, got))
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
	assert.NoError(t, r.clearPods(ctx, testCluster("c1")))
	got := &corev1.Pod{}
	assert.Error(t, r.Get(ctx, ctrlclient.ObjectKey{Name: "old", Namespace: common.PrimusSafeNamespace}, got))
	assert.NoError(t, r.Get(ctx, ctrlclient.ObjectKey{Name: "running", Namespace: common.PrimusSafeNamespace}, got))
}

func TestClusterReconcileReadyNoControlPlaneNodes(t *testing.T) {
	cluster := readyCluster("c1")
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, cluster)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
	assert.NoError(t, err)
	// priority classes should have been created in the data plane
	pcs, err := cs.SchedulingV1().PriorityClasses().List(context.Background(), metav1.ListOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, pcs.Items)
}

func TestClusterReconcileDeletePhase(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Finalizers = []string{v1.ClusterFinalizer}
	cluster.Status.ControlPlaneStatus.Phase = v1.DeletedPhase
	cs := k8sfake.NewSimpleClientset()
	r := newClusterReconcilerFull(t, cs, cluster)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
	assert.NoError(t, err)
}

func TestControlPlaneStatusHelpers(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	r := newClusterReconcilerFull(t, cs, cluster)
	ctx := context.Background()

	// updatePodStatus: succeeded -> CreatedPhase
	succeeded := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}
	assert.NoError(t, r.updatePodStatus(ctx, cluster, succeeded))
	assert.Equal(t, v1.CreatedPhase, cluster.Status.ControlPlaneStatus.Phase)

	// updatePodStatus: failed -> CreationFailed
	failed := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}
	assert.NoError(t, r.updatePodStatus(ctx, cluster, failed))
	assert.Equal(t, v1.CreationFailed, cluster.Status.ControlPlaneStatus.Phase)

	// updateResetPhase variants (pure)
	r.updateResetPhase(cluster, succeeded)
	assert.Equal(t, v1.DeletedPhase, cluster.Status.ControlPlaneStatus.Phase)
	r.updateResetPhase(cluster, failed)
	assert.Equal(t, v1.DeleteFailedPhase, cluster.Status.ControlPlaneStatus.Phase)
	r.updateResetPhase(cluster, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}})
	assert.Equal(t, v1.DeletingPhase, cluster.Status.ControlPlaneStatus.Phase)

	// reset with nil hosts -> DeletedPhase
	assert.NoError(t, r.reset(ctx, cluster, nil))
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
	assert.NoError(t, r.guaranteeMonarchClusterRole(context.Background(), testCluster("c1")))
	assert.NoError(t, r.deleteMonarchClusterRole(context.Background(), testCluster("c1")))
}
