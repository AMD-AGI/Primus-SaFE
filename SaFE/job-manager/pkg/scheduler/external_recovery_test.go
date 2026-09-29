/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/execution"
)

const pinnedImage = "docker.io/team/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// A retry of an unconfirmed publish must resend the body it first sent, even though the
// workload and workspace resource versions have moved on since.
func TestPrepareDemandStatementReplaysUnconfirmedBody(t *testing.T) {
	first := prepareDemandStatement(&v1.WorkloadExternalExecution{DemandId: "d1"}, "100", "200", time.Now())
	if first.DemandRevision != 1 || first.DemandQueueSnapshot != "100" || first.DemandCapacitySnapshot != "200" {
		t.Fatalf("first statement = %+v", first)
	}

	retry := prepareDemandStatement(first, "101", "205", time.Now().Add(time.Minute))
	if retry.DemandRevision != first.DemandRevision || retry.DemandRequestId != first.DemandRequestId ||
		!retry.DemandObservedAt.Equal(first.DemandObservedAt) ||
		retry.DemandQueueSnapshot != "100" || retry.DemandCapacitySnapshot != "200" {
		t.Fatalf("retry changed the body: first=%+v retry=%+v", first, retry)
	}

	accepted := first.DeepCopy()
	expiry := metav1.NewTime(time.Now().Add(demandExpiry))
	accepted.DemandExpiresAt = &expiry
	if next := prepareDemandStatement(accepted, "101", "205", time.Now()); next.DemandRevision != 2 ||
		next.DemandQueueSnapshot != "101" || next.DemandExpiresAt != nil {
		t.Fatalf("an accepted revision must open a new one: %+v", next)
	}

	refused := first.DeepCopy()
	refused.DemandRequestId = ""
	if next := prepareDemandStatement(refused, "101", "205", time.Now()); next.DemandRevision != 2 {
		t.Fatalf("a refused revision must open a new one: %+v", next)
	}

	legacy := first.DeepCopy()
	legacy.DemandQueueSnapshot = ""
	if next := prepareDemandStatement(legacy, "101", "205", time.Now()); next.DemandRevision != 2 {
		t.Fatalf("a revision without stored snapshots cannot be replayed: %+v", next)
	}
}

// A workload whose status was never written has no /status object for a nested add.
func TestExternalStatePatchAddsWholeStatusWhenAbsent(t *testing.T) {
	state := &v1.WorkloadExternalExecution{DemandId: "d1"}
	fresh := &v1.Workload{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "7"}}
	if ops := externalStatePatch(fresh, state); ops[1]["path"] != "/status" {
		t.Fatalf("fresh workload patch = %v", ops)
	}
	started := fresh.DeepCopy()
	started.Status.Phase = v1.WorkloadPending
	if ops := externalStatePatch(started, state); ops[1]["path"] != "/status/externalExecution" {
		t.Fatalf("started workload patch = %v", ops)
	}
}

func TestClaimDispatchProblem(t *testing.T) {
	good := &execution.ClaimResponse{Placements: []execution.ClaimPlacement{{
		UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-1", ImageRef: pinnedImage}}}
	if p := claimDispatchProblem(good); p != "" {
		t.Fatalf("usable claim reported %q", p)
	}
	cases := map[string]*execution.ClaimResponse{
		"no placements": {},
		"no node": {Placements: []execution.ClaimPlacement{{
			UnitKey: v1.ExternalSingleUnitKey, ImageRef: pinnedImage}}},
		"tag image": {Placements: []execution.ClaimPlacement{{
			UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-1", ImageRef: "docker.io/team/app:v1"}}},
		"other unit": {Placements: []execution.ClaimPlacement{{
			UnitKey: "worker/0", NodeName: "vk-1", ImageRef: pinnedImage}}},
	}
	for name, claim := range cases {
		if claimDispatchProblem(claim) == "" {
			t.Errorf("%s: expected a problem", name)
		}
	}
}

func TestBuildDemandUnitsRejectsRDMA(t *testing.T) {
	w := gpuWorkload()
	w.Spec.Resources[0].RdmaResource = "1k"
	_, err := buildDemandUnits(w, "MI355X")
	var unsupported *unsupportedShapeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("RDMA request must be unsupported, got %v", err)
	}
	w.Spec.Resources[0].RdmaResource = "0"
	if _, err = buildDemandUnits(w, "MI355X"); err != nil {
		t.Fatalf("zero RDMA must be accepted: %v", err)
	}
}

// Every non-terminal outcome has to schedule another look, including a plain wait.
func TestExternalRetryDelay(t *testing.T) {
	if d, ok := externalRetryDelay(ExternalCapacityReason, nil); !ok || d != externalWaitRetry {
		t.Fatalf("plain wait = %v %v", d, ok)
	}
	if d, ok := externalRetryDelay(ExternalImageReason, errors.New("boom")); !ok || d != externalExchangeRetry {
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

func newRecoveryReconciler(t *testing.T, w *v1.Workload) *SchedulerReconciler {
	t.Helper()
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).
		WithObjects(w).WithStatusSubresource(&v1.Workload{}).Build()
	return &SchedulerReconciler{Client: cl}
}

func recoveryWorkload() (*v1.Workload, *v1.WorkloadExternalExecution) {
	state := &v1.WorkloadExternalExecution{
		DispatchGeneration: 1, DemandId: "d1", ClaimId: "c-old", ClaimRequestId: "r1",
	}
	w := gpuWorkload()
	w.Status.Phase = v1.WorkloadPending
	w.Status.ExternalExecution = state.DeepCopy()
	return w, state
}

// A claim naming another workload is not released, but its id is replaced so the next
// pass creates a reservation instead of reading the same claim again.
func TestAcceptClaimRemintsForeignClaim(t *testing.T) {
	w, state := recoveryWorkload()
	r := newRecoveryReconciler(t, w)
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	ok, _, err := r.acceptClaim(context.Background(), stored, state, &execution.ClaimResponse{
		ClaimID: "c-old", WorkloadUID: "someone-else", DispatchGeneration: 1, Phase: execution.ClaimPhaseActive,
	})
	if ok || err == nil {
		t.Fatalf("foreign claim admitted: ok=%v err=%v", ok, err)
	}
	after := &v1.Workload{}
	if err = r.Get(context.Background(), client.ObjectKey{Name: w.Name}, after); err != nil {
		t.Fatal(err)
	}
	if after.Status.ExternalExecution.ClaimId == "c-old" {
		t.Fatal("claim id was not replaced")
	}
}

// An Active claim the dispatcher would refuse is replaced here, not admitted.
func TestAcceptClaimRemintsUndispatchableClaim(t *testing.T) {
	w, state := recoveryWorkload()
	r := newRecoveryReconciler(t, w)
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	ok, reason, err := r.acceptClaim(context.Background(), stored, state, &execution.ClaimResponse{
		ClaimID: "c-old", WorkloadUID: string(w.UID), DispatchGeneration: 1, Phase: execution.ClaimPhaseActive,
		Placements: []execution.ClaimPlacement{{
			UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-1", ImageRef: "docker.io/team/app:v1"}},
	})
	if ok || err != nil || reason != ExternalCapacityReason {
		t.Fatalf("undispatchable claim: ok=%v reason=%q err=%v", ok, reason, err)
	}
	after := &v1.Workload{}
	if err = r.Get(context.Background(), client.ObjectKey{Name: w.Name}, after); err != nil {
		t.Fatal(err)
	}
	if after.Status.ExternalExecution.ClaimId == "c-old" || len(after.Status.ExternalExecution.Placements) != 0 {
		t.Fatalf("claim was not replaced: %+v", after.Status.ExternalExecution)
	}
}
