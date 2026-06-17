/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
)

func batchv1AddToScheme(s *runtime.Scheme) error { return batchv1.AddToScheme(s) }

func reconcileReq(name string) ctrlruntime.Request {
	return ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: name}}
}

func clientKey(name string) client.ObjectKey { return client.ObjectKey{Name: name} }

func TestBuildLocalModelPath(t *testing.T) {
	assert.Equal(t, "/root/models/m1", buildLocalModelPath("/root/", "", "m1"))
	assert.Equal(t, "/root/sub/models/m1", buildLocalModelPath("/root", "/sub/", "m1"))
}

func TestModelSetAndGetTriedWorkspaces(t *testing.T) {
	r := newMockModelReconciler(nil)
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	// Empty -> nil.
	assert.Nil(t, r.getTriedWorkspaces(model, "/base"))
	r.setTriedWorkspaces(model, "/base", []string{"ws1", "ws2"})
	got := r.getTriedWorkspaces(model, "/base")
	assert.Equal(t, []string{"ws1", "ws2"}, got)
	// Different base -> nil.
	assert.Nil(t, r.getTriedWorkspaces(model, "/other"))
}

func TestAppendUniqueAndContains(t *testing.T) {
	s := appendUnique(nil, "a")
	s = appendUnique(s, "a")
	s = appendUnique(s, "b")
	assert.Equal(t, []string{"a", "b"}, s)
	assert.True(t, containsString(s, "a"))
	assert.False(t, containsString(s, "z"))
}

func TestModelGetWorkspace(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws).Build()
	r := newMockModelReconciler(cl)
	info, err := r.getWorkspace(context.Background(), "ws1", "")
	assert.NoError(t, err)
	assert.Equal(t, "ws1", info.ID)

	_, err = r.getWorkspace(context.Background(), "missing", "")
	assert.Error(t, err)
}

func TestModelListWorkspaces(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws).Build()
	r := newMockModelReconciler(cl)
	list, err := r.listWorkspaces(context.Background(), "")
	assert.NoError(t, err)
	assert.Len(t, list, 1)
}

func modelSchemeWithBatch(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	assert.NoError(t, batchv1AddToScheme(s))
	return s
}

func TestModelReconcileUploadingDispatch(t *testing.T) {
	model := genMockModel("m-u", v1.AccessModeLocal, "ws1")
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhaseUploading
	s := modelSchemeWithBatch(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model).Build()
	r := newMockModelReconciler(cl)
	// Dispatches to handleUploading; no job -> Failed.
	_, err := r.Reconcile(context.Background(), reconcileReq("m-u"))
	assert.NoError(t, err)
}

func TestModelReconcileReadyNoop(t *testing.T) {
	model := genMockModel("m-r", v1.AccessModeLocal, "ws1")
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhaseReady
	s := modelSchemeWithBatch(t)
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model).Build()
	r := newMockModelReconciler(cl)
	res, err := r.Reconcile(context.Background(), reconcileReq("m-r"))
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestModelReconcileLocalPathMode(t *testing.T) {
	model := genMockModel("m-lp", v1.AccessModeLocalPath, "ws1")
	model.Spec.Source.LocalPath = "/data/models/m-lp"
	cl := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithStatusSubresource(model).
		WithObjects(model).
		Build()
	r := newMockModelReconciler(cl)
	_, err := r.Reconcile(context.Background(), reconcileReq("m-lp"))
	assert.NoError(t, err)
	updated := &v1.Model{}
	assert.NoError(t, cl.Get(context.Background(), clientKey("m-lp"), updated))
	assert.Equal(t, v1.ModelPhaseReady, updated.Status.Phase)
	assert.Len(t, updated.Status.LocalPaths, 1)
}

func TestModelExtractBasePath(t *testing.T) {
	r := newMockModelReconciler(nil)
	assert.Equal(t, "/wekafs", r.extractBasePath("/wekafs/models/llama"))
	assert.Equal(t, "", r.extractBasePath("/nomatch"))
}

func TestModelTryFailoverNoBasePath(t *testing.T) {
	r := newMockModelReconciler(nil)
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	lp := &v1.ModelLocalPath{Workspace: "ws1", Path: "/nobase"}
	assert.False(t, r.tryFailover(context.Background(), model, lp))
}

func TestModelTryFailoverNoCandidates(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws).Build()
	r := newMockModelReconciler(cl)
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "m1"}}
	lp := &v1.ModelLocalPath{Workspace: "ws1", Path: "/wekafs/models/m1"}
	// Only ws1 exists and it's the failed one -> no candidates.
	assert.False(t, r.tryFailover(context.Background(), model, lp))
}

func TestModelInitializeLocalPathsPrivate(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws).Build()
	r := newMockModelReconciler(cl)
	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	paths := r.initializeLocalPaths(context.Background(), model)
	assert.Len(t, paths, 1)
	assert.Equal(t, "ws1", paths[0].Workspace)
}
