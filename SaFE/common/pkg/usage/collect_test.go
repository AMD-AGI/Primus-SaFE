/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package usage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/qos"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// gpuWorkload is a workload of replica pods with 8 GPUs, 16 cores and 64Gi each, of which bound
// pods are on nodes.
func gpuWorkload(name, workspace string, phase v1.WorkloadPhase, replica, bound int) v1.Workload {
	w := v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name + "-uid")},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
			Resources: []v1.WorkloadResource{{
				Replica: replica, CPU: "16", Memory: "64Gi", GPU: "8", GPUName: common.AmdGpu,
			}},
		},
		Status: v1.WorkloadStatus{Phase: phase},
	}
	for i := 0; i < bound; i++ {
		w.Status.NodeUsage = append(w.Status.NodeUsage, v1.NodePodUsage{
			Node: name + "-node-" + string(rune('a'+i)), Active: map[string]int{"0": 1}, Running: map[string]int{"0": 1},
		})
	}
	if bound < replica {
		w.Status.NodeUsage = append(w.Status.NodeUsage, v1.NodePodUsage{Node: "", Active: map[string]int{"0": replica - bound}})
	}
	return w
}

func testWorkspace(name, cluster string, gpus int64) v1.Workspace {
	return v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.WorkspaceSpec{Cluster: cluster},
		Status: v1.WorkspaceStatus{TotalResources: corev1.ResourceList{
			common.AmdGpu:         *resource.NewQuantity(gpus, resource.DecimalSI),
			corev1.ResourceCPU:    resource.MustParse("256"),
			corev1.ResourceMemory: resource.MustParse("1Ti"),
		}},
	}
}

func TestComputeWorkloadBoundAndQueued(t *testing.T) {
	for _, tc := range []struct {
		name       string
		workload   v1.Workload
		wantBound  float64
		wantQueued float64
	}{
		{"running, all bound", gpuWorkload("r", "ws", v1.WorkloadRunning, 2, 2), 16, 0},
		{"pending, no pod yet", gpuWorkload("p", "ws", v1.WorkloadPending, 2, 0), 0, 16},
		{"empty phase counts as pending", gpuWorkload("e", "ws", "", 1, 0), 0, 8},
		{"gang partly bound", gpuWorkload("g", "ws", v1.WorkloadNotReady, 4, 1), 8, 24},
		{"not ready, all bound", gpuWorkload("n", "ws", v1.WorkloadNotReady, 2, 2), 16, 0},
		{"running with an unbound pod queues nothing", gpuWorkload("u", "ws", v1.WorkloadRunning, 2, 1), 8, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := ComputeWorkload(&tc.workload)
			require.NoError(t, err)
			require.NotNil(t, u)
			assert.Equal(t, tc.wantBound, u.Bound[common.AmdGpu])
			assert.Equal(t, tc.wantQueued, u.Queued[common.AmdGpu])
		})
	}
}

func TestComputeWorkloadFinished(t *testing.T) {
	for _, phase := range []v1.WorkloadPhase{v1.WorkloadSucceeded, v1.WorkloadFailed, v1.WorkloadStopped} {
		w := gpuWorkload("f", "ws", phase, 2, 2)
		u, err := ComputeWorkload(&w)
		require.NoError(t, err)
		assert.Nil(t, u, phase)
	}
	deleting := gpuWorkload("d", "ws", v1.WorkloadRunning, 1, 1)
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	u, err := ComputeWorkload(&deleting)
	require.NoError(t, err)
	assert.Nil(t, u)
}

func TestComputeWorkloadCPUOnly(t *testing.T) {
	w := v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "cpu"},
		Spec: v1.WorkloadSpec{Workspace: "ws", Resources: []v1.WorkloadResource{{
			Replica: 3, CPU: "4", Memory: "8Gi",
		}}},
		Status: v1.WorkloadStatus{Phase: v1.WorkloadNotReady, NodeUsage: []v1.NodePodUsage{
			{Node: "n1", Active: map[string]int{"0": 2}},
			{Node: "", Active: map[string]int{"0": 1}},
		}},
	}
	u, err := ComputeWorkload(&w)
	require.NoError(t, err)
	assert.Equal(t, 8.0, u.Bound[string(corev1.ResourceCPU)])
	assert.Equal(t, 4.0, u.Queued[string(corev1.ResourceCPU)])
	assert.Equal(t, float64(8<<30), u.Queued[string(corev1.ResourceMemory)])
	_, hasGPU := u.Bound[common.AmdGpu]
	assert.False(t, hasGPU)
	_, hasGPU = u.Queued[common.AmdGpu]
	assert.False(t, hasGPU)
}

func TestComputeWorkloadFromLegacyPods(t *testing.T) {
	w := gpuWorkload("legacy", "ws", v1.WorkloadNotReady, 3, 0)
	w.Status.NodeUsage = nil
	w.Status.Pods = []v1.WorkloadPod{
		{PodId: "p0", AdminNodeName: "n1", Phase: corev1.PodRunning},
		{PodId: "p1", AdminNodeName: "n2", Phase: corev1.PodPending},
		{PodId: "p2", Phase: corev1.PodPending},
	}
	u, err := ComputeWorkload(&w)
	require.NoError(t, err)
	assert.Equal(t, 16.0, u.Bound[common.AmdGpu], "a pod on a node counts even while it is still pending")
	assert.Equal(t, 8.0, u.Queued[common.AmdGpu])
}

func TestClassifyQueueReason(t *testing.T) {
	for message, want := range map[string]string{
		qos.UserQuotaReasonPrefix + ": gpu used 8 + request 8 > limit 8": ReasonUserQos,
		qos.UserQuotaCheckFailedPrefix + ": invalid":                     ReasonUserQos,
		"In queue - waiting for scale-up (2/4 pods bound)":               ReasonExternal,
		"In queue - insufficient resources, no amd.com/gpu available":    ReasonQuota,
		"In queue - waiting for earlier or higher priority workloads":    ReasonQuota,
		"In queue - dependent workloads not yet complete":                ReasonOther,
		"In queue - waiting for scheduled start time":                    ReasonOther,
		"": ReasonOther,

		commonworkload.ExternalWaitingScaleUpReason + " - 0/2 nodes are available": ReasonExternal,
		commonworkload.ExternalCapacityReason:                                      ReasonExternal,
		commonworkload.ExternalWaitingPRAcceptReason:                               ReasonExternal,
		commonworkload.ExternalWaitingPRReason:                                     ReasonExternal,
		commonworkload.ExternalCapacityUnavailableReason:                           ReasonExternal,
		commonworkload.ExternalUnavailableReason:                                   ReasonExternal,
		commonworkload.ExternalRateLimitedReason:                                   ReasonExternal,
		commonworkload.ExternalPRExpiredReason:                                     ReasonExternal,
		commonworkload.ExternalAuthReason:                                          ReasonOther,
		commonworkload.ExternalProfileReason:                                       ReasonOther,
		commonworkload.ExternalConstraintReason:                                    ReasonOther,
		commonworkload.ExternalUnsupportedReason:                                   ReasonOther,
		commonworkload.ExternalInvalidReason:                                       ReasonOther,
		commonworkload.ExternalBudgetMissingReason:                                 ReasonOther,
		commonworkload.ExternalPRFailedReason + ": quota":                          ReasonOther,
	} {
		assert.Equal(t, want, ClassifyQueueReason(message), message)
	}
}

func TestComputeWorkloadReasonFromCondition(t *testing.T) {
	w := gpuWorkload("c", "ws", v1.WorkloadPending, 1, 0)
	w.Status.Conditions = []metav1.Condition{
		{Type: string(v1.AdminScheduling), Message: "In queue - dependent workloads not yet complete"},
		{Type: string(v1.AdminScheduling), Message: "In queue - insufficient resources"},
	}
	u, err := ComputeWorkload(&w)
	require.NoError(t, err)
	assert.Equal(t, ReasonQuota, u.Reason)

	w.Status.Message = qos.UserQuotaReasonPrefix
	u, err = ComputeWorkload(&w)
	require.NoError(t, err)
	assert.Equal(t, ReasonUserQos, u.Reason, "the status message wins over the condition")
}

func TestNewSnapshot(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	workspaces := []v1.Workspace{testWorkspace("ws1", "c1", 32), testWorkspace("ws2", "c1", 16)}
	queued := gpuWorkload("q", "ws1", v1.WorkloadPending, 1, 0)
	queued.Status.Message = "In queue - insufficient resources"
	gang := gpuWorkload("g", "ws1", v1.WorkloadNotReady, 2, 1)
	gang.Status.Message = "In queue - waiting for scale-up"
	workloads := []v1.Workload{
		gpuWorkload("r", "ws1", v1.WorkloadRunning, 2, 2),
		queued,
		gang,
		gpuWorkload("done", "ws1", v1.WorkloadSucceeded, 1, 1),
		gpuWorkload("orphan", "gone", v1.WorkloadRunning, 1, 1),
	}
	s := NewSnapshot(now, workspaces, workloads)

	gpu := s.Rows[Key{Cluster: "c1", Workspace: "ws1", Resource: common.AmdGpu}]
	require.NotNil(t, gpu)
	assert.Equal(t, now, gpu.SampleTime)
	assert.Equal(t, 32.0, gpu.Quota)
	assert.Equal(t, 24.0, gpu.Allocated)
	assert.Equal(t, 8.0, gpu.QueuedQuota)
	assert.Equal(t, 8.0, gpu.QueuedExternal)
	assert.Equal(t, int32(2), gpu.RunningWorkloads)
	assert.Equal(t, int32(2), gpu.QueuedWorkloads)

	idle := s.Rows[Key{Cluster: "c1", Workspace: "ws2", Resource: common.AmdGpu}]
	require.NotNil(t, idle, "a workspace without workloads still has a row")
	assert.Equal(t, 16.0, idle.Quota)
	assert.Zero(t, idle.Allocated)

	cpu := s.Rows[Key{Cluster: "c1", Workspace: "ws1", Resource: string(corev1.ResourceCPU)}]
	assert.Equal(t, 256.0, cpu.Quota)
	assert.Equal(t, 48.0, cpu.Allocated)

	assert.Len(t, s.Rows, 6, "cpu, memory and amd.com/gpu for two workspaces; the orphan adds none")
	assert.Equal(t, []string{"c1"}, s.Clusters())
	assert.ElementsMatch(t, []string{"r-uid", "g-uid"}, s.GPUWorkloadUIDs("c1"))
}

func TestApplyGPUUtilization(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	workspaces := []v1.Workspace{testWorkspace("ws1", "c1", 32), testWorkspace("ws2", "c1", 16)}
	workloads := []v1.Workload{
		gpuWorkload("a", "ws1", v1.WorkloadRunning, 2, 2),
		gpuWorkload("b", "ws1", v1.WorkloadRunning, 1, 1),
	}
	key := Key{Cluster: "c1", Workspace: "ws1", Resource: common.AmdGpu}
	idleKey := Key{Cluster: "c1", Workspace: "ws2", Resource: common.AmdGpu}
	cpuKey := Key{Cluster: "c1", Workspace: "ws1", Resource: string(corev1.ResourceCPU)}

	t.Run("one workload without data is left out", func(t *testing.T) {
		s := NewSnapshot(now, workspaces, workloads)
		s.ApplyGPUUtilization("c1", map[string]float64{"a-uid": 50})
		row := s.Rows[key]
		require.NotNil(t, row.Busy)
		assert.InDelta(t, 8.0, *row.Busy, 1e-9)
		require.NotNil(t, row.UtilCoverage)
		assert.InDelta(t, 16.0/24.0, *row.UtilCoverage, 1e-9)
		require.NotNil(t, s.Rows[idleKey].Busy)
		assert.Zero(t, *s.Rows[idleKey].Busy)
		assert.Nil(t, s.Rows[idleKey].UtilCoverage)
		assert.Nil(t, s.Rows[cpuKey].Busy)
	})
	t.Run("no workload has data", func(t *testing.T) {
		s := NewSnapshot(now, workspaces, workloads)
		s.ApplyGPUUtilization("c1", map[string]float64{})
		row := s.Rows[key]
		assert.Nil(t, row.Busy, "missing data is not 0%")
		require.NotNil(t, row.UtilCoverage)
		assert.Zero(t, *row.UtilCoverage)
	})
	t.Run("robust unavailable", func(t *testing.T) {
		s := NewSnapshot(now, workspaces, workloads)
		s.ApplyGPUUtilization("c1", nil)
		assert.Nil(t, s.Rows[key].Busy)
		assert.Nil(t, s.Rows[key].UtilCoverage)
		require.NotNil(t, s.Rows[idleKey].Busy)
		assert.Zero(t, *s.Rows[idleKey].Busy)
	})
	t.Run("another cluster is untouched", func(t *testing.T) {
		s := NewSnapshot(now, workspaces, workloads)
		s.ApplyGPUUtilization("c2", map[string]float64{"a-uid": 50})
		assert.Nil(t, s.Rows[key].Busy)
		assert.Nil(t, s.Rows[key].UtilCoverage)
	})
}
