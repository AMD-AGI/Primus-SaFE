/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/execution"
)

// gangWorkload returns an RDMA PyTorchJob with one master and two workers.
func gangWorkload() *v1.Workload {
	w := gpuWorkload()
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: "v1"}
	w.Spec.Resources[0].RdmaResource = "1k"
	worker := w.Spec.Resources[0]
	worker.Replica = 2
	w.Spec.Resources = append(w.Spec.Resources, worker)
	w.Spec.JobPort = 23456
	return w
}

// gangClaim returns a claim that approves every unit of gangWorkload on its own node.
func gangClaim() *execution.ClaimResponse {
	vector := execution.ResourceVector{CPUMillis: 8000, GPUCount: 1}
	return &execution.ClaimResponse{Placements: []execution.ClaimPlacement{
		{UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-a", ImageRef: pinnedImage, Resources: vector},
		{UnitKey: "worker/0", NodeName: "vk-b", ImageRef: pinnedImage, Resources: vector},
		{UnitKey: "worker/1", NodeName: "vk-c", ImageRef: pinnedImage, Resources: vector},
	}}
}

func TestClaimDispatchProblemGang(t *testing.T) {
	w := gangWorkload()
	if p := claimDispatchProblem(w, gangClaim()); p != "" {
		t.Fatalf("usable gang claim reported %q", p)
	}
	missing := gangClaim()
	missing.Placements = missing.Placements[:2]
	duplicateNode := gangClaim()
	duplicateNode.Placements[2].NodeName = "vk-b"
	duplicateKey := gangClaim()
	duplicateKey.Placements[2].UnitKey = "worker/0"
	unknownKey := gangClaim()
	unknownKey.Placements[2].UnitKey = "worker/7"
	vector := gangClaim()
	vector.Placements[1].Resources.CPUMillis = 4000
	image := gangClaim()
	image.Placements[2].ImageRef = pinnedImage[:len(pinnedImage)-1] + "0"
	tagged := gangClaim()
	tagged.Placements[1].ImageRef = "docker.io/team/app:v1"
	cases := map[string]*execution.ClaimResponse{
		"missing unit": missing, "duplicate node": duplicateNode, "duplicate key": duplicateKey,
		"unknown key": unknownKey, "unequal vector": vector, "unequal image": image, "tag image": tagged,
	}
	for name, claim := range cases {
		if claimDispatchProblem(w, claim) == "" {
			t.Errorf("%s: expected a problem", name)
		}
	}
}

// A single-unit workload must get exactly its one unit back.
func TestClaimDispatchProblemSingleUnitRejectsExtraPlacement(t *testing.T) {
	claim := &execution.ClaimResponse{Placements: []execution.ClaimPlacement{
		{UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-1", ImageRef: pinnedImage},
		{UnitKey: "worker/0", NodeName: "vk-2", ImageRef: pinnedImage},
	}}
	if claimDispatchProblem(gpuWorkload(), claim) == "" {
		t.Fatal("extra placement must be a problem")
	}
}

// The contract vector has no RDMA field, so unequal RDMA requests would pass unnoticed.
func TestBuildDemandUnitsRejectsUnequalGangRDMA(t *testing.T) {
	w := gangWorkload()
	w.Spec.Resources[1].RdmaResource = "2k"
	_, err := buildDemandUnits(w, "MI355X")
	var unsupported *unsupportedShapeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("err = %v, want unsupportedShapeError", err)
	}
}

// newGangReconciler stores a gang workload without a job port and returns a stale copy.
func newGangReconciler(t *testing.T) (*SchedulerReconciler, *v1.Workload) {
	w := gangWorkload()
	w.Spec.JobPort = 0
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(w).Build()
	stored := &v1.Workload{}
	if err := cl.Get(context.Background(), ctrlclient.ObjectKeyFromObject(w), stored); err != nil {
		t.Fatal(err)
	}
	return &SchedulerReconciler{Client: cl}, stored
}

// A host-network job port is a host port; a privileged or out-of-range one cannot be bound.
func TestEnsureExternalJobPortRejectsTargetPortOutOfRange(t *testing.T) {
	r, w := newGangReconciler(t)
	w.Spec.Service = &v1.Service{TargetPort: 80}
	var unsupported *unsupportedShapeError
	if err := r.ensureExternalJobPort(context.Background(), w); !errors.As(err, &unsupported) {
		t.Fatalf("err = %v, want unsupportedShapeError", err)
	}
}

// A stale copy must not overwrite a port another reconcile already chose.
func TestEnsureExternalJobPortRefusesStaleCopy(t *testing.T) {
	r, stale := newGangReconciler(t)
	fresh := stale.DeepCopy()
	fresh.Spec.JobPort = 24000
	if err := r.Update(context.Background(), fresh); err != nil {
		t.Fatal(err)
	}
	if err := r.ensureExternalJobPort(context.Background(), stale); !apierrors.IsConflict(err) {
		t.Fatalf("err = %v, want conflict", err)
	}
}
