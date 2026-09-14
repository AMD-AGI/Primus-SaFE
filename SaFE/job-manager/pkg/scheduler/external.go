/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/execution"
	commonquantity "github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// Waiting reasons surfaced for workloads in an external workspace. They are distinct on
// purpose: only a capacity shortage means acquiring more nodes would help, and treating an
// unprepared image or an unvalidated profile as a shortage would drive pointless growth.
const (
	ExternalCapacityReason    = "In queue - waiting for external capacity"
	ExternalImageReason       = "In queue - external image is being prepared"
	ExternalProfileReason     = "In queue - execution profile is not validated"
	ExternalConstraintReason  = "Rejected - constraints cannot be satisfied by external capacity"
	ExternalUnsupportedReason = "Rejected - workload shape is not supported by external capacity"
	ExternalUnavailableReason = "In queue - external capacity service is unavailable"
)

// demandExpiry bounds how long an unconsumed demand stays actionable. It is the admission
// window, not a limit on how long an already running task may run.
const demandExpiry = 5 * time.Minute

// demandRefreshMargin republishes a demand before it lapses, so the provider is never left
// without a current statement of need while the workload is still queued.
const demandRefreshMargin = time.Minute

// externalReleaseRetry paces the wait for a release to be confirmed. Revoking is not a
// terminal answer: it says the withdrawal was recorded, and the devices come back only when
// the provider has stopped the task and verified its cleanup.
const externalReleaseRetry = 15 * time.Second

// isExternalReclaiming reports whether a workload still holds provider capacity. It stays
// true from the moment a claim is created until the provider reports it released, which is
// what keeps the resources charged to the workspace across the workload's own end.
func isExternalReclaiming(workload *v1.Workload) bool {
	state := workload.Status.ExternalExecution
	return state != nil && state.ClaimId != "" && state.ClaimPhase != execution.ClaimPhaseReleased
}

// reconcileExternalRelease withdraws the reservation of a finished workload and reports
// whether the provider has yet to confirm it.
//
// Neither the workload ending nor the release call returning means the devices are free.
// The provider has to stop the task and verify its cleanup first, and until it says
// Released the reservation keeps counting against the workspace. Time passing, the pod
// disappearing and the release being acknowledged are all not evidence of that.
func (r *SchedulerReconciler) reconcileExternalRelease(ctx context.Context,
	workload *v1.Workload) (bool, error) {
	if !workload.IsEnd() || !isExternalReclaiming(workload) {
		return false, nil
	}
	state := workload.Status.ExternalExecution
	client, err := execution.Shared()
	if err != nil {
		return false, err
	}
	// Stop the acquisition first. The demand would lapse on its own at expires_at, but that
	// window is long enough for the provider to buy a node for a workload that has already
	// finished. Best effort: failing to withdraw must not hold up the release below, which
	// is the part that actually returns devices.
	if workspace, wsErr := r.getWorkspace(ctx, workload.Spec.Workspace); wsErr == nil && workspace != nil {
		if wdErr := r.withdrawExternalDemand(ctx, workload, workspace); wdErr != nil {
			klog.V(2).InfoS("failed to withdraw external demand", "workload", workload.Name,
				"error", wdErr)
		}
	}
	claim, err := client.ReleaseClaim(ctx, state.ClaimId, &execution.ReleaseRequest{
		RequestID:          uuid.NewString(),
		ExpectedRevision:   state.ClaimRevision,
		DispatchGeneration: state.DispatchGeneration,
		Reason:             string(workload.Status.Phase),
	})
	if err != nil {
		// A reservation the provider no longer knows about cannot be holding anything. Any
		// other failure leaves the state alone, so the resources stay charged.
		if execution.IsCode(err, execution.CodeNotFound) {
			return false, r.markClaimReleased(ctx, workload, state)
		}
		return false, err
	}

	updated := state.DeepCopy()
	updated.ClaimPhase = claim.Phase
	updated.ClaimRevision = claim.Revision
	updated.Reclaiming = claim.Phase != execution.ClaimPhaseReleased
	if err = r.patchExternalState(ctx, workload, updated); err != nil {
		return false, err
	}
	if updated.Reclaiming {
		klog.V(2).InfoS("external claim is not released yet", "workload", workload.Name,
			"claim", state.ClaimId, "phase", claim.Phase)
	}
	return updated.Reclaiming, nil
}

// markClaimReleased records that a reservation no longer exists on the provider.
func (r *SchedulerReconciler) markClaimReleased(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution) error {
	updated := state.DeepCopy()
	updated.ClaimPhase = execution.ClaimPhaseReleased
	updated.Reclaiming = false
	return r.patchExternalState(ctx, workload, updated)
}

// admitExternalCapacity runs the whole external admission for one queued workload: state
// the need, then try to take a seat.
//
// There is no local capacity test in front of this. Whether the devices exist is the
// provider's judgement, and the only thing this side can see -- the aggregate of the nodes
// the provider has already published -- says nothing about what it could still acquire.
// Gating on it would withhold the demand exactly when acquisition is needed, and since a
// failed plan produces no demand either, the workload would queue forever with nobody
// asked to buy anything.
//
// Publishing unconditionally does not cause over-buying: the provider subtracts its ready
// layout and its in-flight requests before acting on a demand.
func (r *SchedulerReconciler) admitExternalCapacity(ctx context.Context, workload *v1.Workload,
	workspace *v1.Workspace) (bool, string, error) {
	if err := r.ensureExternalDemand(ctx, workload, workspace); err != nil {
		var unsupported *unsupportedShapeError
		if errors.As(err, &unsupported) {
			// The shape cannot be expressed as a unit at all, so no amount of capacity
			// would help. Surfaced as a rejection rather than a wait.
			klog.ErrorS(err, "workload shape is not supported by external capacity",
				"workload", workload.Name)
			return false, ExternalUnsupportedReason, nil
		}
		return false, externalWaitingReason(err), nil
	}
	return r.reserveExternalCapacity(ctx, workload, workspace)
}

// ensureExternalDemand keeps a current statement of need on file with the provider.
//
// A revision is republished only when there is none or the present one is close to
// lapsing. The contract refuses a revision whose body changed, and the body carries its own
// observation time, so re-sending on every pass would either turn an intended replay into a
// conflict or make the revision climb without end.
func (r *SchedulerReconciler) ensureExternalDemand(ctx context.Context, workload *v1.Workload,
	workspace *v1.Workspace) error {
	units, err := buildDemandUnits(workload, workspace)
	if err != nil {
		return err
	}
	state, err := r.ensureExternalState(ctx, workload)
	if err != nil {
		return err
	}
	if !demandNeedsRefresh(state) {
		return nil
	}

	// Persisted before the call, like every other identifier here: a reply that never
	// arrives is then reconciled by replaying this exact revision and body.
	now := time.Now().UTC()
	observedAt := metav1.NewTime(now)
	expiresAt := metav1.NewTime(now.Add(demandExpiry))
	next := state.DeepCopy()
	next.DemandRevision = state.DemandRevision + 1
	next.DemandRequestId = uuid.NewString()
	next.DemandObservedAt = &observedAt
	next.DemandExpiresAt = &expiresAt
	if err = r.patchExternalState(ctx, workload, next); err != nil {
		return err
	}

	client, err := execution.Shared()
	if err != nil {
		return err
	}
	profileID, profileRevision := commonconfig.GetExternalExecutionProfile()
	if _, err = client.PublishDemand(ctx, &execution.CapacityDemand{
		RequestID:                next.DemandRequestId,
		DemandID:                 next.DemandId,
		Revision:                 next.DemandRevision,
		WorkloadUID:              string(workload.UID),
		DispatchGeneration:       next.DispatchGeneration,
		ClusterID:                workspace.Spec.Cluster,
		WorkspaceID:              workspace.Name,
		ProfileID:                profileID,
		ProfileRevision:          int32(profileRevision),
		QueueSnapshotRevision:    workload.ResourceVersion,
		CapacitySnapshotRevision: workspace.ResourceVersion,
		Eligible:                 true,
		Reason:                   execution.ReasonInsufficientCapacity,
		Units:                    units,
		ObservedAt:               execution.NewTimestamp(now),
		ExpiresAt:                execution.NewTimestamp(now.Add(demandExpiry)),
	}); err != nil {
		return err
	}
	klog.V(2).InfoS("published external capacity demand", "workload", workload.Name,
		"demand", next.DemandId, "revision", next.DemandRevision)
	return nil
}

// demandNeedsRefresh reports whether a new revision has to be published.
func demandNeedsRefresh(state *v1.WorkloadExternalExecution) bool {
	if state.DemandRevision == 0 || state.DemandExpiresAt == nil {
		return true
	}
	return time.Now().UTC().After(state.DemandExpiresAt.Time.Add(-demandRefreshMargin))
}

// unsupportedShapeError marks a workload the contract cannot express, as opposed to one
// that is merely waiting. The two must not share a reason: a wait implies more capacity
// would eventually help, and here none would.
type unsupportedShapeError struct{ reason string }

func (e *unsupportedShapeError) Error() string { return e.reason }

// withdrawExternalDemand tells the provider to stop acquiring capacity for a workload that
// is no longer waiting on it. Withdrawing the demand does not release a claim that was
// already granted; that is a separate release call.
func (r *SchedulerReconciler) withdrawExternalDemand(ctx context.Context, workload *v1.Workload,
	workspace *v1.Workspace) error {
	state := workload.Status.ExternalExecution
	if state == nil || state.DemandId == "" {
		return nil
	}
	client, err := execution.Shared()
	if err != nil {
		return err
	}
	units, err := buildDemandUnits(workload, workspace)
	if err != nil {
		return err
	}
	profileID, profileRevision := commonconfig.GetExternalExecutionProfile()
	now := time.Now().UTC()
	_, err = client.PublishDemand(ctx, &execution.CapacityDemand{
		RequestID:                uuid.NewString(),
		DemandID:                 state.DemandId,
		Revision:                 state.DemandRevision + 1,
		WorkloadUID:              string(workload.UID),
		DispatchGeneration:       state.DispatchGeneration,
		ClusterID:                workspace.Spec.Cluster,
		WorkspaceID:              workspace.Name,
		ProfileID:                profileID,
		ProfileRevision:          int32(profileRevision),
		QueueSnapshotRevision:    workload.ResourceVersion,
		CapacitySnapshotRevision: workspace.ResourceVersion,
		Eligible:                 false,
		Reason:                   execution.ReasonWithdrawn,
		Units:                    units,
		ObservedAt:               execution.NewTimestamp(now),
		ExpiresAt:                execution.NewTimestamp(now.Add(demandExpiry)),
	})
	return err
}

// reserveExternalCapacity turns available capacity into a reservation, and reports whether
// the workload may now leave the queue.
//
// A plan holds nothing: between planning and claiming, another workload can take the same
// devices. That is why a conflict here is normal and simply means replanning on the next
// pass, and why only a claim in the Active phase is allowed to release a dispatch.
func (r *SchedulerReconciler) reserveExternalCapacity(ctx context.Context, workload *v1.Workload,
	workspace *v1.Workspace) (bool, string, error) {
	client, err := execution.Shared()
	if err != nil {
		return false, ExternalUnavailableReason, nil
	}
	state, err := r.ensureExternalState(ctx, workload)
	if err != nil {
		return false, "", err
	}

	// A claim recorded earlier may already cover this dispatch. Re-reading it is also the
	// recovery path after a reply was lost: the reservation may exist even though this
	// process never saw the response that created it.
	if state.ClaimId != "" {
		existing, getErr := client.GetClaim(ctx, state.ClaimId)
		if getErr == nil {
			return r.acceptClaim(ctx, workload, state, existing)
		}
		if !execution.IsCode(getErr, execution.CodeNotFound) {
			return false, externalWaitingReason(getErr), nil
		}
	}

	plan, err := client.PlanPlacements(ctx, &execution.PlacementPlanRequest{
		RequestID:          uuid.NewString(),
		DemandID:           state.DemandId,
		DemandRevision:     state.DemandRevision,
		WorkloadUID:        string(workload.UID),
		DispatchGeneration: state.DispatchGeneration,
		WorkspaceID:        workspace.Name,
	})
	if err != nil {
		return false, externalWaitingReason(err), nil
	}

	claim, err := client.CreateClaim(ctx, &execution.ClaimRequest{
		RequestID:          state.ClaimRequestId,
		ClaimID:            state.ClaimId,
		WorkloadUID:        string(workload.UID),
		DispatchGeneration: state.DispatchGeneration,
		DemandID:           state.DemandId,
		DemandRevision:     state.DemandRevision,
		WorkspaceID:        workspace.Name,
		Placements:         plan.Placements,
	})
	if err != nil {
		return false, externalWaitingReason(err), nil
	}
	return r.acceptClaim(ctx, workload, state, claim)
}

// acceptClaim persists a reservation and reports whether it may back a dispatch.
func (r *SchedulerReconciler) acceptClaim(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution, claim *execution.ClaimResponse) (bool, string, error) {
	// Identity is rechecked rather than assumed. A reply that belongs to a different
	// workload or an older generation would otherwise authorise a dispatch that nothing
	// reserved capacity for.
	if claim.WorkloadUID != string(workload.UID) || claim.DispatchGeneration != state.DispatchGeneration {
		return false, ExternalCapacityReason, fmt.Errorf(
			"claim %s belongs to workload %s generation %d, expected %s generation %d",
			claim.ClaimID, claim.WorkloadUID, claim.DispatchGeneration,
			workload.UID, state.DispatchGeneration)
	}

	state.ClaimRevision = claim.Revision
	state.ClaimPhase = claim.Phase
	state.Placements = toStatusPlacements(claim.Placements)
	if err := r.patchExternalState(ctx, workload, state); err != nil {
		return false, "", err
	}
	if !claim.IsActive() {
		// Revoking or Released means the reservation is on its way out. Dispatching against
		// it would place a pod on devices the provider is reclaiming.
		return false, ExternalCapacityReason, nil
	}
	if !claim.ExpiresAt.IsZero() && time.Now().UTC().After(claim.ExpiresAt.Time) {
		return false, ExternalCapacityReason, nil
	}
	return true, "", nil
}

// ensureExternalState allocates and persists the identifiers before any request uses them.
//
// The order matters more than it looks. If the ids were generated per call, a reply lost in
// transit would leave the provider holding a demand or a reservation this side cannot name,
// and the next pass would create a second one.
func (r *SchedulerReconciler) ensureExternalState(ctx context.Context,
	workload *v1.Workload) (*v1.WorkloadExternalExecution, error) {
	generation := int32(v1.GetWorkloadDispatchCnt(workload) + 1)
	current := workload.Status.ExternalExecution
	if current != nil && current.DispatchGeneration == generation &&
		current.DemandId != "" && current.ClaimId != "" {
		return current.DeepCopy(), nil
	}

	// A new dispatch generation is a new attempt and gets its own identifiers. Reusing the
	// previous claim id would attach this attempt to a reservation made for the last one.
	// DemandRevision stays zero: ensureExternalDemand owns it and publishes the first
	// revision together with the observation window that has to stay fixed inside it.
	state := &v1.WorkloadExternalExecution{
		DispatchGeneration: generation,
		DemandId:           uuid.NewString(),
		ClaimId:            uuid.NewString(),
		ClaimRequestId:     uuid.NewString(),
	}
	if err := r.patchExternalState(ctx, workload, state); err != nil {
		return nil, err
	}
	return state, nil
}

// patchExternalState writes the bookkeeping back to the workload status.
func (r *SchedulerReconciler) patchExternalState(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution) error {
	if err := jobutils.PatchWorkloadStatusFields(ctx, r.Client, workload,
		map[string]any{"externalExecution": state}); err != nil {
		return err
	}
	workload.Status.ExternalExecution = state
	return nil
}

// buildDemandUnits expands a workload into the contract's units.
//
// The first release supports single-pod workloads only. A multi-replica workload needs a
// stable role and index on every child pod, and the operators that create those pods do not
// expose one that this side can bind a reservation to. Declining is the contract's own
// instruction for that case: the alternative is every replica claiming the same unit key.
func buildDemandUnits(workload *v1.Workload, workspace *v1.Workspace) ([]execution.DemandUnit, error) {
	if len(workload.Spec.Resources) != 1 || workload.Spec.Resources[0].Replica != 1 {
		return nil, &unsupportedShapeError{"external capacity supports single replica workloads only"}
	}
	res := &workload.Spec.Resources[0]
	if !res.HasGpu() {
		return nil, &unsupportedShapeError{"external capacity requires a gpu request"}
	}
	resources, err := toResourceVector(res, workspace)
	if err != nil {
		return nil, err
	}
	if len(workload.Spec.Images) == 0 || workload.Spec.Images[0] == "" {
		return nil, &unsupportedShapeError{"external capacity requires an image reference"}
	}

	constraints := execution.PlacementConstraints{
		NodeSelector:     map[string]string{},
		AllowedNodeNames: []string{},
	}
	digest, err := constraintsDigest(&constraints)
	if err != nil {
		return nil, err
	}
	runtimeSeconds := int32(3600)
	if workload.Spec.Timeout != nil && *workload.Spec.Timeout > 0 {
		runtimeSeconds = int32(*workload.Spec.Timeout)
	}
	return []execution.DemandUnit{{
		UnitKey:           "master/0",
		Replicas:          1,
		Resources:         resources,
		Ports:             []execution.Port{},
		RequiredRuntimeS:  runtimeSeconds,
		ImageRef:          workload.Spec.Images[0],
		Constraints:       constraints,
		ConstraintsDigest: digest,
	}}, nil
}

// toResourceVector converts a workload resource request into the contract's units:
// millicores, bytes and whole devices.
func toResourceVector(res *v1.WorkloadResource, workspace *v1.Workspace) (execution.ResourceVector, error) {
	list, err := commonquantity.CvtToResourceList(res.CPU, res.Memory, res.GPU, res.GPUName,
		res.EphemeralStorage, res.RdmaResource, 1)
	if err != nil {
		return execution.ResourceVector{}, err
	}
	cpu := list.Cpu()
	memory := list.Memory()
	scratch := list.StorageEphemeral()
	gpu := list[corev1.ResourceName(res.GPUName)]
	return execution.ResourceVector{
		CPUMillis:    cpu.MilliValue(),
		MemoryBytes:  memory.Value(),
		ScratchBytes: scratch.Value(),
		GPUResource:  res.GPUName,
		// The flavor identifies the machine type the workspace draws from, which is the
		// closest thing SaFE knows to a GPU model without a per-node lookup.
		GPUModel: workspace.Spec.NodeFlavor,
		GPUCount: int32(gpu.Value()),
	}, nil
}

// constraintsDigest is the sha256 over the canonical encoding of the supported constraint
// object: keys sorted, no whitespace, UTF-8. The provider recomputes it and compares, so
// the digest cannot stand in for the constraints themselves.
func constraintsDigest(constraints *execution.PlacementConstraints) (string, error) {
	encoded, err := json.Marshal(constraints)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// toStatusPlacements narrows the approved placements to what the dispatcher needs, keeping
// the wire response out of the stored object.
func toStatusPlacements(placements []execution.ClaimPlacement) []v1.WorkloadExternalPlacement {
	result := make([]v1.WorkloadExternalPlacement, 0, len(placements))
	for i := range placements {
		p := &placements[i]
		result = append(result, v1.WorkloadExternalPlacement{
			UnitKey:              p.UnitKey,
			NodeName:             p.NodeName,
			ImageRef:             p.ImageRef,
			ImageDigest:          p.ImageDigest,
			AllocationId:         p.AllocationID,
			AllocationGeneration: p.AllocationGeneration,
			DeviceIds:            p.DeviceIDs,
		})
	}
	return result
}

// externalWaitingReason maps a provider refusal to the reason shown on the workload.
//
// The distinction is the point. Reporting an unprepared image or an unvalidated profile as
// a capacity shortage would make the pool grow for a problem more nodes cannot solve.
func externalWaitingReason(err error) string {
	switch execution.CodeOf(err) {
	case execution.CodeCapacityUnavailable, execution.CodeConflict:
		return ExternalCapacityReason
	case execution.CodeImagePreparing:
		return ExternalImageReason
	case execution.CodeProfileUnvalidated:
		return ExternalProfileReason
	case execution.CodeConstraintUnsatisfiable:
		return ExternalConstraintReason
	default:
		return ExternalUnavailableReason
	}
}
