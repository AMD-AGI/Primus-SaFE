/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestRelevantChangePredicateCreate(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	p := r.relevantChangePredicate()

	// Workload without cronjobs -> Create returns true.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &v1.Workload{}}), true)

	// Wrong type -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &corev1.Pod{}}), false)
}

func TestRelevantChangePredicateUpdate(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	p := r.relevantChangePredicate()

	// Wrong types -> false.
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}), false)

	// Scheduled-state change -> true.
	oldW := &v1.Workload{}
	newW := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadScheduledAnnotation: "true"},
	}}
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: oldW, ObjectNew: newW}), true)
}

func TestNotifyDependentWorkspacesEmpty(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	// No dependent workspaces -> no-op, no panic.
	r.notifyDependentWorkspaces(&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}})
}

func TestHandleWorkspaceEventWrongTypes(t *testing.T) {
	r := &SchedulerReconciler{}
	h := r.handleWorkspaceEvent()
	// Create/Delete are no-ops; Update with wrong types returns without enqueue.
	h.Create(context.Background(), event.CreateEvent{Object: &v1.Workspace{}}, nil)
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}, nil)
	h.Delete(context.Background(), event.DeleteEvent{Object: &v1.Workspace{}}, nil)
}

func TestPreemptNotEnabled(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	// Request workload without preempt enabled -> no preemption.
	ok, err := r.preempt(context.Background(), &v1.Workload{}, nil, corev1.ResourceList{})
	assert.NilError(t, err)
	assert.Equal(t, ok, false)
}
