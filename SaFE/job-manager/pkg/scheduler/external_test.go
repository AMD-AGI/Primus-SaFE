/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/execution"
)

func gpuWorkload() *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "train-1", UID: "11111111-1111-1111-1111-111111111111"},
		Spec: v1.WorkloadSpec{
			Workspace: "ws-external",
			Images:    []string{"registry.example.invalid/train:v1"},
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

func externalWorkspace() *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-external"},
		Spec:       v1.WorkspaceSpec{Cluster: "crusoe", NodeFlavor: "mi355x-8"},
	}
}

func TestBuildDemandUnitsProducesOneUnitPerPod(t *testing.T) {
	units, err := buildDemandUnits(gpuWorkload(), externalWorkspace())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("got %d units, want 1", len(units))
	}
	unit := units[0]
	if unit.UnitKey != v1.ExternalSingleUnitKey {
		t.Fatalf("unit key = %q, want %q", unit.UnitKey, v1.ExternalSingleUnitKey)
	}
	if unit.Replicas != 1 {
		t.Fatalf("replicas = %d, want 1: the contract expands a workload into one unit per pod",
			unit.Replicas)
	}
	if unit.Resources.CPUMillis != 8000 {
		t.Fatalf("cpu = %d millis, want 8000", unit.Resources.CPUMillis)
	}
	if unit.Resources.GPUCount != 1 || unit.Resources.GPUResource != "amd.com/gpu" {
		t.Fatalf("unexpected gpu request: %+v", unit.Resources)
	}
	if unit.ConstraintsDigest == "" {
		t.Fatal("constraints digest must accompany the constraints themselves")
	}
}

// The digest is recomputed by the provider and compared, so the same constraints have to
// encode identically on every call.
func TestConstraintsDigestIsStable(t *testing.T) {
	constraints := execution.PlacementConstraints{
		NodeSelector:     map[string]string{"b": "2", "a": "1"},
		AllowedNodeNames: []string{"vk-1", "vk-2"},
	}
	first, err := constraintsDigest(&constraints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reordered := execution.PlacementConstraints{
		NodeSelector:     map[string]string{"a": "1", "b": "2"},
		AllowedNodeNames: []string{"vk-1", "vk-2"},
	}
	second, err := constraintsDigest(&reordered)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Fatalf("digest changed with map ordering: %s vs %s", first, second)
	}
}

// Multi-replica workloads are declined rather than guessed at: every child pod would need a
// stable role and index for the provider to bind a reservation to, and the operators that
// create them do not supply one.
func TestBuildDemandUnitsRejectsUnsupportedShapes(t *testing.T) {
	multiReplica := gpuWorkload()
	multiReplica.Spec.Resources[0].Replica = 4
	if _, err := buildDemandUnits(multiReplica, externalWorkspace()); err == nil {
		t.Fatal("expected a multi replica workload to be rejected")
	}

	cpuOnly := gpuWorkload()
	cpuOnly.Spec.Resources[0].GPU = ""
	cpuOnly.Spec.Resources[0].GPUName = ""
	if _, err := buildDemandUnits(cpuOnly, externalWorkspace()); err == nil {
		t.Fatal("expected a workload without a gpu request to be rejected")
	}

	noImage := gpuWorkload()
	noImage.Spec.Images = nil
	if _, err := buildDemandUnits(noImage, externalWorkspace()); err == nil {
		t.Fatal("expected a workload without an image to be rejected")
	}
}

func TestReclaimingHoldsCapacityUntilReleaseIsConfirmed(t *testing.T) {
	workload := gpuWorkload()
	if isExternalReclaiming(workload) {
		t.Fatal("a workload with no claim holds nothing")
	}

	workload.Status.ExternalExecution = &v1.WorkloadExternalExecution{
		ClaimId:    "claim-1",
		ClaimPhase: execution.ClaimPhaseActive,
	}
	if !isExternalReclaiming(workload) {
		t.Fatal("an active claim holds capacity")
	}

	// Revoking records that the withdrawal was accepted. The devices are not back until
	// the provider has stopped the task and verified its cleanup, so the workload keeps
	// counting against the workspace.
	workload.Status.ExternalExecution.ClaimPhase = execution.ClaimPhaseRevoking
	if !isExternalReclaiming(workload) {
		t.Fatal("a revoking claim still holds capacity")
	}

	workload.Status.ExternalExecution.ClaimPhase = execution.ClaimPhaseReleased
	if isExternalReclaiming(workload) {
		t.Fatal("a released claim holds nothing")
	}
}

func TestWaitingReasonsSeparateShortageFromOtherRefusals(t *testing.T) {
	cases := map[string]string{
		execution.CodeCapacityUnavailable:     ExternalCapacityReason,
		execution.CodeConflict:                ExternalCapacityReason,
		execution.CodeImagePreparing:          ExternalImageReason,
		execution.CodeProfileUnvalidated:      ExternalProfileReason,
		execution.CodeConstraintUnsatisfiable: ExternalConstraintReason,
	}
	for code, want := range cases {
		got := externalWaitingReason(&execution.APIError{Code: code})
		if got != want {
			t.Fatalf("code %s mapped to %q, want %q", code, got, want)
		}
	}
	// A transport failure carries no contract code. Reporting it as a capacity shortage
	// would make the provider acquire nodes for what is only a connectivity problem.
	if got := externalWaitingReason(errNoConnection{}); got != ExternalUnavailableReason {
		t.Fatalf("transport failure mapped to %q, want %q", got, ExternalUnavailableReason)
	}
}

type errNoConnection struct{}

func (errNoConnection) Error() string { return "connection refused" }
