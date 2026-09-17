/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"encoding/json"
	"errors"
	"time"

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

// A demand is republished only when there is none or the current one is close to lapsing.
// Re-sending on every pass would either conflict with the stored revision, whose body
// carries a fixed observation time, or make the revision climb without end.
func TestDemandRefreshesOnlyWhenAbsentOrExpiring(t *testing.T) {
	now := time.Now().UTC()
	stamp := func(d time.Duration) *metav1.Time {
		value := metav1.NewTime(now.Add(d))
		return &value
	}

	cases := []struct {
		name  string
		state *v1.WorkloadExternalExecution
		want  bool
	}{
		{"never published", &v1.WorkloadExternalExecution{}, true},
		{
			"published but no window recorded",
			&v1.WorkloadExternalExecution{DemandRevision: 1},
			true,
		},
		{
			"window has room left",
			&v1.WorkloadExternalExecution{DemandRevision: 1, DemandExpiresAt: stamp(demandExpiry)},
			false,
		},
		{
			"inside the refresh margin",
			&v1.WorkloadExternalExecution{
				DemandRevision:  1,
				DemandExpiresAt: stamp(demandRefreshMargin / 2),
			},
			true,
		},
		{
			"already lapsed",
			&v1.WorkloadExternalExecution{DemandRevision: 3, DemandExpiresAt: stamp(-time.Minute)},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := demandNeedsRefresh(tc.state); got != tc.want {
				t.Fatalf("demandNeedsRefresh() = %t, want %t", got, tc.want)
			}
		})
	}
}

// An unsupported shape must not be reported as a wait. A wait implies more capacity would
// eventually help; here no amount of it would, so the workload is rejected instead.
func TestUnsupportedShapeIsDistinguishableFromAWait(t *testing.T) {
	multiReplica := gpuWorkload()
	multiReplica.Spec.Resources[0].Replica = 4
	_, err := buildDemandUnits(multiReplica, externalWorkspace())
	if err == nil {
		t.Fatal("expected an error")
	}
	var unsupported *unsupportedShapeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected an unsupportedShapeError, got %T", err)
	}
}

// The stored state is written with a JSON patch precisely so a field can go back to its
// zero value. Under merge semantics every field here is omitempty, so a false or an empty
// list would drop out of the payload and the previous value would survive -- which would
// make Reclaiming a one-way flag and carry one generation's demand into the next.
func TestExternalStateMustBeExpressibleAtItsZeroValue(t *testing.T) {
	encoded, err := json.Marshal(&v1.WorkloadExternalExecution{
		DispatchGeneration: 2,
		DemandId:           "d",
		ClaimId:            "c",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, field := range []string{"reclaiming", "demandRevision", "placements", "demandExpiresAt"} {
		if _, present := decoded[field]; present {
			t.Fatalf("field %q survived at its zero value; a merge patch could then never clear it", field)
		}
	}
}

// A demand whose publish was never confirmed must be retried, not waited out. Recording the
// expiry before the provider accepted it would make a failed call look like a live demand
// and suppress every retry for the length of the window.
func TestUnconfirmedDemandIsRetriedRatherThanWaitedOut(t *testing.T) {
	observed := metav1.NewTime(time.Now().UTC())
	unconfirmed := &v1.WorkloadExternalExecution{
		DemandRevision:   3,
		DemandObservedAt: &observed,
		DemandExpiresAt:  nil,
	}
	if !demandNeedsRefresh(unconfirmed) {
		t.Fatal("a demand with no confirmed expiry has to be retried")
	}

	future := metav1.NewTime(time.Now().UTC().Add(demandExpiry))
	confirmed := &v1.WorkloadExternalExecution{
		DemandRevision:   3,
		DemandObservedAt: &observed,
		DemandExpiresAt:  &future,
	}
	if demandNeedsRefresh(confirmed) {
		t.Fatal("a confirmed demand with room left must not be republished")
	}
}

// A workload under deletion has to reach the release path while its finalizer still holds
// the object in place. Once the finalizer is dropped there is nothing left to retry a
// failed release from, and nothing to carry the Revoking to Released confirmation.
func TestDeletionReachesTheReleasePath(t *testing.T) {
	deleting := gpuWorkload()
	now := metav1.NewTime(time.Now())
	deleting.DeletionTimestamp = &now
	deleting.Status.ExternalExecution = &v1.WorkloadExternalExecution{
		ClaimId:    "claim-1",
		ClaimPhase: execution.ClaimPhaseActive,
	}
	// Both conditions the release path gates on.
	if !deleting.IsEnd() {
		t.Fatal("deletion has to satisfy IsEnd, otherwise the release never runs")
	}
	if !isExternalReclaiming(deleting) {
		t.Fatal("a workload being deleted still holds its reservation")
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
