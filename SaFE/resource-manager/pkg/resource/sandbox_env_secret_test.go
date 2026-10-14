/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
)

// adoptionSequence runs the sequence a workspace admin can drive through the API against a
// Secret a system admin made with kubectl in ws-a: create a same-named Secret bound to ws-a,
// reconcile, then unbind it and reconcile again. It returns the ws-a Secret after the bind,
// whether it still exists after the unbind, the events, and the result of the bind's reconcile.
func adoptionSequence(t *testing.T, name string) (afterBind *corev1.Secret, survivedUnbind bool, events []string,
	bindResult ctrlruntime.Result, bindErr error) {
	t.Helper()
	ctx := context.Background()
	kubectlMade := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ws-a"},
		Data: map[string][]byte{"HF_TOKEN": []byte("admin-value")}}
	cs := k8sfake.NewSimpleClientset(kubectlMade)
	apiMade := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: common.PrimusSafeNamespace,
			Annotations: map[string]string{v1.WorkspaceIdsAnnotation: `["ws-a"]`}},
		Data: map[string][]byte{"LD_PRELOAD": []byte("/tmp/x.so")},
		Type: corev1.SecretTypeOpaque,
	}
	recorder := record.NewFakeRecorder(10)
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), apiMade, selectorWorkspace("ws-a", nil))
	r.recorder = recorder
	bindResult, bindErr = r.Reconcile(ctx, ctrlruntime.Request{
		NamespacedName: types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: name}})

	got, err := cs.CoreV1().Secrets("ws-a").Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)

	live := liveSecret(t, r, name)
	live.Annotations[v1.WorkspaceIdsAnnotation] = `[]`
	require.NoError(t, r.Update(ctx, live))
	reconcileSecret(t, r, name)
	_, err = cs.CoreV1().Secrets("ws-a").Get(ctx, name, metav1.GetOptions{})
	for len(recorder.Events) > 0 {
		events = append(events, <-recorder.Events)
	}
	return got, err == nil, events, bindResult, bindErr
}

// A sandbox-env made in the workspace namespace is never taken over by an ids grant, nor deleted
// when that grant is withdrawn: the one in the workspace namespace wins. That is a state the docs
// support, so the reconcile does not fail on it; it is checked again later.
func TestMirrorNeverAdoptsAHandMadeSandboxEnvSecret(t *testing.T) {
	got, survived, events, bindResult, bindErr := adoptionSequence(t, commonsecret.SandboxEnvSecretName)
	assert.NoError(t, bindErr)
	assert.Equal(t, sandboxEnvConflictRecheck, bindResult.RequeueAfter)
	assert.Equal(t, map[string][]byte{"HF_TOKEN": []byte("admin-value")}, got.Data)
	assert.Empty(t, got.Labels[managedSecretLabelKey])
	assert.True(t, survived, "unbinding deleted the hand-made sandbox-env")
	require.NotEmpty(t, events)
	assert.Contains(t, events[0], "MirrorConflict")
}

// Reverse proof: the same sequence with any other name still adopts, then deletes, as before.
func TestMirrorStillAdoptsOtherNamesOnAnIdsGrant(t *testing.T) {
	got, survived, events, bindResult, bindErr := adoptionSequence(t, "image-pull")
	assert.NoError(t, bindErr)
	assert.Zero(t, bindResult.RequeueAfter)
	assert.Empty(t, events)
	assert.Equal(t, map[string][]byte{"LD_PRELOAD": []byte("/tmp/x.so")}, got.Data)
	assert.Equal(t, managedSecretLabelVal, got.Labels[managedSecretLabelKey])
	assert.False(t, survived)
}

// A copy the mirror made itself is still kept in sync and removed on unbind.
func TestMirrorStillManagesItsOwnSandboxEnvCopy(t *testing.T) {
	ctx := context.Background()
	name := commonsecret.SandboxEnvSecretName
	cs := k8sfake.NewSimpleClientset()
	apiMade := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: common.PrimusSafeNamespace,
			Annotations: map[string]string{v1.WorkspaceIdsAnnotation: `["ws-a"]`}},
		Data: map[string][]byte{"HF_TOKEN": []byte("v1")},
		Type: corev1.SecretTypeOpaque,
	}
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), apiMade, selectorWorkspace("ws-a", nil))
	reconcileSecret(t, r, name)
	require.True(t, mirrored(t, cs, "ws-a", name))

	live := liveSecret(t, r, name)
	live.Data = map[string][]byte{"HF_TOKEN": []byte("v2")}
	require.NoError(t, r.Update(ctx, live))
	reconcileSecret(t, r, name)
	got, err := cs.CoreV1().Secrets("ws-a").Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got.Data["HF_TOKEN"]))

	live = liveSecret(t, r, name)
	live.Annotations[v1.WorkspaceIdsAnnotation] = `[]`
	require.NoError(t, r.Update(ctx, live))
	reconcileSecret(t, r, name)
	assert.False(t, mirrored(t, cs, "ws-a", name))
}

// A sandbox-env that cannot reach a workspace it is bound to, because the namespace holds one
// the mirror did not make, is a state the docs support (the one in the namespace wins), so it
// does not fail the reconcile: the workspaces are recorded on the Secret, a Warning event is
// raised once per workspace that enters the record (not on every reconcile), and the Secret is
// checked again later. Once the conflicting Secret is gone the copy is made and the record
// removed.
func TestSandboxEnvMirrorConflictIsRecordedOnceAndRechecked(t *testing.T) {
	ctx := context.Background()
	name := commonsecret.SandboxEnvSecretName
	handMade := func(ns string) *corev1.Secret {
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data: map[string][]byte{"HF_TOKEN": []byte("admin-value")}}
	}
	cs := k8sfake.NewSimpleClientset(handMade("ws-a"))
	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: common.PrimusSafeNamespace,
			Annotations: map[string]string{v1.WorkspaceIdsAnnotation: `["ws-a","ws-b","ws-c"]`}},
		Data: map[string][]byte{"HF_TOKEN": []byte("shared")},
		Type: corev1.SecretTypeOpaque,
	}
	recorder := record.NewFakeRecorder(10)
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), source,
		selectorWorkspace("ws-a", nil), selectorWorkspace("ws-b", nil), selectorWorkspace("ws-c", nil))
	r.recorder = recorder
	request := ctrlruntime.Request{NamespacedName: types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: name}}

	result, err := r.Reconcile(ctx, request)
	require.NoError(t, err)
	assert.Equal(t, sandboxEnvConflictRecheck, result.RequeueAfter)
	assert.Equal(t, `["ws-a"]`, liveSecret(t, r, name).Annotations[v1.WorkspaceMirrorConflictsAnnotation])
	require.Len(t, recorder.Events, 1)
	event := <-recorder.Events
	assert.Contains(t, event, "MirrorConflict")
	assert.Contains(t, event, "ws-a")
	assert.True(t, mirrored(t, cs, "ws-b", name), "the other workspaces are still served")
	assert.True(t, mirrored(t, cs, "ws-c", name))

	// The same conflict found again raises nothing new.
	result, err = r.Reconcile(ctx, request)
	require.NoError(t, err)
	assert.Equal(t, sandboxEnvConflictRecheck, result.RequeueAfter)
	assert.Empty(t, recorder.Events)

	// A workspace that enters the record raises one event, for that workspace only.
	require.NoError(t, cs.CoreV1().Secrets("ws-c").Delete(ctx, name, metav1.DeleteOptions{}))
	_, err = cs.CoreV1().Secrets("ws-c").Create(ctx, handMade("ws-c"), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = r.Reconcile(ctx, request)
	require.NoError(t, err)
	assert.Equal(t, `["ws-a","ws-c"]`, liveSecret(t, r, name).Annotations[v1.WorkspaceMirrorConflictsAnnotation])
	require.Len(t, recorder.Events, 1)
	event = <-recorder.Events
	assert.Contains(t, event, "ws-c")
	assert.NotContains(t, event, "ws-a")

	require.NoError(t, cs.CoreV1().Secrets("ws-a").Delete(ctx, name, metav1.DeleteOptions{}))
	require.NoError(t, cs.CoreV1().Secrets("ws-c").Delete(ctx, name, metav1.DeleteOptions{}))
	result, err = r.Reconcile(ctx, request)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	assert.True(t, mirrored(t, cs, "ws-a", name))
	assert.True(t, mirrored(t, cs, "ws-c", name))
	assert.NotContains(t, liveSecret(t, r, name).Annotations, v1.WorkspaceMirrorConflictsAnnotation)
	assert.Empty(t, recorder.Events)
}

// A conflict on any other name is recorded and raised once in the same way, and, as before,
// does not fail the reconcile: a hand-made Secret where a selector reaches is a supported state
// there. Only the sandbox env Secret is checked again on a timer.
func TestOtherMirrorConflictIsRecordedButNotRechecked(t *testing.T) {
	handMade := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "extra-ca", Namespace: "ws-a"},
		Data: map[string][]byte{"mine": []byte("x")}}
	cs := k8sfake.NewSimpleClientset(handMade)
	sec := selectorSecret("extra-ca", sandboxSelector, "")
	recorder := record.NewFakeRecorder(10)
	r := newSecretReconcilerFull(t, cs, testCluster("c1"), sec, sandboxWorkspace("ws-a"))
	r.recorder = recorder
	request := ctrlruntime.Request{NamespacedName: types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: sec.Name}}
	for i := 0; i < 2; i++ {
		result, err := r.Reconcile(context.Background(), request)
		require.NoError(t, err)
		assert.Zero(t, result.RequeueAfter)
	}
	assert.Equal(t, `["ws-a"]`, liveSecret(t, r, sec.Name).Annotations[v1.WorkspaceMirrorConflictsAnnotation])
	require.Len(t, recorder.Events, 1)
	assert.Contains(t, <-recorder.Events, "MirrorConflict")
}
