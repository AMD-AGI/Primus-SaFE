/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestHandleRaySubmitterTimeoutNonRayJob(t *testing.T) {
	r := &SyncerReconciler{}
	w := &v1.Workload{}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	ok, err := r.handleRaySubmitterTimeout(context.Background(), w, pod)
	assert.NilError(t, err)
	assert.Equal(t, ok, false)
}

func TestBuildPodTerminatedInfoRunningNoop(t *testing.T) {
	w := &v1.Workload{}
	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, w, pod, wp, "main")
	// Running pod -> no termination info recorded.
	assert.Equal(t, wp.EndTime, "")
	assert.Equal(t, len(wp.Containers), 0)
}

func TestBuildPodTerminatedInfoFailed(t *testing.T) {
	w := &v1.Workload{}
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase:   corev1.PodFailed,
		Reason:  "OOMKilled",
		Message: "out of memory",
	}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, w, pod, wp, "main")
	assert.Assert(t, wp.FailedMessage != "")
	assert.Assert(t, wp.EndTime != "")
}

func TestBuildPodTerminatedInfoSucceeded(t *testing.T) {
	w := &v1.Workload{}
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodSucceeded,
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "main",
			State: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
			},
		}},
	}}
	wp := &v1.WorkloadPod{}
	buildPodTerminatedInfo(context.Background(), nil, w, pod, wp, "main")
	assert.Equal(t, len(wp.Containers), 1)
	assert.Assert(t, wp.EndTime != "")
}

func TestGenerateStickyFault(t *testing.T) {
	// Empty node id -> nil fault, no error.
	f, err := generateStickyFault(&v1.Workload{}, "", syncerScheme(t))
	assert.NilError(t, err)
	assert.Assert(t, f == nil)

	// Valid node id -> a fault is generated.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	f2, err := generateStickyFault(w, "node-1", syncerScheme(t))
	assert.NilError(t, err)
	assert.Assert(t, f2 != nil)
	assert.Equal(t, f2.Spec.Node.AdminName, "node-1")
}
