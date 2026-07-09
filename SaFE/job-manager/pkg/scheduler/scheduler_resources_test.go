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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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

func TestGetLeftTotalResourcesBalanceIgnoresUnboundPendingWorkload(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec:       v1.WorkspaceSpec{QueuePolicy: v1.QueueBalancePolicy},
	}
	ws.Status.AvailableResources = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("20")}
	ws.Status.TotalResources = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("20")}

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pending"}}
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "10", Memory: "1Gi"}}
	w.Status.Phase = v1.WorkloadPending
	w.Status.NodeUsage = []v1.NodePodUsage{{
		Node:   "",
		Active: map[string]int{"0": 1},
	}}

	_, total, err := r.getLeftTotalResources(context.Background(), ws, []*v1.Workload{w})
	assert.NilError(t, err)
	assert.Equal(t, total.Cpu().String(), "20")
}

func TestScheduleWorkloadsBalanceSchedulesBehindUnboundPendingReservation(t *testing.T) {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: v1.WorkspaceSpec{
			Cluster:     "cluster-1",
			QueuePolicy: v1.QueueBalancePolicy,
		},
	}
	ws.Status.AvailableResources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("12"),
		corev1.ResourceMemory: resource.MustParse("12Gi"),
	}
	ws.Status.TotalResources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("12"),
		corev1.ResourceMemory: resource.MustParse("12Gi"),
	}

	reserved := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "reserved",
			Annotations: map[string]string{
				v1.WorkloadScheduledAnnotation: "2026-07-09T03:00:51Z",
			},
			Labels: map[string]string{
				v1.ClusterIdLabel:   "cluster-1",
				v1.WorkspaceIdLabel: "ws",
			},
		},
		Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{{Replica: 1, CPU: "12", Memory: "1Gi"}},
			Workspace: "ws",
		},
		Status: v1.WorkloadStatus{
			Phase: v1.WorkloadPending,
			NodeUsage: []v1.NodePodUsage{{
				Node:   "",
				Active: map[string]int{"0": 1},
			}},
		},
	}
	target := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "target",
			Labels: map[string]string{
				v1.ClusterIdLabel:   "cluster-1",
				v1.WorkspaceIdLabel: "ws",
			},
		},
		Spec: v1.WorkloadSpec{
			IsTolerateAll: true,
			Resources:     []v1.WorkloadResource{{Replica: 1, CPU: "2", Memory: "1Gi"}},
			Workspace:     "ws",
		},
	}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(ws, reserved, target).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}

	err := r.scheduleWorkloads(context.Background(), &SchedulerMessage{WorkspaceId: "ws"})
	assert.NilError(t, err)

	got := &v1.Workload{}
	err = cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "target"}, got)
	assert.NilError(t, err)
	assert.Assert(t, v1.IsWorkloadScheduled(got))
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
