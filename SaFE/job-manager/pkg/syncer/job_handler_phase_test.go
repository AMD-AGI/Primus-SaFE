/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func syncerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUpdateAdminWorkloadPhase(t *testing.T) {
	r := &SyncerReconciler{}
	msg := &resourceMessage{dispatchCount: 1}

	cases := []struct {
		k8sPhase string
		want     v1.WorkloadPhase
		maxRetry int
		count    int
	}{
		{string(v1.K8sPending), v1.WorkloadPending, 3, 1},
		{string(v1.K8sSucceeded), v1.WorkloadSucceeded, 3, 1},
		{string(v1.K8sNotReady), v1.WorkloadNotReady, 3, 1},
		{string(v1.K8sRunning), v1.WorkloadRunning, 3, 1},
		{string(v1.K8sUpdating), v1.WorkloadUpdating, 3, 1},
		{string(v1.AdminStopped), v1.WorkloadStopped, 3, 1},
		// Failed beyond retry budget -> Failed.
		{string(v1.K8sFailed), v1.WorkloadFailed, 1, 99},
	}
	for _, c := range cases {
		w := &v1.Workload{}
		w.Spec.MaxRetry = c.maxRetry
		m := &resourceMessage{dispatchCount: c.count}
		r.updateAdminWorkloadPhase(w, &jobutils.K8sObjectStatus{Phase: c.k8sPhase}, m)
		assert.Equal(t, string(w.Status.Phase), string(c.want))
	}
	_ = msg
}

func TestUpdateWorkloadCondition(t *testing.T) {
	w := &v1.Workload{}
	cond := jobutils.NewCondition("TypeA", "msg1", "reason1")

	// First insert appends.
	updateWorkloadCondition(w, cond)
	assert.Equal(t, len(w.Status.Conditions), 1)

	// Same type+reason but different message updates in place.
	cond2 := jobutils.NewCondition("TypeA", "msg2", "reason1")
	updateWorkloadCondition(w, cond2)
	assert.Equal(t, len(w.Status.Conditions), 1)
	assert.Equal(t, w.Status.Conditions[0].Message, "msg2")

	// Different reason appends a new condition.
	cond3 := jobutils.NewCondition("TypeA", "msg", "reason2")
	updateWorkloadCondition(w, cond3)
	assert.Equal(t, len(w.Status.Conditions), 2)
}

func TestReSchedule(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true", v1.WorkloadScheduledAnnotation: "true"},
	}}
	w.Status.Phase = v1.WorkloadRunning
	w.Status.Pods = []v1.WorkloadPod{{PodId: "p"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}

	err := r.reSchedule(context.Background(), w, 1)
	assert.NilError(t, err)
	assert.Equal(t, string(w.Status.Phase), string(v1.WorkloadPending))
	// Dispatched annotation removed during reschedule.
	assert.Equal(t, v1.IsWorkloadDispatched(w), false)
}
