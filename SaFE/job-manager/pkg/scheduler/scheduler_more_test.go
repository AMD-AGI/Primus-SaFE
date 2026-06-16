/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

func schedWorkload(name string) *v1.Workload {
	return &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestGetWorkspaceNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	ws, err := r.getWorkspace(context.Background(), "missing")
	assert.NilError(t, err)
	assert.Assert(t, ws == nil)
}

func TestGetWorkspaceFound(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(ws).Build()
	r := &SchedulerReconciler{Client: cl}
	got, err := r.getWorkspace(context.Background(), "ws")
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
	assert.Equal(t, got.Name, "ws")
}

func TestUpdateStatusAlreadyScheduled(t *testing.T) {
	w := schedWorkload("w")
	// Pre-add the scheduled condition so updateStatus is a no-op.
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(w) + 1)
	w.Status.Conditions = []metav1.Condition{{
		Type:   string(v1.AdminScheduled),
		Reason: reason,
	}}
	r := &SchedulerReconciler{}
	err := r.updateStatus(context.Background(), w)
	assert.NilError(t, err)
}

func TestUpdateStatusPatch(t *testing.T) {
	w := schedWorkload("w")
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.updateStatus(context.Background(), w)
	assert.NilError(t, err)
}

func TestMarkAsScheduled(t *testing.T) {
	w := schedWorkload("w")
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.markAsScheduled(context.Background(), w)
	assert.NilError(t, err)
	assert.Assert(t, v1.IsWorkloadScheduled(w))
}

func TestCascadeStopChildrenEmpty(t *testing.T) {
	owner := schedWorkload("owner")
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.cascadeStopChildren(context.Background(), owner)
	assert.NilError(t, err)
}

func TestCascadeStopChildrenWithChild(t *testing.T) {
	owner := schedWorkload("owner")
	child := schedWorkload("child")
	child.Labels = map[string]string{v1.OwnerLabel: "owner"}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(child).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.cascadeStopChildren(context.Background(), owner)
	assert.NilError(t, err)
}

func TestSetDependencyPhaseSucceeded(t *testing.T) {
	dep := schedWorkload("dep")
	depended := schedWorkload("depended")
	depended.Spec.Dependencies = []string{"dep"}
	dep.Status.Phase = v1.WorkloadSucceeded
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(depended).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.setDependencyPhase(context.Background(), dep, depended)
	assert.NilError(t, err)
}

func TestSetDependencyPhaseFailed(t *testing.T) {
	dep := schedWorkload("dep")
	depended := schedWorkload("depended")
	depended.Spec.Dependencies = []string{"dep"}
	dep.Status.Phase = v1.WorkloadFailed
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(depended).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.setDependencyPhase(context.Background(), dep, depended)
	assert.NilError(t, err)
}

func TestUpdateDependentsPhaseNotEnded(t *testing.T) {
	// A not-ended workload returns nil without listing dependents.
	r := &SchedulerReconciler{}
	err := r.updateDependentsPhase(context.Background(), schedWorkload("w"))
	assert.NilError(t, err)
}

func TestCheckWorkloadDependenciesNoDeps(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	ready, err := r.checkWorkloadDependencies(context.Background(), schedWorkload("w"))
	assert.NilError(t, err)
	assert.Equal(t, ready, true)
}

func TestCheckWorkloadDependenciesNotFound(t *testing.T) {
	w := schedWorkload("w")
	w.Spec.Dependencies = []string{"missing-dep"}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	// Dependency not found -> the workload is marked failed; when that status
	// update succeeds the function returns (false, nil) per its control flow.
	ready, err := r.checkWorkloadDependencies(context.Background(), w)
	assert.Equal(t, ready, false)
	assert.NilError(t, err)
	assert.Equal(t, w.Status.Phase, v1.WorkloadFailed)
}
