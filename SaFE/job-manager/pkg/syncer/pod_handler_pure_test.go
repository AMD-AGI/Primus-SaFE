/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func TestConvertPodFromUnstructured(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": "p1"},
		"status":     map[string]interface{}{"phase": "Running"},
	}}
	pod := convertPodFromUnstructured(obj)
	assert.Assert(t, pod != nil)
	assert.Equal(t, pod.Name, "p1")
	assert.Equal(t, string(pod.Status.Phase), "Running")

	// Failed pod hits the failure-logging branch.
	failed := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": "p2"},
		"status":     map[string]interface{}{"phase": "Failed"},
	}}
	pod2 := convertPodFromUnstructured(failed)
	assert.Assert(t, pod2 != nil)
	assert.Equal(t, string(pod2.Status.Phase), "Failed")
}

func TestUpdateCICDScalingRunnerSetPhase(t *testing.T) {
	mkPod := func(phase corev1.PodPhase) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{appComponent: scaleSetListener}},
			Status:     corev1.PodStatus{Phase: phase},
		}
	}

	w := &v1.Workload{}
	updateCICDScalingRunnerSetPhase(w, mkPod(corev1.PodRunning))
	assert.Equal(t, string(w.Status.Phase), string(v1.WorkloadRunning))

	updateCICDScalingRunnerSetPhase(w, mkPod(corev1.PodPending))
	assert.Equal(t, string(w.Status.Phase), string(v1.WorkloadPending))

	updateCICDScalingRunnerSetPhase(w, mkPod(corev1.PodSucceeded))
	assert.Equal(t, string(w.Status.Phase), string(v1.WorkloadNotReady))

	// Pod without the listener label is ignored.
	w2 := &v1.Workload{}
	updateCICDScalingRunnerSetPhase(w2, &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}})
	assert.Equal(t, string(w2.Status.Phase), "")
}

func TestCompareRayJobPodPriority(t *testing.T) {
	running := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "a"}
	pending := v1.WorkloadPod{Phase: corev1.PodPending, PodId: "b"}
	// Running has higher phase priority than pending.
	assert.Assert(t, compareRayJobPodPriority(running, pending) > 0)
	assert.Assert(t, compareRayJobPodPriority(pending, running) < 0)

	// Same phase, tie broken by start time (later wins).
	now := time.Now().UTC()
	early := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "a", StartTime: timeutil.FormatRFC3339(now.Add(-time.Hour))}
	late := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "a", StartTime: timeutil.FormatRFC3339(now)}
	assert.Assert(t, compareRayJobPodPriority(late, early) > 0)

	// Same phase and time, tie broken by pod id.
	p1 := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "a"}
	p2 := v1.WorkloadPod{Phase: corev1.PodRunning, PodId: "b"}
	assert.Assert(t, compareRayJobPodPriority(p2, p1) > 0)
	assert.Equal(t, compareRayJobPodPriority(p1, p1), 0)
}

func TestGetFailedPodInfo(t *testing.T) {
	// No failed pods -> empty string.
	w := &v1.Workload{}
	w.Status.Pods = []v1.WorkloadPod{{Phase: corev1.PodRunning}}
	assert.Equal(t, getFailedPodInfo(w), "")

	// Failed pod -> JSON with details.
	w.Status.Pods = []v1.WorkloadPod{{
		Phase:         corev1.PodFailed,
		PodId:         "p1",
		AdminNodeName: "node-1",
		FailedMessage: "oom",
		Containers:    []v1.Container{{ExitCode: 137}},
	}}
	out := getFailedPodInfo(w)
	assert.Assert(t, out != "")
}
