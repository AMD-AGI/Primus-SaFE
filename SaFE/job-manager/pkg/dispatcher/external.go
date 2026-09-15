/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/execution"
)

// externalClaimRecheckDelay spaces out retries when the reservation cannot be confirmed.
// The workload stays scheduled meanwhile: the reservation may still be valid and the
// failure transport-only, and dropping the schedule would send it back through admission
// for capacity it may already hold.
const externalClaimRecheckDelay = 10 * time.Second

// isExternalWorkload reports whether a workload was admitted against external capacity.
func isExternalWorkload(workload *v1.Workload) bool {
	return workload != nil && workload.Status.ExternalExecution != nil &&
		workload.Status.ExternalExecution.ClaimId != ""
}

// verifyExternalClaim rechecks the reservation immediately before the execution object is
// created.
//
// The scheduler already checked it, but time passes between marking a workload scheduled
// and building its pods, and the reservation can be revoked or expire inside that window.
// Dispatching against a claim that is no longer Active would place a pod on devices the
// provider has started reclaiming, and the pod would look legitimate to everything
// downstream.
func (r *DispatcherReconciler) verifyExternalClaim(ctx context.Context,
	workload *v1.Workload) error {
	state := workload.Status.ExternalExecution
	if state == nil || state.ClaimId == "" {
		return fmt.Errorf("workload %s has no external claim recorded", workload.Name)
	}
	client, err := execution.Shared()
	if err != nil {
		return err
	}
	claim, err := client.GetClaim(ctx, state.ClaimId)
	if err != nil {
		return err
	}
	// Matching the identity is the point of the recheck. An HTTP 200 only says the claim
	// exists; it does not say it belongs to this workload or to this attempt.
	if claim.WorkloadUID != string(workload.UID) {
		return fmt.Errorf("claim %s belongs to workload %s, not %s",
			state.ClaimId, claim.WorkloadUID, workload.UID)
	}
	if claim.DispatchGeneration != state.DispatchGeneration {
		return fmt.Errorf("claim %s is for dispatch generation %d, current is %d",
			state.ClaimId, claim.DispatchGeneration, state.DispatchGeneration)
	}
	if !claim.IsActive() {
		return fmt.Errorf("claim %s is %s", state.ClaimId, claim.Phase)
	}
	if !claim.ExpiresAt.IsZero() && time.Now().UTC().After(claim.ExpiresAt.Time) {
		return fmt.Errorf("claim %s admission expired at %s", state.ClaimId, claim.ExpiresAt.Time)
	}
	// Refuse rather than dispatch unconstrained. The node restriction is expressed as
	// affinity built from these placements, so an empty set would not narrow the pod to
	// anything -- it would let the execution cluster schedule it wherever it liked, on
	// capacity no claim covers.
	if len(externalApprovedNodes(workload)) == 0 {
		return fmt.Errorf("claim %s approved no nodes for workload %s", state.ClaimId, workload.Name)
	}
	if image := externalApprovedImage(workload, v1.ExternalSingleUnitKey); !isDigestPinned(image) {
		// The contract pins image_ref to a digest. Anything else means the pod would run
		// content the reservation was not granted against, and a tag can be moved after
		// the fact.
		return fmt.Errorf("claim %s approved image %q is not digest pinned", state.ClaimId, image)
	}
	return nil
}

// isDigestPinned reports whether a reference names immutable content.
func isDigestPinned(image string) bool {
	at := strings.LastIndex(image, "@sha256:")
	return at > 0 && len(image) == at+len("@sha256:")+64
}

// externalPodAnnotations are the identifiers the provider rechecks after the pod binds.
// They are derived from the approved workload and its claim, never from user input.
func externalPodAnnotations(workload *v1.Workload, unitKey string) map[string]interface{} {
	state := workload.Status.ExternalExecution
	if state == nil {
		return nil
	}
	profileID, profileRevision := commonconfig.GetExternalExecutionProfile()
	result := map[string]interface{}{
		v1.ExternalWorkloadUIDAnnotation: string(workload.UID),
		v1.ExternalDispatchGenAnnotation: strconv.Itoa(int(state.DispatchGeneration)),
		v1.ExternalClaimIdAnnotation:     state.ClaimId,
		v1.ExternalClaimRevAnnotation:    strconv.Itoa(int(state.ClaimRevision)),
		v1.ExternalUnitKeyAnnotation:     unitKey,
		v1.ExternalProfileIdAnnotation:   profileID,
		v1.ExternalProfileRevAnnotation:  strconv.Itoa(profileRevision),
	}
	if placement := findPlacement(state, unitKey); placement != nil {
		result[v1.ExternalAllocationIdAnnotation] = placement.AllocationId
	}
	return result
}

// findPlacement returns the approved seat for one unit.
func findPlacement(state *v1.WorkloadExternalExecution, unitKey string) *v1.WorkloadExternalPlacement {
	for i := range state.Placements {
		if state.Placements[i].UnitKey == unitKey {
			return &state.Placements[i]
		}
	}
	return nil
}

// externalApprovedNodes lists the virtual nodes a workload's pods may bind to. The pod is
// restricted to them through required node affinity, so the execution cluster scheduler
// still performs the binding rather than having it forced with spec.nodeName.
func externalApprovedNodes(workload *v1.Workload) []string {
	state := workload.Status.ExternalExecution
	if state == nil {
		return nil
	}
	names := make([]string, 0, len(state.Placements))
	seen := make(map[string]struct{}, len(state.Placements))
	for i := range state.Placements {
		name := state.Placements[i].NodeName
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// externalApprovedImage returns the image the provider froze at claim time. The contract
// requires this reference to name a digest, and verifyExternalClaim refuses the dispatch
// when it does not; the tag the user submitted is never used, because it can be moved to
// different content after the reservation was granted.
func externalApprovedImage(workload *v1.Workload, unitKey string) string {
	state := workload.Status.ExternalExecution
	if state == nil {
		return ""
	}
	if placement := findPlacement(state, unitKey); placement != nil {
		return placement.ImageRef
	}
	return ""
}
