/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"testing"

	"gotest.tools/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func TestShouldWorkloadStopRetry(t *testing.T) {
	// Disable failover -> stop retry.
	w := &v1.Workload{}
	w.Annotations = map[string]string{v1.WorkloadDisableFailoverAnnotation: v1.TrueStr}
	w.Spec.MaxRetry = 3
	assert.Equal(t, shouldWorkloadStopRetry(w, 1), true)

	// MaxRetry <= 0 -> stop retry.
	w2 := &v1.Workload{}
	assert.Equal(t, shouldWorkloadStopRetry(w2, 1), true)

	// count > MaxRetry -> stop retry.
	w3 := &v1.Workload{}
	w3.Spec.MaxRetry = 2
	assert.Equal(t, shouldWorkloadStopRetry(w3, 3), true)

	// Within retry budget -> keep retrying.
	w4 := &v1.Workload{}
	w4.Spec.MaxRetry = 3
	assert.Equal(t, shouldWorkloadStopRetry(w4, 1), false)
}

func TestShouldTerminateWorkload(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.MaxRetry = 3

	// Succeeded -> terminate.
	assert.Equal(t, shouldTerminateWorkload(w, &jobutils.K8sObjectStatus{Phase: string(v1.K8sSucceeded)}, 1), true)

	// Failed within retry budget -> do not terminate.
	assert.Equal(t, shouldTerminateWorkload(w, &jobutils.K8sObjectStatus{Phase: string(v1.K8sFailed)}, 1), false)

	// Failed beyond retry budget -> terminate.
	assert.Equal(t, shouldTerminateWorkload(w, &jobutils.K8sObjectStatus{Phase: string(v1.K8sFailed)}, 99), true)

	// Preempted workload is never terminated here.
	wp := &v1.Workload{}
	wp.Annotations = map[string]string{v1.WorkloadPreemptedAnnotation: "true"}
	assert.Equal(t, shouldTerminateWorkload(wp, &jobutils.K8sObjectStatus{Phase: string(v1.K8sSucceeded)}, 1), false)
}

func TestIsTorchFTGroupFailed(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.Env = map[string]string{
		common.ReplicaCount:    "4",
		common.MinReplicaCount: "2",
	}
	// 3 of 4 failed -> remaining 1 < min 2 -> failed.
	w.Status.TorchFTPhase = map[string]v1.WorkloadPhase{
		"1": v1.WorkloadFailed, "2": v1.WorkloadFailed, "3": v1.WorkloadFailed,
	}
	assert.Equal(t, isTorchFTGroupFailed(w), true)

	// Only 1 failed -> remaining 3 >= min 2 -> not failed.
	w.Status.TorchFTPhase = map[string]v1.WorkloadPhase{"1": v1.WorkloadFailed}
	assert.Equal(t, isTorchFTGroupFailed(w), false)
}

func TestHandleTorchFTGroupStatusSingleGroup(t *testing.T) {
	// No replica count env -> treated as single group, returns the phase as-is.
	w := &v1.Workload{}
	got := handleTorchFTGroupStatus(w, "1", v1.WorkloadRunning)
	assert.Equal(t, string(got), string(v1.WorkloadRunning))
}

func TestHandleTorchFTGroupStatusFailed(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.Env = map[string]string{
		common.ReplicaCount:    "2",
		common.MinReplicaCount: "2",
	}
	// First group fails; remaining (1) < min (2) -> overall failed.
	got := handleTorchFTGroupStatus(w, "1", v1.WorkloadFailed)
	assert.Equal(t, string(got), string(v1.WorkloadFailed))
}

func TestHandleTorchFTGroupStatusInvalidGroupId(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.Env = map[string]string{
		common.ReplicaCount:    "2",
		common.MinReplicaCount: "1",
	}
	// Group id beyond total groups -> empty phase.
	got := handleTorchFTGroupStatus(w, "5", v1.WorkloadRunning)
	assert.Equal(t, string(got), "")
}
