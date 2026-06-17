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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func newSecretReconcilerFull(t *testing.T, cs *k8sfake.Clientset, objs ...ctrlclient.Object) *SecretReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	mgr := commonutils.NewObjectManager()
	_ = mgr.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))
	return &SecretReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl, clientSet: cs},
		clientManager:         mgr,
	}
}

func boundSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Annotations: map[string]string{v1.WorkspaceIdsAnnotation: `["ws1"]`},
		},
		Data: map[string][]byte{"k": []byte("v")},
		Type: corev1.SecretTypeOpaque,
	}
}

func TestProcessSecretsMirror(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	cluster := testCluster("c1")
	cs := k8sfake.NewSimpleClientset()
	r := newSecretReconcilerFull(t, cs, ws, cluster)
	_, err := r.processSecrets(context.Background(), boundSecret("s1"))
	assert.NoError(t, err)
	// mirrored secret should be created in data plane under workspace namespace
	_, err = cs.CoreV1().Secrets("ws1").Get(context.Background(), "s1", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestProcessSecretsNoWorkspace(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := newSecretReconcilerFull(t, cs)
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s2", Namespace: "default"}}
	_, err := r.processSecrets(context.Background(), sec)
	assert.NoError(t, err)
}

func TestCleanupMirroredSecretsFull(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	cluster := testCluster("c1")
	// pre-create a managed mirrored secret in data plane
	mirrored := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s1",
			Namespace: "ws1",
			Labels:    map[string]string{managedSecretLabelKey: managedSecretLabelVal},
		},
	}
	cs := k8sfake.NewSimpleClientset(mirrored)
	r := newSecretReconcilerFull(t, cs, ws, cluster)
	err := r.cleanupMirroredSecrets(context.Background(), "s1", nil)
	assert.NoError(t, err)
	_, err = cs.CoreV1().Secrets("ws1").Get(context.Background(), "s1", metav1.GetOptions{})
	assert.Error(t, err)
}

func TestSecretDeleteFull(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := boundSecret("s1")
	sec.Finalizers = []string{v1.SecretFinalizer}
	r := newSecretReconcilerFull(t, cs, sec)
	err := r.delete(context.Background(), sec)
	assert.NoError(t, err)
}

func TestSecretReconcileEntry(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	cluster := testCluster("c1")
	sec := boundSecret("s1")
	cs := k8sfake.NewSimpleClientset()
	r := newSecretReconcilerFull(t, cs, ws, cluster, sec)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "s1", Namespace: "default"}})
	assert.NoError(t, err)

	// not found path
	_, err = r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"}})
	assert.NoError(t, err)
}

func TestProcessSecretsUpdatePath(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	cluster := testCluster("c1")
	// pre-existing mirrored secret in data plane with stale data -> update branch
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "ws1"},
		Data:       map[string][]byte{"k": []byte("old")},
		Type:       corev1.SecretTypeOpaque,
	}
	cs := k8sfake.NewSimpleClientset(existing)
	r := newSecretReconcilerFull(t, cs, ws, cluster)
	_, err := r.processSecrets(context.Background(), boundSecret("s1"))
	assert.NoError(t, err)
	got, err := cs.CoreV1().Secrets("ws1").Get(context.Background(), "s1", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "v", string(got.Data["k"]))
}

func TestRemoveSecretFromWorkspacesFull(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	ws.Spec.ImageSecrets = []corev1.ObjectReference{{Name: "s1"}, {Name: "other"}}
	cs := k8sfake.NewSimpleClientset()
	r := newSecretReconcilerFull(t, cs, ws)
	assert.NoError(t, r.removeSecretFromWorkspaces(context.Background(), boundSecret("s1")))
}

func TestUpdateClusterRefSecretFull(t *testing.T) {
	cluster := testCluster("c1")
	cluster.Spec.ControlPlane.ImageSecret = &corev1.ObjectReference{Name: "s1", ResourceVersion: "old"}
	cs := k8sfake.NewSimpleClientset()
	r := newSecretReconcilerFull(t, cs, cluster)
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1", ResourceVersion: "new"}}
	assert.NoError(t, r.updateClusterRefSecret(context.Background(), sec))
	assert.NoError(t, r.removeSecretFromCluster(context.Background(), sec))
}
