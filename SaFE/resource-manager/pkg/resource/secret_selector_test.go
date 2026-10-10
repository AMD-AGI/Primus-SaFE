/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const sandboxSelector = v1.WorkspaceSandboxScopeLabel + "=true"

func selectorWorkspace(name string, lbls map[string]string) *v1.Workspace {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: lbls}}
	ws.Spec.Cluster = "c1"
	return ws
}

func sandboxWorkspace(name string) *v1.Workspace {
	return selectorWorkspace(name, map[string]string{v1.WorkspaceSandboxScopeLabel: v1.TrueStr})
}

func selectorSecret(name, selector string, ids string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   common.PrimusSafeNamespace,
			Annotations: map[string]string{v1.WorkspaceSelectorAnnotation: selector},
		},
		Data: map[string][]byte{"ca.crt": []byte("pem")},
		Type: corev1.SecretTypeOpaque,
	}
	if ids != "" {
		s.Annotations[v1.WorkspaceIdsAnnotation] = ids
	}
	return s
}

func managedCopy(name, ns string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: ns, Labels: map[string]string{managedSecretLabelKey: managedSecretLabelVal}}}
}

func reconcileSecret(t *testing.T, r *SecretReconciler, name string) {
	t.Helper()
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: name}})
	require.NoError(t, err)
}

// mirrored returns whether ns holds a copy of name made by the controller, and fails on a
// Secret of that name the controller did not make.
func mirrored(t *testing.T, cs *k8sfake.Clientset, ns, name string) bool {
	t.Helper()
	got, err := cs.CoreV1().Secrets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	require.Equal(t, managedSecretLabelVal, got.Labels[managedSecretLabelKey], "%s/%s is not a mirror", ns, name)
	return true
}

func TestSelectorMirrorsOnlyMatchingWorkspaces(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec,
		sandboxWorkspace("ws-a"), sandboxWorkspace("ws-b"),
		selectorWorkspace("ws-train", map[string]string{"other": "true"}))
	reconcileSecret(t, r, sec.Name)

	assert.True(t, mirrored(t, cs, "ws-a", sec.Name))
	assert.True(t, mirrored(t, cs, "ws-b", sec.Name))
	assert.False(t, mirrored(t, cs, "ws-train", sec.Name))
	got, err := cs.CoreV1().Secrets("ws-a").Get(context.Background(), sec.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, sec.Data, got.Data)
	// The selector grants nothing: the ids list, which other checks read as authorisation,
	// is left as it was.
	stored := &corev1.Secret{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(sec), stored))
	assert.False(t, v1.HasAnnotation(stored, v1.WorkspaceIdsAnnotation))
}

func TestSelectorWorkspaceCreatedLaterGetsCopyWithoutTouchingSecret(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	unrelated := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: common.PrimusSafeNamespace}}
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, unrelated)
	reconcileSecret(t, r, sec.Name)

	late := sandboxWorkspace("ws-late")
	require.NoError(t, r.Create(context.Background(), late))
	assert.True(t, workspaceMembershipPredicate{}.Create(event.CreateEvent{Object: late}))
	requests := r.secretsForWorkspace(context.Background(), late)
	require.Len(t, requests, 1, "only the Secret carrying a selector is enqueued")
	assert.Equal(t, sec.Name, requests[0].Name)
	assert.Equal(t, common.PrimusSafeNamespace, requests[0].Namespace)

	_, err := r.Reconcile(context.Background(), requests[0])
	require.NoError(t, err)
	assert.True(t, mirrored(t, cs, "ws-late", sec.Name))
}

func TestIdsListedWorkspaceCreatedLaterIsEnqueued(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	listed := boundSecret("listed")
	listed.Namespace = common.PrimusSafeNamespace
	other := boundSecret("other")
	other.Namespace = common.PrimusSafeNamespace
	other.Annotations[v1.WorkspaceIdsAnnotation] = `["ws2"]`
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), listed, other)
	reconcileSecret(t, r, listed.Name)
	assert.False(t, mirrored(t, cs, "ws1", listed.Name))

	ws := selectorWorkspace("ws1", nil)
	require.NoError(t, r.Create(context.Background(), ws))
	requests := r.secretsForWorkspace(context.Background(), ws)
	require.Len(t, requests, 1)
	assert.Equal(t, listed.Name, requests[0].Name)
	_, err := r.Reconcile(context.Background(), requests[0])
	require.NoError(t, err)
	assert.True(t, mirrored(t, cs, "ws1", listed.Name))
}

func TestSelectorLabelRemovedRemovesOnlyOwnCopy(t *testing.T) {
	handMade := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "extra-ca", Namespace: "ws-own"},
		Data: map[string][]byte{"mine": []byte("x")}}
	cs := k8sfake.NewSimpleClientset(handMade)
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	ws := sandboxWorkspace("ws-a")
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, ws, selectorWorkspace("ws-own", nil))
	reconcileSecret(t, r, sec.Name)
	require.True(t, mirrored(t, cs, "ws-a", sec.Name))

	current := &v1.Workspace{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(ws), current))
	old := current.DeepCopy()
	v1.RemoveLabel(current, v1.WorkspaceSandboxScopeLabel)
	require.NoError(t, r.Update(context.Background(), current))
	assert.True(t, workspaceMembershipPredicate{}.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: current}))
	assert.False(t, workspaceMembershipPredicate{}.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
	requests := r.secretsForWorkspace(context.Background(), current)
	require.Len(t, requests, 1)
	_, err := r.Reconcile(context.Background(), requests[0])
	require.NoError(t, err)

	assert.False(t, mirrored(t, cs, "ws-a", sec.Name))
	kept, err := cs.CoreV1().Secrets("ws-own").Get(context.Background(), "extra-ca", metav1.GetOptions{})
	require.NoError(t, err, "a Secret the controller did not make is never deleted")
	assert.Equal(t, handMade.Data, kept.Data)
}

func TestSelectorDoesNotTakeOverHandMadeSecret(t *testing.T) {
	handMade := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "extra-ca", Namespace: "ws-a"},
		Data: map[string][]byte{"mine": []byte("x")}}
	cs := k8sfake.NewSimpleClientset(handMade)
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	recorder := record.NewFakeRecorder(10)
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"))
	r.recorder = recorder
	reconcileSecret(t, r, sec.Name)

	got, err := cs.CoreV1().Secrets("ws-a").Get(context.Background(), "extra-ca", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, handMade.Data, got.Data)
	assert.Empty(t, got.Labels[managedSecretLabelKey])
	require.Len(t, recorder.Events, 1)
	assert.Contains(t, <-recorder.Events, "MirrorConflict")
}

func TestSelectorExcludesWorkspaceBeingDeleted(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	dying := sandboxWorkspace("ws-dying")
	dying.Finalizers = []string{v1.WorkspaceFinalizer}
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, dying)
	require.NoError(t, r.Delete(context.Background(), dying))
	reconcileSecret(t, r, sec.Name)
	assert.False(t, mirrored(t, cs, "ws-dying", sec.Name))
}

func TestSelectorUnionWithIds(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-stale"))
	sec := selectorSecret("extra-ca", sandboxSelector, `["ws-listed"]`)
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec,
		sandboxWorkspace("ws-a"), selectorWorkspace("ws-listed", nil), selectorWorkspace("ws-stale", nil))
	reconcileSecret(t, r, sec.Name)
	assert.True(t, mirrored(t, cs, "ws-a", sec.Name))
	assert.True(t, mirrored(t, cs, "ws-listed", sec.Name))
	assert.False(t, mirrored(t, cs, "ws-stale", sec.Name))
}

func TestIdsOnlyBehaviourUnchanged(t *testing.T) {
	// A listed workspace takes over a same-named Secret, as before; a copy outside the list
	// is removed; a workspace carrying the label the selector would use is not touched.
	handMade := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "ws1"},
		Data: map[string][]byte{"k": []byte("old")}}
	cs := k8sfake.NewSimpleClientset(handMade, managedCopy("s1", "ws-stale"))
	sec := boundSecret("s1")
	sec.Namespace = common.PrimusSafeNamespace
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec,
		selectorWorkspace("ws1", nil), selectorWorkspace("ws-stale", nil), sandboxWorkspace("ws-sandbox"))
	reconcileSecret(t, r, sec.Name)
	assert.True(t, mirrored(t, cs, "ws1", sec.Name))
	got, err := cs.CoreV1().Secrets("ws1").Get(context.Background(), "s1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "v", string(got.Data["k"]))
	assert.False(t, mirrored(t, cs, "ws-stale", sec.Name))
	assert.False(t, mirrored(t, cs, "ws-sandbox", sec.Name))
}

func TestEmptySelectorMatchesNothing(t *testing.T) {
	for _, selector := range []string{"", "   "} {
		t.Run("selector="+selector, func(t *testing.T) {
			cs := k8sfake.NewSimpleClientset()
			sec := selectorSecret("extra-ca", selector, "")
			r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec,
				sandboxWorkspace("ws-a"), selectorWorkspace("ws-b", nil))
			assert.True(t, relevantChangePredicate{}.Create(event.CreateEvent{Object: sec}))
			reconcileSecret(t, r, sec.Name)
			assert.False(t, mirrored(t, cs, "ws-a", sec.Name))
			assert.False(t, mirrored(t, cs, "ws-b", sec.Name))
		})
	}
}

func TestInvalidSelectorMirrorsNothingAndRemovesNothing(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-a"))
	sec := selectorSecret("extra-ca", v1.WorkspaceSandboxScopeLabel+" in (true", `["ws-listed"]`)
	recorder := record.NewFakeRecorder(10)
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec,
		sandboxWorkspace("ws-a"), selectorWorkspace("ws-b", nil), selectorWorkspace("ws-listed", nil))
	r.recorder = recorder
	reconcileSecret(t, r, sec.Name)

	assert.True(t, mirrored(t, cs, "ws-listed", sec.Name), "the ids list still applies")
	assert.False(t, mirrored(t, cs, "ws-b", sec.Name), "an invalid selector selects nothing")
	assert.True(t, mirrored(t, cs, "ws-a", sec.Name), "nothing is removed while the selector is invalid")
	require.Len(t, recorder.Events, 1)
	assert.True(t, strings.Contains(<-recorder.Events, "InvalidWorkspaceSelector"))
}

func TestSecretPredicatesSeeSelector(t *testing.T) {
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	assert.True(t, relevantChangePredicate{}.Create(event.CreateEvent{Object: sec}))
	changed := sec.DeepCopy()
	changed.Annotations[v1.WorkspaceSelectorAnnotation] = "a=b"
	assert.True(t, relevantChangePredicate{}.Update(event.UpdateEvent{ObjectOld: sec, ObjectNew: changed}))
	removed := sec.DeepCopy()
	delete(removed.Annotations, v1.WorkspaceSelectorAnnotation)
	assert.True(t, relevantChangePredicate{}.Update(event.UpdateEvent{ObjectOld: sec, ObjectNew: removed}))
	assert.False(t, relevantChangePredicate{}.Update(event.UpdateEvent{ObjectOld: sec, ObjectNew: sec.DeepCopy()}))
}
