/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package adopt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func newAdminClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))
	return ctrlfake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&v1.Workload{}).WithObjects(objs...).Build()
}

func globalUser(id, name string) *v1.User {
	return &v1.User{ObjectMeta: metav1.ObjectMeta{Name: id, Annotations: map[string]string{v1.UserNameAnnotation: name}}}
}

func localWorkload(name, userId, userName string, phase v1.WorkloadPhase) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name + "-local-uid"),
			Labels: map[string]string{v1.UserIdLabel: userId, v1.WorkloadDispatchCntLabel: "1",
				v1.WorkspaceIdLabel: "ccs300-gpu"},
			Annotations: map[string]string{v1.UserNameAnnotation: userName,
				v1.WorkloadScheduledAnnotation: "2026-11-23T11:24:02", v1.WorkloadDispatchedAnnotation: "2026-11-23T11:24:02",
				lastAppliedAnnotation: "{}"}},
		Spec:   v1.WorkloadSpec{Workspace: "ccs300-gpu", Images: []string{"busybox"}},
		Status: v1.WorkloadStatus{Phase: phase, Message: "local status"},
	}
}

func adoptedWorkspace(cluster string) *v1.Workspace {
	return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ccs300-gpu"}, Spec: v1.WorkspaceSpec{Cluster: cluster}}
}

func TestListUnfinishedWorkloads(t *testing.T) {
	gvr := v1.SchemeGroupVersion.WithResource("workloads")
	toUnstructured := func(w *v1.Workload) *unstructured.Unstructured {
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(w)
		assert.NoError(t, err)
		u := &unstructured.Unstructured{Object: obj}
		u.SetAPIVersion(v1.SchemeGroupVersion.String())
		u.SetKind(v1.WorkloadKind)
		return u
	}
	newClient := func(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
		return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
			map[schema.GroupVersionResource]string{gvr: "WorkloadList"}, objs...)
	}

	got, err := ListUnfinishedWorkloads(context.Background(), newClient(
		toUnstructured(localWorkload("running", "u1", "root", v1.WorkloadRunning)),
		toUnstructured(localWorkload("pending", "u1", "root", v1.WorkloadPending)),
		toUnstructured(localWorkload("done", "u1", "root", v1.WorkloadSucceeded))))
	assert.NoError(t, err)
	names := []string{}
	for _, w := range got {
		names = append(names, w.Name)
	}
	assert.ElementsMatch(t, []string{"running", "pending"}, names)

	missing := newClient()
	missing.PrependReactor("list", "workloads", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(gvr.GroupResource(), "")
	})
	got, err = ListUnfinishedWorkloads(context.Background(), missing)
	assert.NoError(t, err)
	assert.Empty(t, got)
}

func TestAdminWorkloadKeepsLocalMarksAndReplacesOwner(t *testing.T) {
	local := localWorkload("w1", "local-id", "weilei", v1.WorkloadRunning)
	got := AdminWorkload(local, globalUser("global-id", "Wei, Lei"))

	assert.Equal(t, "w1", got.Name)
	assert.Equal(t, local.Spec, got.Spec)
	assert.Equal(t, "global-id", v1.GetUserId(got))
	assert.Equal(t, "Wei, Lei", v1.GetUserName(got))
	assert.Equal(t, "2026-11-23T11:24:02", v1.GetAnnotation(got, v1.WorkloadScheduledAnnotation))
	assert.Equal(t, "2026-11-23T11:24:02", v1.GetAnnotation(got, v1.WorkloadDispatchedAnnotation))
	assert.Equal(t, "1", v1.GetLabel(got, v1.WorkloadDispatchCntLabel))
	assert.True(t, v1.IsAdoptedWorkload(got))
	assert.Equal(t, "w1-local-uid", v1.GetAnnotation(got, v1.WorkloadAdoptUidAnnotation))
	assert.False(t, v1.HasAnnotation(got, lastAppliedAnnotation))
	assert.Equal(t, "local-id", v1.GetUserId(local))
}

func TestPlanImports(t *testing.T) {
	c := newAdminClient(t,
		globalUser("root-id", "root"), globalUser("mapped-id", "Wei, Lei"), globalUser("chenluo-id", "Chen, Luo"),
		&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "existing"}})
	locals := []*v1.Workload{
		localWorkload("existing", "root-id", "root", v1.WorkloadRunning),
		localWorkload("matched", "root-id", "root", v1.WorkloadRunning),
		localWorkload("mapped", "local-weilei", "weilei", v1.WorkloadPending),
		localWorkload("same-id-other-name", "chenluo-id", "chenluo", v1.WorkloadRunning),
		localWorkload("unmapped", "local-meili", "meili", v1.WorkloadRunning),
	}
	plans, err := PlanImports(context.Background(), c, locals, map[string]string{"weilei": "mapped-id"})
	assert.NoError(t, err)
	actions := map[string]string{}
	for _, p := range plans {
		actions[p.Workload.Name] = p.Action
	}
	assert.Equal(t, map[string]string{"existing": ActionExists, "matched": ActionMatched, "mapped": ActionMapped,
		"same-id-other-name": ActionUnmapped, "unmapped": ActionUnmapped}, actions)
	assert.Equal(t, "mapped-id", plans[2].User.Name)

	_, err = PlanImports(context.Background(), c, locals[4:], map[string]string{"meili": "no-such-user"})
	assert.Error(t, err)
}

func TestImportWorkloadCopiesStatus(t *testing.T) {
	c := newAdminClient(t, adoptedWorkspace("ccs300"))
	local := localWorkload("w1", "local-id", "weilei", v1.WorkloadPending)

	created, err := ImportWorkload(context.Background(), c, "ccs300", local, globalUser("global-id", "Wei, Lei"))
	assert.NoError(t, err)
	assert.True(t, created)
	got := &v1.Workload{}
	assert.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "w1"}, got))
	assert.Equal(t, v1.WorkloadPending, got.Status.Phase)
	assert.Equal(t, "local status", got.Status.Message)
	assert.Equal(t, "global-id", v1.GetUserId(got))

	created, err = ImportWorkload(context.Background(), c, "ccs300", local, globalUser("global-id", "Wei, Lei"))
	assert.NoError(t, err)
	assert.False(t, created)
}

func TestImportWorkloadRejectsWorkspaceOfAnotherCluster(t *testing.T) {
	c := newAdminClient(t, adoptedWorkspace("other"))
	_, err := ImportWorkload(context.Background(), c, "ccs300",
		localWorkload("w1", "local-id", "weilei", v1.WorkloadRunning), globalUser("global-id", "Wei, Lei"))
	assert.ErrorContains(t, err, "belongs to cluster other")
	assert.True(t, apierrors.IsNotFound(c.Get(context.Background(), types.NamespacedName{Name: "w1"}, &v1.Workload{})))
}

func TestCopyStatusKeepsAdminPlaneStatus(t *testing.T) {
	existing := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}
	c := newAdminClient(t, existing)
	assert.NoError(t, copyStatus(context.Background(), c, "w1", &v1.WorkloadStatus{Phase: v1.WorkloadPending}))
	got := &v1.Workload{}
	assert.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "w1"}, got))
	assert.Equal(t, v1.WorkloadRunning, got.Status.Phase)
}
