/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func gpuWorkload() *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "train-1", UID: "11111111-1111-1111-1111-111111111111"},
		Spec: v1.WorkloadSpec{
			Workspace: "ws-external",
			Images: []string{
				"registry.example.invalid/train@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Resources: []v1.WorkloadResource{{
				Replica: 1,
				CPU:     "8",
				Memory:  "64Gi",
				GPU:     "1",
				GPUName: "amd.com/gpu",
			}},
		},
	}
}

func TestReclaimingTracksProvisioningRequest(t *testing.T) {
	workload := gpuWorkload()
	if isExternalReclaiming(workload) {
		t.Fatal("nil external state is not reclaiming")
	}
	workload.Status.ExternalExecution = &v1.WorkloadExternalExecution{
		ClaimId:    "claim-1",
		ClaimPhase: "Active",
	}
	if isExternalReclaiming(workload) {
		t.Fatal("HTTP claim leftovers must not count as reclaiming")
	}
	workload.Status.ExternalExecution = &v1.WorkloadExternalExecution{
		PlacementMode:       v1.ExternalPlacementKubeScheduler,
		ProvisioningRequest: "pr-1",
	}
	if !isExternalReclaiming(workload) {
		t.Fatal("open ProvisioningRequest must count as reclaiming")
	}
	workload.Status.ExternalExecution.ProvisioningRequest = ""
	if isExternalReclaiming(workload) {
		t.Fatal("cleared ProvisioningRequest must not reclaim")
	}
}

func TestWaitingReasonsAreTerminal(t *testing.T) {
	for _, reason := range []string{
		ExternalUnsupportedReason, ExternalConstraintReason, ExternalInvalidReason,
		ExternalBudgetMissingReason, ExternalPRFailedReason,
		ExternalBudgetMissingReason + " - rdma/hca",
	} {
		if !isTerminalExternalReason(reason) {
			t.Fatalf("%q must be terminal", reason)
		}
	}
	if isTerminalExternalReason(ExternalCapacityReason) {
		t.Fatal("capacity wait must not be terminal")
	}
}

// Single-pod kube-scheduler work never creates a ProvisioningRequest. Release must
// clear DispatchGeneration without synthesizing a DELETE against a guessed name.
func TestReconcileExternalReleaseSkipsDeleteWithoutPR(t *testing.T) {
	w := gpuWorkload()
	w.Status.Phase = v1.WorkloadSucceeded
	w.Status.ExternalExecution = &v1.WorkloadExternalExecution{
		PlacementMode:      v1.ExternalPlacementKubeScheduler,
		DispatchGeneration: 3,
	}
	r := &SchedulerReconciler{Client: ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).
		WithObjects(w).WithStatusSubresource(&v1.Workload{}).Build()}
	still, err := r.reconcileExternalRelease(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if still {
		t.Fatal("empty ProvisioningRequest must not keep reclaiming")
	}
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.ExternalExecution.DispatchGeneration != 0 {
		t.Fatalf("DispatchGeneration=%d want 0", stored.Status.ExternalExecution.DispatchGeneration)
	}
}

func TestExternalRetryDelay(t *testing.T) {
	if d, ok := externalRetryDelay(ExternalCapacityReason, nil); !ok || d != externalWaitRetry {
		t.Fatalf("plain wait = %v %v", d, ok)
	}
	if d, ok := externalRetryDelay(ExternalCapacityReason, errors.New("boom")); !ok || d != externalExchangeRetry {
		t.Fatalf("error without Retry-After = %v %v", d, ok)
	}
	for _, reason := range []string{ExternalUnsupportedReason, ExternalConstraintReason} {
		if _, ok := externalRetryDelay(reason, nil); ok {
			t.Fatalf("%q must not be retried", reason)
		}
		if !isTerminalExternalReason(reason) {
			t.Fatalf("%q must be terminal", reason)
		}
	}
}

func TestPersistExternalTerminalFailure(t *testing.T) {
	w := gpuWorkload()
	w.Status.Phase = v1.WorkloadPending
	r := &SchedulerReconciler{Client: ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).
		WithObjects(w).WithStatusSubresource(&v1.Workload{}).Build()}
	reason := ExternalPRFailedReason + " - capacity revoked permanently"
	if err := r.persistExternalTerminalFailure(context.Background(), w, reason); err != nil {
		t.Fatal(err)
	}
	// Caller snapshot must mirror Failed so updateUnScheduled does not overwrite.
	if w.Status.Phase != v1.WorkloadFailed {
		t.Fatalf("caller phase=%s want Failed", w.Status.Phase)
	}
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.Phase != v1.WorkloadFailed {
		t.Fatalf("phase=%s", stored.Status.Phase)
	}
	if !strings.Contains(stored.Status.Message, "provisioning request failed") {
		t.Fatalf("message=%q", stored.Status.Message)
	}
	if err := r.persistExternalTerminalFailure(context.Background(), stored, reason); err != nil {
		t.Fatal(err)
	}
}
