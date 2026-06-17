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
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func newSecretReconciler(t *testing.T, objs ...client.Object) *SecretReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &SecretReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		clientManager:         commonutils.NewObjectManager(),
	}
}

func TestCompareSecretData(t *testing.T) {
	a := map[string][]byte{"k": []byte("v")}
	b := map[string][]byte{"k": []byte("v")}
	assert.True(t, compareSecretData(a, b))
	assert.False(t, compareSecretData(a, map[string][]byte{"k": []byte("x")}))
	assert.False(t, compareSecretData(a, map[string][]byte{}))
	assert.False(t, compareSecretData(a, map[string][]byte{"other": []byte("v")}))
}

func TestRelevantChangePredicateCreate(t *testing.T) {
	p := relevantChangePredicate{}
	// Wrong namespace -> false.
	s1 := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "other"}}
	assert.False(t, p.Create(event.CreateEvent{Object: s1}))
	// Correct namespace + annotation -> true.
	s2 := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Namespace:   common.PrimusSafeNamespace,
		Annotations: map[string]string{v1.WorkspaceIdsAnnotation: "ws1"},
	}}
	assert.True(t, p.Create(event.CreateEvent{Object: s2}))
}

func TestRelevantChangePredicateUpdate(t *testing.T) {
	p := relevantChangePredicate{}
	old := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: common.PrimusSafeNamespace}}
	upd := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: common.PrimusSafeNamespace},
		Type:       corev1.SecretTypeDockerConfigJson,
	}
	// Type changed -> true.
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: upd}))
	// No change -> false.
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
}

func TestSecretReconcileNotFound(t *testing.T) {
	r := newSecretReconciler(t)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing", Namespace: common.PrimusSafeNamespace}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestGetClientSetOfDataplaneEmpty(t *testing.T) {
	r := newSecretReconciler(t)
	cs, err := r.getClientSetOfDataplane(context.Background(), "")
	assert.NoError(t, err)
	assert.Nil(t, cs)
}

func TestGetClientSetOfDataplaneClusterMissing(t *testing.T) {
	r := newSecretReconciler(t)
	_, err := r.getClientSetOfDataplane(context.Background(), "missing")
	assert.Error(t, err)
}

func TestCopySecret(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s1"},
		Data:       map[string][]byte{"k": []byte("v")},
	}
	err := copySecret(context.Background(), cs, secret, "ns1")
	assert.NoError(t, err)
	created, err := cs.CoreV1().Secrets("ns1").Get(context.Background(), "s1", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, managedSecretLabelVal, created.Labels[managedSecretLabelKey])
}

func TestSyncSecretToWorkspaceCreate(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newSecretReconciler(t)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1"}, Data: map[string][]byte{"k": []byte("v")}}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	err := r.syncSecretToWorkspace(context.Background(), cs, secret, ws)
	assert.NoError(t, err)
	_, err = cs.CoreV1().Secrets("ws1").Get(context.Background(), "s1", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestSyncSecretToWorkspaceUpdate(t *testing.T) {
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "ws1"},
		Data:       map[string][]byte{"k": []byte("old")},
	}
	cs := k8sfake.NewSimpleClientset(existing)
	r := newSecretReconciler(t)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1"}, Data: map[string][]byte{"k": []byte("new")}}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	err := r.syncSecretToWorkspace(context.Background(), cs, secret, ws)
	assert.NoError(t, err)
	updated, _ := cs.CoreV1().Secrets("ws1").Get(context.Background(), "s1", metav1.GetOptions{})
	assert.Equal(t, "new", string(updated.Data["k"]))
}

func TestRemoveSecretFromWorkspace(t *testing.T) {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{ImageSecrets: []corev1.ObjectReference{
			{Name: "s1"}, {Name: "s2"},
		}},
	}
	r := newSecretReconciler(t, ws)
	err := r.removeSecretFromWorkspace(context.Background(), "s1", ws)
	assert.NoError(t, err)
	assert.Len(t, ws.Spec.ImageSecrets, 1)
}

func TestRemoveSecretFromCluster(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cluster.Spec.ControlPlane.ImageSecret = &corev1.ObjectReference{Name: "s1"}
	r := newSecretReconciler(t, cluster)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1"}}
	err := r.removeSecretFromCluster(context.Background(), secret)
	assert.NoError(t, err)
	updated := &v1.Cluster{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "c1"}, updated))
	assert.Nil(t, updated.Spec.ControlPlane.ImageSecret)
}

func TestUpdateWorkspaceRefSecret(t *testing.T) {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{ImageSecrets: []corev1.ObjectReference{
			{Name: "s1", ResourceVersion: "1"},
		}},
	}
	r := newSecretReconciler(t, ws)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1", ResourceVersion: "2"}}
	err := r.updateWorkspaceRefSecret(context.Background(), secret, ws)
	assert.NoError(t, err)
}

func TestUpdateClusterRefSecret(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	cluster.Spec.ControlPlane.ImageSecret = &corev1.ObjectReference{Name: "s1", ResourceVersion: "1"}
	r := newSecretReconciler(t, cluster)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1", ResourceVersion: "2"}}
	err := r.updateClusterRefSecret(context.Background(), secret)
	assert.NoError(t, err)
}

func TestRemoveSecretFromWorkspaces(t *testing.T) {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{ImageSecrets: []corev1.ObjectReference{{Name: "s1"}}},
	}
	r := newSecretReconciler(t, ws)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s1", Annotations: map[string]string{v1.WorkspaceIdsAnnotation: "ws1"}},
	}
	err := r.removeSecretFromWorkspaces(context.Background(), secret)
	assert.NoError(t, err)
}

func TestSecretDelete(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: common.PrimusSafeNamespace, Finalizers: []string{v1.SecretFinalizer}},
	}
	r := newSecretReconciler(t, secret)
	// No cluster/workspace refs -> cleanup paths no-op, removes finalizer.
	err := r.delete(context.Background(), secret)
	assert.NoError(t, err)
}

func TestProcessSecretsNoWorkspaces(t *testing.T) {
	r := newSecretReconciler(t)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: common.PrimusSafeNamespace}}
	_, err := r.processSecrets(context.Background(), secret)
	assert.NoError(t, err)
}
