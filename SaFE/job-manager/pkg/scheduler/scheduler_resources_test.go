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
	"k8s.io/apimachinery/pkg/api/resource"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetLeftTotalResourcesEmpty(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	ws.Status.AvailableResources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("10"),
		corev1.ResourceMemory: resource.MustParse("20Gi"),
	}
	ws.Status.TotalResources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("10"),
		corev1.ResourceMemory: resource.MustParse("20Gi"),
	}

	avail, total, err := r.getLeftTotalResources(context.Background(), ws, nil)
	assert.NilError(t, err)
	assert.Assert(t, avail != nil)
	assert.Assert(t, total != nil)
}

func TestGetLeftTotalResourcesWithPendingWorkload(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	ws.Status.AvailableResources = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")}
	ws.Status.TotalResources = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")}

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "2", Memory: "4Gi"}}
	// Pending (not running) -> uses GetTotalResourceList.
	avail, _, err := r.getLeftTotalResources(context.Background(), ws, []*v1.Workload{w})
	assert.NilError(t, err)
	assert.Assert(t, avail != nil)
}

func TestGetUnfinishedWorkloadsEmpty(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	ws.Spec.Cluster = "cluster-1"
	scheduling, scheduled, err := r.getUnfinishedWorkloads(context.Background(), ws)
	assert.NilError(t, err)
	assert.Equal(t, len(scheduling), 0)
	assert.Equal(t, len(scheduled), 0)
}
