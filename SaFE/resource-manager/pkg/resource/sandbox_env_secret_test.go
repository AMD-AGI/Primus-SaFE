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
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
)

// adoptionSequence runs the sequence a workspace admin can drive through the API against a
// Secret a system admin made with kubectl in ws-a: create a same-named Secret bound to ws-a,
// reconcile, then unbind it and reconcile again. It returns the ws-a Secret after the bind and
// whether it still exists after the unbind.
func adoptionSequence(t *testing.T, name string) (afterBind *corev1.Secret, survivedUnbind bool, events []string) {
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
	reconcileSecret(t, r, name)

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
	return got, err == nil, events
}

// A sandbox-env made in the workspace namespace is never taken over by an ids grant, nor deleted
// when that grant is withdrawn: the one in the workspace namespace wins.
func TestMirrorNeverAdoptsAHandMadeSandboxEnvSecret(t *testing.T) {
	got, survived, events := adoptionSequence(t, commonsecret.SandboxEnvSecretName)
	assert.Equal(t, map[string][]byte{"HF_TOKEN": []byte("admin-value")}, got.Data)
	assert.Empty(t, got.Labels[managedSecretLabelKey])
	assert.True(t, survived, "unbinding deleted the hand-made sandbox-env")
	require.NotEmpty(t, events)
	assert.Contains(t, events[0], "MirrorConflict")
}

// Reverse proof: the same sequence with any other name still adopts, then deletes, as before.
func TestMirrorStillAdoptsOtherNamesOnAnIdsGrant(t *testing.T) {
	got, survived, _ := adoptionSequence(t, "image-pull")
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
