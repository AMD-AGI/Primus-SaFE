/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		ExternalPRFailedReason, ExternalImageResolveReason,
		ExternalImageResolveReason + " - tls: unknown authority",
	} {
		if !isTerminalExternalReason(reason) {
			t.Fatalf("%q must be terminal", reason)
		}
	}
	if isTerminalExternalReason(ExternalCapacityReason) {
		t.Fatal("capacity wait must not be terminal")
	}
}
