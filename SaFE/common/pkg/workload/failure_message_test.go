/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"testing"
	"time"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetWorkloadFailureMessage_Selection(t *testing.T) {
	now := metav1.Now()
	condition := func(kind v1.WorkloadConditionType, count int, message string) metav1.Condition {
		return metav1.Condition{Type: string(kind), Status: metav1.ConditionTrue, Reason: GenerateDispatchReason(count), Message: message, LastTransitionTime: now}
	}
	for _, tc := range []struct {
		name       string
		conditions []metav1.Condition
		count      int
		want       string
	}{
		{"absent", nil, 1, WorkloadFailureFallback},
		{"current", []metav1.Condition{condition(v1.K8sFailed, 2, "controller failure")}, 2, "controller failure"},
		{"initial", []metav1.Condition{condition(v1.AdminFailed, 1, "initial failure")}, 0, "initial failure"},
		{"tie", []metav1.Condition{condition(v1.K8sFailed, 1, "first"), condition(v1.AdminFailed, 1, "last")}, 1, "last"},
		{"previous attempt", []metav1.Condition{condition(v1.AdminFailed, 1, "old")}, 2, WorkloadFailureFallback},
		{"informational", []metav1.Condition{condition(v1.K8sPending, 1, "pending")}, 1, WorkloadFailureFallback},
		{"blank", []metav1.Condition{condition(v1.AdminFailed, 1, " \n")}, 1, WorkloadFailureFallback},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, GetWorkloadFailureMessage(tc.conditions, tc.count), tc.want) })
	}
	older := condition(v1.AdminFailed, 1, "older")
	older.LastTransitionTime = metav1.NewTime(now.Add(-time.Second))
	newest := condition(v1.K8sFailed, 1, "newest")
	assert.Equal(t, GetWorkloadFailureMessage([]metav1.Condition{newest, older}, 1), "newest")
	newest.Status = metav1.ConditionFalse
	assert.Equal(t, GetWorkloadFailureMessage([]metav1.Condition{newest, older}, 1), "older")
}
