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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const sandboxSelector = v1.WorkspaceSandboxScopeLabel + "=true"

// selectorWorkspace is a workspace that does not accept Sandbox workloads.
func selectorWorkspace(name string, lbls map[string]string) *v1.Workspace {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: lbls}}
	ws.Spec.Cluster = "c1"
	ws.Spec.Scopes = []v1.WorkspaceScope{v1.TrainScope}
	return ws
}

func sandboxWorkspace(name string) *v1.Workspace {
	ws := selectorWorkspace(name, map[string]string{v1.WorkspaceSandboxScopeLabel: v1.TrueStr})
	ws.Spec.Scopes = []v1.WorkspaceScope{v1.SandboxScope}
	return ws
}

func selectorSecret(name, selector string, ids string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   common.PrimusSafeNamespace,
			Annotations: map[string]string{v1.WorkspaceSelectorAnnotation: selector, v1.SecretBroadcastAnnotation: v1.TrueStr},
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

func TestSelectorScopeRemovedRemovesOnlyOwnCopy(t *testing.T) {
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
	// Sandbox dropped from the scopes; the webhook removes the label with it.
	current.Spec.Scopes = []v1.WorkspaceScope{v1.TrainScope}
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
			assert.False(t, hasMirrorTargets(sec), "a blank selector is no target")
			reconcileSecret(t, r, sec.Name)
			assert.False(t, mirrored(t, cs, "ws-a", sec.Name))
			assert.False(t, mirrored(t, cs, "ws-b", sec.Name))
		})
	}
}

func TestUnparsableSelectorMirrorsNothingAndFreezes(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-a"))
	sec := selectorSecret("extra-ca", v1.WorkspaceSandboxScopeLabel+" in (true", `["ws-listed"]`)
	sec.Annotations[v1.WorkspaceSelectorAppliedAnnotation] = `["ws-a"]` // it was valid once
	recorder := record.NewFakeRecorder(10)
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec,
		sandboxWorkspace("ws-a"), selectorWorkspace("ws-b", nil), selectorWorkspace("ws-listed", nil))
	r.recorder = recorder
	reconcileSecret(t, r, sec.Name)

	assert.True(t, mirrored(t, cs, "ws-listed", sec.Name), "the ids list still applies")
	assert.False(t, mirrored(t, cs, "ws-b", sec.Name), "an invalid selector selects nothing")
	assert.True(t, mirrored(t, cs, "ws-a", sec.Name), "an invalid selector removes nothing")
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
	// A Secret with no annotation left but our finalizer, or being deleted, must still be seen.
	bare := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: common.PrimusSafeNamespace}}
	assert.False(t, relevantChangePredicate{}.Create(event.CreateEvent{Object: bare}))
	withFinalizer := bare.DeepCopy()
	withFinalizer.Finalizers = []string{v1.SecretFinalizer}
	assert.True(t, relevantChangePredicate{}.Create(event.CreateEvent{Object: withFinalizer}))
	deleting := bare.DeepCopy()
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	assert.True(t, relevantChangePredicate{}.Create(event.CreateEvent{Object: deleting}))
	// An operator who adds the broadcast mark after a freeze must get the Secret reconciled.
	unmarked := sec.DeepCopy()
	delete(unmarked.Annotations, v1.SecretBroadcastAnnotation)
	assert.True(t, relevantChangePredicate{}.Update(event.UpdateEvent{ObjectOld: unmarked, ObjectNew: sec}))
}

// A workspace written before the scope label existed carries none. The selector must still
// reach it, and must not delete the copy its sandboxes already mount; a hand-set label that
// its scopes contradict must not reach it either.
func TestSelectorReachesSandboxWorkspaceWithoutTheLabel(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-existing"))
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	existing := sandboxWorkspace("ws-existing")
	existing.Labels = nil
	allScopes := selectorWorkspace("ws-all-scopes", nil)
	allScopes.Spec.Scopes = nil
	forged := selectorWorkspace("ws-forged", map[string]string{v1.WorkspaceSandboxScopeLabel: v1.TrueStr})
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, existing, allScopes, forged)
	reconcileSecret(t, r, sec.Name)

	assert.True(t, mirrored(t, cs, "ws-existing", sec.Name), "the copy already there is kept")
	assert.True(t, mirrored(t, cs, "ws-all-scopes", sec.Name), "no scopes means every scope")
	assert.False(t, mirrored(t, cs, "ws-forged", sec.Name), "the label follows the scopes")
}

func TestWorkspaceScopeChangeEnqueues(t *testing.T) {
	old := selectorWorkspace("ws-a", nil)
	changed := old.DeepCopy()
	changed.Spec.Scopes = append(changed.Spec.Scopes, v1.SandboxScope)
	assert.True(t, workspaceMembershipPredicate{}.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: changed}))
	other := old.DeepCopy()
	other.Spec.Scopes = append(other.Spec.Scopes, v1.InferScope)
	assert.False(t, workspaceMembershipPredicate{}.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: other}))
}

// Deleting a Secret that names its workspaces only by selector removes the copies it made.
func TestDeleteSelectorOnlySecretRemovesCopies(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	sec.Finalizers = []string{v1.SecretFinalizer}
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"))
	reconcileSecret(t, r, sec.Name)
	require.True(t, mirrored(t, cs, "ws-a", sec.Name))

	require.NoError(t, r.Delete(context.Background(), sec))
	reconcileSecret(t, r, sec.Name)
	assert.False(t, mirrored(t, cs, "ws-a", sec.Name))
	gone := &corev1.Secret{}
	assert.True(t, apierrors.IsNotFound(r.Get(context.Background(), client.ObjectKeyFromObject(sec), gone)),
		"the finalizer is released")
}

func TestInvalidSelectorIsRejectedAndFrozen(t *testing.T) {
	// A workspace creator can set any label outside the primus-safe. prefix, so a selector on
	// one would hand the Secret to whoever sets it. A negative operator is true of a workspace
	// that lacks the label, so a misspelt reserved key would select every workspace.
	for _, selector := range []string{
		"team=infra",
		"team in (infra)",
		"!team",
		v1.WorkspaceSandboxScopeLabel + "=true,team=infra",
		"!primus-safe.workspace.scope.sandbxo",
		v1.WorkspaceSandboxScopeLabel + "!=false",
		v1.WorkspaceSandboxScopeLabel + " notin (false)",
		"!" + v1.WorkspaceSandboxScopeLabel,
		"(((",
		v1.PrimusSafePrefix + "display.name=ws-other",
		v1.PrimusSafePrefix + "display.name",
	} {
		t.Run(selector, func(t *testing.T) {
			cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-old"), managedCopy("extra-ca", "ws-sandbox"))
			sec := selectorSecret("extra-ca", selector, `["ws-listed"]`)
			sec.Annotations[v1.WorkspaceSelectorAppliedAnnotation] = `["ws-old","ws-sandbox"]` // it was valid once
			recorder := record.NewFakeRecorder(10)
			r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec,
				sandboxWorkspace("ws-sandbox"),
				selectorWorkspace("ws-team", map[string]string{"team": "infra"}),
				selectorWorkspace("ws-other", nil),
				selectorWorkspace("ws-old", nil), selectorWorkspace("ws-listed", nil))
			r.recorder = recorder
			reconcileSecret(t, r, sec.Name)

			assert.False(t, mirrored(t, cs, "ws-team", sec.Name))
			assert.False(t, mirrored(t, cs, "ws-other", sec.Name))
			assert.True(t, mirrored(t, cs, "ws-sandbox", sec.Name), "an invalid selector removes nothing")
			assert.True(t, mirrored(t, cs, "ws-old", sec.Name), "an invalid selector removes nothing")
			assert.True(t, mirrored(t, cs, "ws-listed", sec.Name), "the ids list still applies")
			require.Len(t, recorder.Events, 1)
			assert.Contains(t, <-recorder.Events, "InvalidWorkspaceSelector")
			got := &corev1.Secret{}
			require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(sec), got))
			assert.NotEmpty(t, got.Annotations[v1.WorkspaceSelectorErrorAnnotation], "the reason is visible on the Secret")
		})
	}
}

// Fixing the selector clears the error annotation and mirrors again; positive operators pass.
func TestValidSelectorClearsErrorAnnotation(t *testing.T) {
	for _, selector := range []string{
		sandboxSelector,
		v1.WorkspaceSandboxScopeLabel + "==true",
		v1.WorkspaceSandboxScopeLabel + " in (true)",
		v1.WorkspaceSandboxScopeLabel,
	} {
		t.Run(selector, func(t *testing.T) {
			cs := k8sfake.NewSimpleClientset()
			sec := selectorSecret("extra-ca", selector, "")
			sec.Annotations[v1.WorkspaceSelectorErrorAnnotation] = "stale"
			r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"))
			reconcileSecret(t, r, sec.Name)

			assert.True(t, mirrored(t, cs, "ws-a", sec.Name))
			got := &corev1.Secret{}
			require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(sec), got))
			_, has := got.Annotations[v1.WorkspaceSelectorErrorAnnotation]
			assert.False(t, has)
		})
	}
}

func TestOneTargetFailingDoesNotStarveOthersOrCleanup(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-a-bad"), managedCopy("extra-ca", "ws-stale"))
	// ws-a-bad stands for a workspace whose namespace is not there yet.
	cs.PrependReactor("*", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "ws-a-bad" && action.GetVerb() != "get" {
			return true, nil, apierrors.NewNotFound(corev1.Resource("namespaces"), "ws-a-bad")
		}
		return false, nil, nil
	})
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec,
		sandboxWorkspace("ws-a-bad"), sandboxWorkspace("ws-b-ok"), selectorWorkspace("ws-stale", nil))
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: sec.Name}})

	assert.Error(t, err, "the failure is returned so the default backoff applies")
	assert.Equal(t, ctrlruntime.Result{}, res)
	assert.True(t, mirrored(t, cs, "ws-b-ok", sec.Name), "a later workspace is still served")
	assert.False(t, mirrored(t, cs, "ws-stale", sec.Name), "cleanup still runs for workspaces no longer selected")
	assert.True(t, mirrored(t, cs, "ws-a-bad", sec.Name), "a copy whose state is unknown is kept")
}

func TestClusterNotReadyIsBackedOffNotHotLooped(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	notReady := sandboxWorkspace("ws-a-lost")
	notReady.Spec.Cluster = "c2"
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), testCluster("c2"), sec, notReady, sandboxWorkspace("ws-b-ok"))
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: sec.Name}})

	assert.Error(t, err)
	assert.Zero(t, res.RequeueAfter)
	assert.True(t, mirrored(t, cs, "ws-b-ok", sec.Name))
}

// A selector is honoured only on a Secret marked as safe to broadcast: scope.sandbox follows
// Spec.Scopes, which a workspace's editors can change.
func TestSelectorWithoutBroadcastMarkIsFrozen(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-a"))
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	delete(sec.Annotations, v1.SecretBroadcastAnnotation)
	sec.Annotations[v1.WorkspaceSelectorAppliedAnnotation] = `["ws-a"]` // it was valid once
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"), sandboxWorkspace("ws-b"))
	reconcileSecret(t, r, sec.Name)

	assert.False(t, mirrored(t, cs, "ws-b", sec.Name), "no new copy without the mark")
	assert.True(t, mirrored(t, cs, "ws-a", sec.Name), "existing copies stay")
	got := &corev1.Secret{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(sec), got))
	assert.Contains(t, got.Annotations[v1.WorkspaceSelectorErrorAnnotation], v1.SecretBroadcastAnnotation)
}

// A Secret made with kubectl has no finalizer; the reconcile adds one, so deleting it still
// removes the copies.
func TestDeleteKubectlCreatedSecretRemovesCopies(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	require.Empty(t, sec.Finalizers)
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"))
	reconcileSecret(t, r, sec.Name)
	require.True(t, mirrored(t, cs, "ws-a", sec.Name))

	live := &corev1.Secret{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(sec), live))
	require.Contains(t, live.Finalizers, v1.SecretFinalizer)
	require.NoError(t, r.Delete(context.Background(), live))
	reconcileSecret(t, r, sec.Name)
	assert.False(t, mirrored(t, cs, "ws-a", sec.Name))
}

// While the selector is invalid, the ids list still works both ways: a workspace removed from
// it loses its copy and reference, and the copies the selector made stay.
func TestFrozenSelectorStillHonoursIdsRemoval(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, `["ws-b","ws-c"]`)
	wsB := selectorWorkspace("ws-b", nil)
	wsB.Spec.ImageSecrets = []corev1.ObjectReference{{Name: "extra-ca"}}
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"), wsB, selectorWorkspace("ws-c", nil))
	reconcileSecret(t, r, sec.Name)
	for _, ws := range []string{"ws-a", "ws-b", "ws-c"} {
		require.True(t, mirrored(t, cs, ws, sec.Name), ws)
	}

	live := &corev1.Secret{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(sec), live))
	live.Annotations[v1.WorkspaceSelectorAnnotation] = "team=infra"
	live.Annotations[v1.WorkspaceIdsAnnotation] = `["ws-c"]`
	require.NoError(t, r.Update(context.Background(), live))
	reconcileSecret(t, r, sec.Name)

	assert.True(t, mirrored(t, cs, "ws-a", sec.Name), "the selector's copy is frozen")
	assert.True(t, mirrored(t, cs, "ws-c", sec.Name))
	assert.False(t, mirrored(t, cs, "ws-b", sec.Name), "removed from ids: copy reclaimed")
	got := &v1.Workspace{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ws-b"}, got))
	assert.Empty(t, got.Spec.ImageSecrets, "and the reference")
}

// A selector that was never valid mirrored nothing, so only the ids list is in play, and
// removing a workspace from it still reclaims its copy and reference.
func TestNeverValidSelectorDoesNotFreezeIdsRemoval(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", "", `["ws-b","ws-c"]`)
	delete(sec.Annotations, v1.WorkspaceSelectorAnnotation)
	wsB := selectorWorkspace("ws-b", nil)
	wsB.Spec.ImageSecrets = []corev1.ObjectReference{{Name: "extra-ca"}}
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, wsB, selectorWorkspace("ws-c", nil))
	reconcileSecret(t, r, sec.Name)
	require.True(t, mirrored(t, cs, "ws-b", sec.Name))

	live := &corev1.Secret{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(sec), live))
	live.Annotations[v1.WorkspaceSelectorAnnotation] = "team=infra"
	require.NoError(t, r.Update(context.Background(), live))
	reconcileSecret(t, r, sec.Name)
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(sec), live))
	require.Contains(t, live.Annotations, v1.WorkspaceSelectorErrorAnnotation)
	live.Annotations[v1.WorkspaceIdsAnnotation] = `["ws-c"]`
	require.NoError(t, r.Update(context.Background(), live))
	reconcileSecret(t, r, sec.Name)

	assert.True(t, mirrored(t, cs, "ws-c", sec.Name))
	assert.False(t, mirrored(t, cs, "ws-b", sec.Name), "removed from ids: copy reclaimed")
	got := &v1.Workspace{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "ws-b"}, got))
	assert.Empty(t, got.Spec.ImageSecrets)
}

func liveSecret(t *testing.T, r *SecretReconciler, name string) *corev1.Secret {
	t.Helper()
	got := &corev1.Secret{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: name}, got))
	return got
}

// Removing the selector annotation, as the docs say to stop mirroring, removes the copies and
// releases the finalizer added for them.
func TestRemovingSelectorReleasesFinalizer(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"))
	reconcileSecret(t, r, sec.Name)
	live := liveSecret(t, r, sec.Name)
	require.Contains(t, live.Finalizers, v1.SecretFinalizer)
	require.True(t, mirrored(t, cs, "ws-a", sec.Name))

	delete(live.Annotations, v1.WorkspaceSelectorAnnotation)
	require.NoError(t, r.Update(context.Background(), live))
	reconcileSecret(t, r, sec.Name)

	assert.False(t, mirrored(t, cs, "ws-a", sec.Name))
	assert.NotContains(t, liveSecret(t, r, sec.Name).Finalizers, v1.SecretFinalizer)
}

// A Secret a cluster still references keeps its finalizer, which clears that reference.
func TestFinalizerKeptWhileClusterReferencesSecret(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", "", "")
	delete(sec.Annotations, v1.WorkspaceSelectorAnnotation)
	sec.Finalizers = []string{v1.SecretFinalizer}
	c1 := testCluster("c1")
	c1.Spec.ControlPlane.ImageSecret = &corev1.ObjectReference{Name: "extra-ca"}
	r := newSecretReconcilerFull(t, cs, c1, sec)
	reconcileSecret(t, r, sec.Name)
	assert.Contains(t, liveSecret(t, r, sec.Name).Finalizers, v1.SecretFinalizer)
}

// One workspace on an unreachable cluster must not stop the others being reclaimed.
func TestCleanupSurvivesAnUnreachableCluster(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-stale"))
	sec := selectorSecret("extra-ca", "", `["ws-keep"]`)
	delete(sec.Annotations, v1.WorkspaceSelectorAnnotation)
	bad := selectorWorkspace("ws-a-bad", nil)
	bad.Spec.Cluster = "c2" // registered, but its client is not ready
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), testCluster("c2"), sec,
		bad, selectorWorkspace("ws-keep", nil), selectorWorkspace("ws-stale", nil))
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: sec.Name}})

	assert.Error(t, err, "the unreachable cluster is reported for retry")
	assert.Equal(t, ctrlruntime.Result{}, res)
	assert.False(t, mirrored(t, cs, "ws-stale", sec.Name), "the reachable workspace is still reclaimed")
}

// A frozen selector keeps syncing the copies it made, so a rotated CA reaches them, but adds
// and removes none.
func TestFrozenSelectorStillSyncsAppliedCopies(t *testing.T) {
	old := managedCopy("extra-ca", "ws-a")
	old.Data = map[string][]byte{"ca.crt": []byte("old")}
	cs := k8sfake.NewSimpleClientset(old)
	sec := selectorSecret("extra-ca", "team=infra", "")
	sec.Annotations[v1.WorkspaceSelectorAppliedAnnotation] = `["ws-a"]`
	sec.Data = map[string][]byte{"ca.crt": []byte("new")}
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"), sandboxWorkspace("ws-new"))
	reconcileSecret(t, r, sec.Name)

	got, err := cs.CoreV1().Secrets("ws-a").Get(context.Background(), "extra-ca", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []byte("new"), got.Data["ca.crt"], "content stays in sync")
	assert.False(t, mirrored(t, cs, "ws-new", sec.Name), "nothing new is added")
}

// A workspace that already holds someone else's Secret is not recorded as mirrored into.
func TestConflictingTargetIsNotRecordedAsApplied(t *testing.T) {
	handMade := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "extra-ca", Namespace: "ws-a"}}
	cs := k8sfake.NewSimpleClientset(handMade)
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"), sandboxWorkspace("ws-b"))
	reconcileSecret(t, r, sec.Name)

	assert.JSONEq(t, `["ws-b"]`, liveSecret(t, r, sec.Name).Annotations[v1.WorkspaceSelectorAppliedAnnotation])
}

// A blank selector left behind must not hold the finalizer for ever.
func TestBlankSelectorReleasesFinalizer(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", "  ", "")
	sec.Finalizers = []string{v1.SecretFinalizer}
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"))
	reconcileSecret(t, r, sec.Name)
	assert.NotContains(t, liveSecret(t, r, sec.Name).Finalizers, v1.SecretFinalizer)
}

// Annotations removed just before the delete (or while the controller was down) must not skip
// the cleanup of the copies.
func TestDeleteAfterAnnotationsRemovedStillRemovesCopies(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"))
	reconcileSecret(t, r, sec.Name)
	require.True(t, mirrored(t, cs, "ws-a", sec.Name))

	live := liveSecret(t, r, sec.Name)
	live.Annotations = nil
	require.NoError(t, r.Update(context.Background(), live))
	require.NoError(t, r.Delete(context.Background(), live))
	reconcileSecret(t, r, sec.Name)
	assert.False(t, mirrored(t, cs, "ws-a", sec.Name))
}

// The record is widened to old-plus-new before any data plane is touched, so a process killed
// mid-loop still leaves a record that covers every copy that may exist.
func TestAppliedRecordWidenedBeforeMirrorLoop(t *testing.T) {
	cs := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-old"))
	cs.PrependReactor("create", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		panic("killed mid-loop")
	})
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	sec.Annotations[v1.WorkspaceSelectorAppliedAnnotation] = `["ws-old"]`
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"), selectorWorkspace("ws-old", nil))
	func() {
		defer func() { _ = recover() }()
		reconcileSecret(t, r, sec.Name)
	}()
	assert.JSONEq(t, `["ws-a","ws-old"]`, liveSecret(t, r, sec.Name).Annotations[v1.WorkspaceSelectorAppliedAnnotation])
}

// A Secret that appears between our Get and Create belongs to someone else unless it carries
// the mirror label.
func TestCopyRaceOnForeignSecretIsAConflict(t *testing.T) {
	foreign := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "extra-ca", Namespace: "ws-a"}}
	cs := k8sfake.NewSimpleClientset()
	cs.PrependReactor("create", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		_ = cs.Tracker().Add(foreign)
		return true, nil, apierrors.NewAlreadyExists(corev1.Resource("secrets"), "extra-ca")
	})
	err := copySecret(context.Background(), cs, selectorSecret("extra-ca", sandboxSelector, ""), "ws-a")
	assert.ErrorIs(t, err, errMirrorConflict)

	cs2 := k8sfake.NewSimpleClientset(managedCopy("extra-ca", "ws-a"))
	assert.NoError(t, copySecret(context.Background(), cs2, selectorSecret("extra-ca", sandboxSelector, ""), "ws-a"))
}
