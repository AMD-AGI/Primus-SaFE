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
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// Existing workspaces arrive as create events when resource-manager starts; each one whose
// label disagrees with its scopes is passed and patched, and the rest are left alone.
func TestScopeLabelControllerBackfillsExistingWorkspaces(t *testing.T) {
	unlabelled := sandboxWorkspace("ws-unlabelled")
	unlabelled.Labels = map[string]string{"keep": "me"}
	allScopes := selectorWorkspace("ws-all-scopes", nil)
	allScopes.Spec.Scopes = nil
	stale := selectorWorkspace("ws-stale", map[string]string{v1.WorkspaceSandboxScopeLabel: v1.TrueStr})
	current := sandboxWorkspace("ws-current")
	train := selectorWorkspace("ws-train", nil)

	scheme, err := genMockScheme()
	require.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(unlabelled, allScopes, stale, current, train).Build()
	r := &WorkspaceScopeLabelReconciler{Client: cl}

	want := map[string]string{
		"ws-unlabelled": v1.TrueStr, "ws-all-scopes": v1.TrueStr, "ws-stale": "",
		"ws-current": v1.TrueStr, "ws-train": "",
	}
	passed := map[string]bool{"ws-unlabelled": true, "ws-all-scopes": true, "ws-stale": true}
	for _, ws := range []*v1.Workspace{unlabelled, allScopes, stale, current, train} {
		assert.Equal(t, passed[ws.Name], scopeLabelOutOfDatePredicate{}.Create(event.CreateEvent{Object: ws}), ws.Name)
		_, err = r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: ws.Name}})
		require.NoError(t, err)
	}
	for name, value := range want {
		got := &v1.Workspace{}
		require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Name: name}, got))
		assert.Equal(t, value, v1.GetLabel(got, v1.WorkspaceSandboxScopeLabel), name)
		_, has := got.Labels[v1.WorkspaceSandboxScopeLabel]
		assert.Equal(t, value != "", has, name)
	}
	kept := &v1.Workspace{}
	require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Name: "ws-unlabelled"}, kept))
	assert.Equal(t, "me", kept.Labels["keep"], "only the one label is touched")
}

func TestScopeLabelPredicateFollowsScopeChange(t *testing.T) {
	old := sandboxWorkspace("ws-a")
	changed := old.DeepCopy()
	changed.Spec.Scopes = []v1.WorkspaceScope{v1.TrainScope}
	assert.True(t, scopeLabelOutOfDatePredicate{}.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: changed}))
	assert.False(t, scopeLabelOutOfDatePredicate{}.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
	assert.False(t, scopeLabelOutOfDatePredicate{}.Delete(event.DeleteEvent{Object: old}))
}

func TestScopeLabelReconcileMissingWorkspace(t *testing.T) {
	scheme, err := genMockScheme()
	require.NoError(t, err)
	r := &WorkspaceScopeLabelReconciler{Client: ctrlfake.NewClientBuilder().WithScheme(scheme).Build()}
	_, err = r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "gone"}})
	assert.NoError(t, err)
}
