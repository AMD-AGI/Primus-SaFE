/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestExternalUnschedulableMessage(t *testing.T) {
	w := &v1.Workload{
		Status: v1.WorkloadStatus{
			ExternalExecution: &v1.WorkloadExternalExecution{
				PlacementMode: v1.ExternalPlacementKubeScheduler,
			},
		},
	}
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type:    corev1.PodScheduled,
				Status:  corev1.ConditionFalse,
				Reason:  corev1.PodReasonUnschedulable,
				Message: "0/2 nodes are available",
			}},
		},
	}
	got := externalUnschedulableMessage(w, pod)
	if !strings.HasPrefix(got, "In queue - waiting for scale-up") ||
		!strings.Contains(got, "0/2 nodes are available") {
		t.Fatalf("got %q", got)
	}

	claim := w.DeepCopy()
	claim.Status.ExternalExecution.PlacementMode = v1.ExternalPlacementClaim
	if externalUnschedulableMessage(claim, pod) != "" {
		t.Fatal("claim path must not set scale-up message")
	}

	running := pod.DeepCopy()
	running.Status.Phase = corev1.PodRunning
	if externalUnschedulableMessage(w, running) != "" {
		t.Fatal("running pod must not set scale-up message")
	}

	w.Status.Message = externalWaitingScaleUpPrefix + " - 0/2 nodes are available"
	if !shouldClearExternalScaleUpMessage(w, running) {
		t.Fatal("running pod must clear stale scale-up message")
	}
}
