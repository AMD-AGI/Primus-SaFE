/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
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
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ws"},
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
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 2}}
	w.Status.Pods = []v1.WorkloadPod{
		{PodId: "p0", Phase: corev1.PodRunning, AdminNodeName: "n0"},
		{PodId: "p1", Phase: corev1.PodPending, AdminNodeName: ""},
	}
	if shouldClearExternalScaleUpMessage(w, running) {
		t.Fatal("must not clear scale-up message while a sibling is still unscheduled")
	}
	w.Status.Pods[1] = v1.WorkloadPod{PodId: "p1", Phase: corev1.PodRunning, AdminNodeName: "n1"}
	if !shouldClearExternalScaleUpMessage(w, running) {
		t.Fatal("all assigned pods must clear stale scale-up message")
	}
}

func TestExternalWaitingMessageIncludesEvent(t *testing.T) {
	w := &v1.Workload{
		Status: v1.WorkloadStatus{
			ExternalExecution: &v1.WorkloadExternalExecution{
				PlacementMode: v1.ExternalPlacementKubeScheduler,
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ws"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type:    corev1.PodScheduled,
				Status:  corev1.ConditionFalse,
				Reason:  corev1.PodReasonUnschedulable,
				Message: "0/1 nodes are available",
			}},
		},
	}
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "p1.1", Namespace: "ws"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: "p1", Namespace: "ws",
		},
		Type:    corev1.EventTypeNormal,
		Reason:  "TriggeredScaleUp",
		Message: "pod triggered scale-up",
	}
	cs := k8sfake.NewSimpleClientset(ev)
	sets := &ClusterClientSets{}
	sets.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))

	got := externalWaitingMessage(context.Background(), sets, w, pod)
	if !strings.Contains(got, "0/1 nodes are available") ||
		!strings.Contains(got, "TriggeredScaleUp") ||
		!strings.Contains(got, "pod triggered scale-up") {
		t.Fatalf("got %q", got)
	}
}
