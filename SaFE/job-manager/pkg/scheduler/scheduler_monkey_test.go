/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestScheduleWorkloadsFullPath patches the heavy helpers so scheduleWorkloads runs its
// orchestration: one scheduling workload that passes canScheduleWorkload is marked scheduled.
func TestScheduleWorkloadsFullPath(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(ws).Build()
	r := &SchedulerReconciler{Client: cl}

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "1Gi"}}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SchedulerReconciler{}), "getUnfinishedWorkloads",
		func(_ *SchedulerReconciler, _ context.Context, _ *v1.Workspace) ([]*v1.Workload, []*v1.Workload, error) {
			return []*v1.Workload{w}, nil, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SchedulerReconciler{}), "getLeftTotalResources",
		func(_ *SchedulerReconciler, _ context.Context, _ *v1.Workspace, _ []*v1.Workload) (corev1.ResourceList, corev1.ResourceList, error) {
			return corev1.ResourceList{}, corev1.ResourceList{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SchedulerReconciler{}), "canScheduleWorkload",
		func(_ *SchedulerReconciler, _ context.Context, _ *v1.Workload, _ *v1.Workspace, _ []*v1.Workload, _, _ corev1.ResourceList) (bool, string, error) {
			return true, "", nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SchedulerReconciler{}), "markAsScheduled",
		func(_ *SchedulerReconciler, _ context.Context, _ *v1.Workload) error { return nil })

	err := r.scheduleWorkloads(context.Background(), &SchedulerMessage{WorkspaceId: "ws"})
	assert.NilError(t, err)
}