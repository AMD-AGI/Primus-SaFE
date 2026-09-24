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
	"strings"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/execution"
	commonquantity "github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
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
	ExternalRateLimitedReason = "In queue - external capacity controller rate limited"
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
	// IsEnd covers deletion as well as the terminal phases, which is what lets this run
	// before the finalizer is dropped.
	if !workload.IsEnd() || !isExternalReclaiming(workload) {
		return false, nil
	}
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
	// Re-read after the withdrawal, which rewrote the stored state. Working from the copy
	// captured before it would patch the withdrawal back out, and the next pass would
	// republish the same revision under a changed body forever.
	state := workload.Status.ExternalExecution
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

// releaseSupersededClaim gives back a reservation left over from an earlier dispatch
// generation, before its identifier is replaced.
//
// Unlike the terminal-state release this does not wait for Released: the devices come back
// on the provider's own schedule, and the new attempt is not competing for them -- it will
// plan against whatever is free when it asks. What matters is that the withdrawal is
// recorded against the id that owns them, which stops being possible the moment it is
// overwritten.
func (r *SchedulerReconciler) releaseSupersededClaim(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution) error {
	client, err := execution.Shared()
	if err != nil {
		return err
	}
	_, err = client.ReleaseClaim(ctx, state.ClaimId, &execution.ReleaseRequest{
		RequestID:          uuid.NewString(),
		ExpectedRevision:   state.ClaimRevision,
		DispatchGeneration: state.DispatchGeneration,
		Reason:             "superseded by a new dispatch generation",
	})
	if err != nil && !execution.IsCode(err, execution.CodeNotFound) {
		return err
	}
	klog.V(2).InfoS("released claim from a superseded dispatch generation",
		"workload", workload.Name, "claim", state.ClaimId, "generation", state.DispatchGeneration)
	return nil
}

// markClaimReleased records that a reservation no longer exists on the provider.
func (r *SchedulerReconciler) markClaimReleased(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution) error {
	updated := state.DeepCopy()
	updated.ClaimPhase = execution.ClaimPhaseReleased
	updated.Reclaiming = false
	return r.patchExternalState(ctx, workload, updated)
}

// requestExternalCapacity asks the provider to acquire nodes for a workload the workspace
// has no room for, and reports the wait.
//
// Reaching here means the aggregate synced from the execution cluster cannot hold this
// workload, which is the one situation where more hardware is the answer. Workloads waiting
// on anything else returned before the resource comparison and never ask for capacity.
func (r *SchedulerReconciler) requestExternalCapacity(ctx context.Context, workload *v1.Workload,
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
		// Preserve rate-limit / unavailable errors so the workspace is requeued after
		// the backoff the provider asked for, rather than hammering the same write.
		return false, externalWaitingReason(err), err
	}
	return false, ExternalCapacityReason, nil
}

// handleReservationRefusal decides what to do when a seat could not be taken even though
// the workspace aggregate said there was room.
//
// A capacity refusal here means the provider disagrees with the local view, and this side
// cannot tell why: devices may be held by a cleanup it has not confirmed, or the allocation
// behind a published node may be gone while the node object lingers. So the need is stated
// and the provider decides whether that calls for new hardware -- it subtracts its ready
// layout and in-flight requests first, so saying so cannot cause over-buying.
//
// Without this the workload would replan forever against a local view that nothing
// corrects, with the provider never told anyone was waiting.
//
// Other refusals are left alone. An unprepared image or an unvalidated profile is not
// something more nodes would fix, and a transport failure is not an answer at all.
func (r *SchedulerReconciler) handleReservationRefusal(ctx context.Context, workload *v1.Workload,
	workspace *v1.Workspace, cause error) (bool, string, error) {
	reason := externalWaitingReason(cause)
	if !execution.IsCode(cause, execution.CodeCapacityUnavailable) {
		// Rate-limited / unavailable answers carry a backoff; keep the error so
		// externalOutcome can re-stage the workspace after Retry-After.
		if execution.RetryAfterOf(cause) > 0 {
			return false, reason, cause
		}
		return false, reason, nil
	}
	if err := r.ensureExternalDemand(ctx, workload, workspace); err != nil {
		klog.ErrorS(err, "failed to state capacity need after a refused reservation",
			"workload", workload.Name)
		if execution.RetryAfterOf(err) > 0 {
			return false, externalWaitingReason(err), err
		}
	}
	return false, reason, nil
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

	// The identifiers are persisted before the call, so a reply that never arrives is
	// reconciled by replaying this exact revision and body.
	//
	// The expiry is persisted only after the provider has accepted it, and it is what
	// demandNeedsRefresh reads to decide the demand is current. Recording it upfront would
	// make a failed publish look like a live demand and suppress every retry until the
	// window ran out, leaving the provider with no statement of need at all.
	next := state.DeepCopy()
	if next.DemandRevision == 0 || next.DemandExpiresAt != nil {
		// Either nothing has been published, or the last revision was accepted. Both mean
		// this is a new statement and needs its own revision and observation time.
		observedAt := metav1.NewTime(time.Now().UTC())
		next.DemandRevision++
		next.DemandRequestId = uuid.NewString()
		next.DemandObservedAt = &observedAt
	}
	// A retry of an unconfirmed publish keeps the revision, the request id and the
	// observation time, so the body is byte for byte what the first attempt sent.
	next.DemandExpiresAt = nil
	if err = r.patchExternalState(ctx, workload, next); err != nil {
		return err
	}

	client, err := execution.Shared()
	if err != nil {
		return err
	}
	observedAt := next.DemandObservedAt.Time.UTC()
	expiresAt := observedAt.Add(demandExpiry)
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
		ObservedAt:               execution.NewTimestamp(observedAt),
		ExpiresAt:                execution.NewTimestamp(expiresAt),
	}); err != nil {
		return err
	}

	accepted := next.DeepCopy()
	acceptedExpiry := metav1.NewTime(expiresAt)
	accepted.DemandExpiresAt = &acceptedExpiry
	if err = r.patchExternalState(ctx, workload, accepted); err != nil {
		return err
	}
	klog.V(2).InfoS("published external capacity demand", "workload", workload.Name,
		"demand", accepted.DemandId, "revision", accepted.DemandRevision)
	return nil
}

// demandNeedsRefresh reports whether the provider needs a statement of need published.
//
// A nil expiry means the last attempt was never confirmed, so it has to be retried rather
// than waited out.
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
//
// The new revision is persisted before it is sent, and the withdrawal is skipped once it
// has been recorded. Otherwise every retry would republish the same revision number under a
// different body and a different timestamp, which the contract refuses, and the number
// would eventually collide with one ensureExternalDemand issues for the opposite meaning.
func (r *SchedulerReconciler) withdrawExternalDemand(ctx context.Context, workload *v1.Workload,
	workspace *v1.Workspace) error {
	state := workload.Status.ExternalExecution
	if state == nil || state.DemandId == "" || state.DemandWithdrawn {
		return nil
	}
	units, err := buildDemandUnits(workload, workspace)
	if err != nil {
		return err
	}
	client, err := execution.Shared()
	if err != nil {
		return err
	}

	// Same two steps as publishing a need: the revision is fixed before the call so a retry
	// resends an identical body, and the outcome is recorded only once the provider has
	// taken it. Marking the withdrawal upfront would make a failed call permanent, since
	// the guard above then skips it forever.
	next := state.DeepCopy()
	if next.DemandExpiresAt != nil {
		observedAt := metav1.NewTime(time.Now().UTC())
		next.DemandRevision++
		next.DemandRequestId = uuid.NewString()
		next.DemandObservedAt = &observedAt
	}
	next.DemandExpiresAt = nil
	if err = r.patchExternalState(ctx, workload, next); err != nil {
		return err
	}

	observedAt := next.DemandObservedAt.Time.UTC()
	expiresAt := observedAt.Add(demandExpiry)
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
		Eligible:                 false,
		Reason:                   execution.ReasonWithdrawn,
		Units:                    units,
		ObservedAt:               execution.NewTimestamp(observedAt),
		ExpiresAt:                execution.NewTimestamp(expiresAt),
	}); err != nil {
		return err
	}

	accepted := next.DeepCopy()
	acceptedExpiry := metav1.NewTime(expiresAt)
	accepted.DemandExpiresAt = &acceptedExpiry
	accepted.DemandWithdrawn = true
	return r.patchExternalState(ctx, workload, accepted)
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
			return false, externalWaitingReason(getErr), getErr
		}
	}

	// A claim references the demand it was planned against, so one has to exist even when
	// the workspace already has room and no acquisition is needed.
	if err = r.ensureExternalDemand(ctx, workload, workspace); err != nil {
		var unsupported *unsupportedShapeError
		if errors.As(err, &unsupported) {
			return false, ExternalUnsupportedReason, nil
		}
		return false, externalWaitingReason(err), err
	}
	state = workload.Status.ExternalExecution

	plan, err := client.PlanPlacements(ctx, &execution.PlacementPlanRequest{
		RequestID:          uuid.NewString(),
		DemandID:           state.DemandId,
		DemandRevision:     state.DemandRevision,
		WorkloadUID:        string(workload.UID),
		DispatchGeneration: state.DispatchGeneration,
		WorkspaceID:        workspace.Name,
	})
	if err != nil {
		return r.handleReservationRefusal(ctx, workload, workspace, err)
	}

	// A fresh request id for this set of placements. The id is the server's idempotency
	// key, and replanning produces a different body: reusing the previous id would be
	// refused as a conflicting replay, so a claim that failed once could never succeed
	// again within the same dispatch generation. Safe because a claim that did get created
	// is found by the GetClaim above and never reaches this call.
	claimState := state.DeepCopy()
	claimState.ClaimRequestId = uuid.NewString()
	if err = r.patchExternalState(ctx, workload, claimState); err != nil {
		return false, "", err
	}
	state = claimState

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
		return r.handleReservationRefusal(ctx, workload, workspace, err)
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
	//
	// The previous reservation has to be handed back first. Overwriting the id would strand
	// it on the provider with nothing left on this side naming it, and a workload that keeps
	// failing over would leak one reservation per attempt. A failure here blocks the new
	// attempt on purpose: waiting is recoverable, a leak is not.
	if current != nil && current.ClaimId != "" && current.ClaimPhase != execution.ClaimPhaseReleased {
		if err := r.releaseSupersededClaim(ctx, workload, current); err != nil {
			return nil, err
		}
	}
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
//
// A JSON patch, not a merge patch. Every field of the stored object is omitempty, so under
// merge semantics a field returning to its zero value simply vanishes from the payload and
// the old value survives. That makes two things impossible to express: clearing Reclaiming
// once it has been set, and starting a new dispatch generation with a clean slate -- the
// previous generation's demand revision and placements would carry over, and the workload
// would then plan against a demand id the provider has never seen.
//
// The test operation gives the same optimistic concurrency the merge path had through
// metadata.resourceVersion.
func (r *SchedulerReconciler) patchExternalState(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution) error {
	patch := []map[string]any{
		{"op": "test", "path": "/metadata/resourceVersion", "value": workload.ResourceVersion},
		{"op": "add", "path": "/status/externalExecution", "value": state},
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	if err = r.Status().Patch(ctx, workload, client.RawPatch(apitypes.JSONPatchType, raw)); err != nil {
		klog.ErrorS(err, "failed to patch external execution state", "workload", workload.Name)
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
	if len(workload.Spec.Images) != 1 || workload.Spec.Images[0] == "" {
		return nil, &unsupportedShapeError{
			"external capacity supports a single digest-pinned image only"}
	}
	imageRef := workload.Spec.Images[0]
	if !isDigestPinnedImage(imageRef) {
		return nil, &unsupportedShapeError{
			"external capacity requires an image reference pinned with @sha256:"}
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
	imageDigest := imageRef[strings.LastIndex(imageRef, "@sha256:")+1:]
	return []execution.DemandUnit{{
		UnitKey:           "master/0",
		Replicas:          1,
		Resources:         resources,
		Ports:             []execution.Port{},
		RequiredRuntimeS:  runtimeSeconds,
		PIDLimit:          commonconfig.GetExternalPIDLimit(),
		ImageRef:          imageRef,
		ImageDigest:       imageDigest,
		Constraints:       constraints,
		ConstraintsDigest: digest,
	}}, nil
}

// isDigestPinnedImage reports whether the reference names immutable content.
func isDigestPinnedImage(image string) bool {
	at := strings.LastIndex(image, "@sha256:")
	return at > 0 && len(image) == at+len("@sha256:")+64
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
	case execution.CodeRateLimited:
		return ExternalRateLimitedReason
	default:
		return ExternalUnavailableReason
	}
}
