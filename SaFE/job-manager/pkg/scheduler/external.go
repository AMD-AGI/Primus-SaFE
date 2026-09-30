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
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	ExternalInvalidReason     = "Rejected - request refused by external capacity"
	ExternalUnavailableReason = "In queue - external capacity service is unavailable"
	ExternalRateLimitedReason = "In queue - external capacity controller rate limited"
	ExternalAuthReason        = "In queue - not authorized by external capacity"
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

// externalExchangeRetry re-stages the workspace schedule after a failed demand/claim
// exchange that carried no Retry-After (for example a stale status patch).
const externalExchangeRetry = 10 * time.Second

// externalWaitRetry re-stages the workspace schedule while a workload waits on the provider
// without an error, so its demand is renewed before it lapses.
const externalWaitRetry = 30 * time.Second

// isTerminalExternalReason reports whether a provider answer rules the workload out for
// good, as opposed to asking it to wait.
// Terminal reasons may carry the provider's message after the prefix.
func isTerminalExternalReason(reason string) bool {
	for _, prefix := range []string{ExternalUnsupportedReason, ExternalConstraintReason, ExternalInvalidReason} {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}

// externalStatePatchAttempts covers create-time races where another writer bumps
// resourceVersion between the cached read and the JSON-patch test.
const externalStatePatchAttempts = 5

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
		// No client can be built (feature off, endpoint or mTLS missing), so no retry can
		// release anything. On deletion the hold is abandoned so the finalizer can go.
		if !workload.GetDeletionTimestamp().IsZero() {
			klog.ErrorS(err, "abandoning external claim release for deleted workload",
				"workload", workload.Name, "claim", workload.Status.ExternalExecution.ClaimId)
			return false, nil
		}
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
	state := workload.Status.ExternalExecution
	// An accepted release is not repeated: the provider is already stopping the task, and
	// only a read can tell when it has verified the cleanup.
	if state.ClaimPhase == execution.ClaimPhaseRevoking {
		return r.pollRevokingClaim(ctx, client, workload, state)
	}
	state, err = r.ensureReleaseRequest(ctx, workload, state, releaseReason(workload))
	if err != nil {
		return false, err
	}
	claim, err := client.ReleaseClaim(ctx, state.ClaimId, releaseRequest(state))
	if err != nil {
		// A reservation the provider no longer knows about cannot be holding anything. Any
		// other failure leaves the state alone, so the resources stay charged.
		if execution.IsCode(err, execution.CodeNotFound) {
			return false, r.markClaimReleased(ctx, workload, state)
		}
		// A conflict is either a stale expected_revision or a body the provider does not
		// accept under this id. Neither is evidence the devices are still held; a read
		// decides what the next release has to carry.
		if execution.IsCode(err, execution.CodeConflict) {
			return r.reconcileStaleRelease(ctx, client, workload, state, err)
		}
		return false, err
	}
	return r.recordClaimObservation(ctx, workload, state, claim)
}

// pollRevokingClaim reads a claim whose release was accepted and reports whether it is
// still held. Read failures keep the hold and wait for the next pass.
func (r *SchedulerReconciler) pollRevokingClaim(ctx context.Context, client *execution.Client,
	workload *v1.Workload, state *v1.WorkloadExternalExecution) (bool, error) {
	claim, err := client.GetClaim(ctx, state.ClaimId)
	if err != nil {
		if execution.IsCode(err, execution.CodeNotFound) {
			return false, r.markClaimReleased(ctx, workload, state)
		}
		klog.V(2).InfoS("failed to read revoking external claim", "workload", workload.Name,
			"claim", state.ClaimId, "error", err)
		return true, nil
	}
	return r.recordClaimObservation(ctx, workload, state, claim)
}

// recordClaimObservation stores the phase and revision the provider reported and reports
// whether the claim is still held. A claim seen Active again drops the frozen release so
// the next pass releases the revision it now carries.
func (r *SchedulerReconciler) recordClaimObservation(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution, claim *execution.ClaimResponse) (bool, error) {
	if claim.Phase == execution.ClaimPhaseReleased {
		updated := state.DeepCopy()
		updated.ClaimRevision = claim.Revision
		return false, r.markClaimReleased(ctx, workload, updated)
	}
	updated := state.DeepCopy()
	updated.ClaimPhase = claim.Phase
	updated.ClaimRevision = claim.Revision
	updated.Reclaiming = true
	if claim.Phase == execution.ClaimPhaseActive {
		clearReleaseRequest(updated)
	}
	if err := r.patchExternalState(ctx, workload, updated); err != nil {
		return false, err
	}
	klog.V(2).InfoS("external claim is not released yet", "workload", workload.Name,
		"claim", state.ClaimId, "phase", claim.Phase, "revision", claim.Revision)
	return true, nil
}

// reconcileStaleRelease re-reads a claim after ReleaseClaim answered Conflict and reports
// whether it is still held. Released and Revoking are recorded. A claim still Active was
// not released by the refused request, so its frozen body is dropped and the next pass
// sends the observed revision under a new id.
func (r *SchedulerReconciler) reconcileStaleRelease(ctx context.Context, client *execution.Client,
	workload *v1.Workload, state *v1.WorkloadExternalExecution, releaseErr error) (bool, error) {
	observed, err := client.GetClaim(ctx, state.ClaimId)
	if err != nil {
		if execution.IsCode(err, execution.CodeNotFound) {
			return false, r.markClaimReleased(ctx, workload, state)
		}
		return false, releaseErr
	}
	klog.V(2).InfoS("external claim release was refused, re-read the claim", "workload", workload.Name,
		"claim", state.ClaimId, "phase", observed.Phase, "revision", observed.Revision, "error", releaseErr)
	return r.recordClaimObservation(ctx, workload, state, observed)
}

// releaseReason names why a claim is released. The provider refuses an empty reason, and a
// workload deleted outside the API has no phase.
func releaseReason(workload *v1.Workload) string {
	if workload.Status.Phase != "" {
		return string(workload.Status.Phase)
	}
	if !workload.GetDeletionTimestamp().IsZero() {
		return "Deleted"
	}
	return "Finished"
}

// releaseRequest builds the withdrawal from the body frozen with its request id.
func releaseRequest(state *v1.WorkloadExternalExecution) *execution.ReleaseRequest {
	return &execution.ReleaseRequest{
		RequestID:          state.ReleaseRequestId,
		ExpectedRevision:   state.ReleaseExpectedRevision,
		DispatchGeneration: state.DispatchGeneration,
		Reason:             state.ReleaseReason,
	}
}

// ensureReleaseRequest persists the ReleaseClaim idempotency key together with the body it
// is sent with, so a replay under the same id carries the same body.
func (r *SchedulerReconciler) ensureReleaseRequest(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution, reason string) (*v1.WorkloadExternalExecution, error) {
	if state.ReleaseRequestId != "" && state.ReleaseExpectedRevision > 0 && state.ReleaseReason != "" {
		return state, nil
	}
	next := state.DeepCopy()
	if next.ReleaseRequestId == "" {
		next.ReleaseRequestId = uuid.NewString()
	}
	next.ReleaseExpectedRevision = claimExpectedRevision(state)
	next.ReleaseReason = reason
	if err := r.patchExternalState(ctx, workload, next); err != nil {
		return nil, err
	}
	return next, nil
}

// clearReleaseRequest drops the release id and the body frozen with it.
func clearReleaseRequest(state *v1.WorkloadExternalExecution) {
	state.ReleaseRequestId = ""
	state.ReleaseExpectedRevision = 0
	state.ReleaseReason = ""
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
	state, err := r.ensureReleaseRequest(ctx, workload, state, "superseded by a new dispatch generation")
	if err != nil {
		return err
	}
	client, err := execution.Shared()
	if err != nil {
		return err
	}
	_, err = client.ReleaseClaim(ctx, state.ClaimId, releaseRequest(state))
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
	clearReleaseRequest(updated)
	return r.patchExternalState(ctx, workload, updated)
}

// claimExpectedRevision returns the revision the provider requires on release. A claim that
// never got an accepted CreateClaim keeps ClaimRevision at zero, which the contract rejects.
func claimExpectedRevision(state *v1.WorkloadExternalExecution) int32 {
	if state == nil || state.ClaimRevision < 1 {
		return 1
	}
	return state.ClaimRevision
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
	gpuModel, err := r.nodeFlavorGPUModel(ctx, workspace)
	if err != nil {
		return err
	}
	units, err := buildDemandUnits(workload, gpuModel)
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
	next := prepareDemandStatement(state, workload.ResourceVersion, workspace.ResourceVersion,
		time.Now())
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
		QueueSnapshotRevision:    next.DemandQueueSnapshot,
		CapacitySnapshotRevision: next.DemandCapacitySnapshot,
		Eligible:                 true,
		Reason:                   execution.ReasonInsufficientCapacity,
		Units:                    units,
		ObservedAt:               execution.NewTimestamp(observedAt),
		ExpiresAt:                execution.NewTimestamp(expiresAt),
	}); err != nil {
		// A refused replay means the provider holds a different body under this revision.
		// Clearing the request id makes the next pass open a new revision.
		if execution.IsCode(err, execution.CodeConflict) {
			abandoned := next.DeepCopy()
			abandoned.DemandRequestId = ""
			if patchErr := r.patchExternalState(ctx, workload, abandoned); patchErr != nil {
				klog.ErrorS(patchErr, "failed to abandon refused demand revision",
					"workload", workload.Name, "revision", next.DemandRevision)
			}
		}
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

// prepareDemandStatement returns the state for the next eligible publish. A retry of an
// unconfirmed revision keeps its request id, observation time and snapshots, so the body
// is byte for byte what the first attempt sent. Anything else opens a new revision.
func prepareDemandStatement(state *v1.WorkloadExternalExecution, queueSnapshot, capacitySnapshot string,
	now time.Time) *v1.WorkloadExternalExecution {
	next := state.DeepCopy()
	if next.DemandRevision == 0 || next.DemandExpiresAt != nil || next.DemandRequestId == "" ||
		next.DemandObservedAt == nil || next.DemandQueueSnapshot == "" || next.DemandCapacitySnapshot == "" {
		observedAt := metav1.NewTime(now.UTC())
		next.DemandRevision++
		next.DemandRequestId = uuid.NewString()
		next.DemandObservedAt = &observedAt
		next.DemandQueueSnapshot = queueSnapshot
		next.DemandCapacitySnapshot = capacitySnapshot
	}
	next.DemandExpiresAt = nil
	return next
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
	// A demand that was never published has no provider-side statement to stop.
	// DemandObservedAt is also unset in that case; proceeding would panic on it.
	if state.DemandRevision == 0 {
		return nil
	}
	gpuModel, err := r.nodeFlavorGPUModel(ctx, workspace)
	if err != nil {
		return err
	}
	units, err := buildDemandUnits(workload, gpuModel)
	if err != nil {
		return err
	}
	client, err := execution.Shared()
	if err != nil {
		return err
	}

	// A withdrawal always changes Eligible and Reason, so the contract requires a new
	// revision — reusing the eligible revision under a withdrawn body is refused.
	// Persist the bumped revision and fixed observation before the call. DemandWithdrawn
	// is set only after acceptance so a failed call is not skipped forever. Retries open
	// a further revision; that remains a valid new statement until one is accepted.
	next := state.DeepCopy()
	observedAt := metav1.NewTime(time.Now().UTC())
	next.DemandRevision++
	next.DemandRequestId = uuid.NewString()
	next.DemandObservedAt = &observedAt
	next.DemandQueueSnapshot = workload.ResourceVersion
	next.DemandCapacitySnapshot = workspace.ResourceVersion
	next.DemandExpiresAt = nil
	if err = r.patchExternalState(ctx, workload, next); err != nil {
		return err
	}

	observedAtTime := next.DemandObservedAt.Time.UTC()
	expiresAt := observedAtTime.Add(demandExpiry)
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
		QueueSnapshotRevision:    next.DemandQueueSnapshot,
		CapacitySnapshotRevision: next.DemandCapacitySnapshot,
		Eligible:                 false,
		Reason:                   execution.ReasonWithdrawn,
		Units:                    units,
		ObservedAt:               execution.NewTimestamp(observedAtTime),
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
		// Returned, not dropped. externalOutcome logs it and re-stages the workspace;
		// a nil error here leaves the workload waiting with no wake-up.
		return false, ExternalUnavailableReason, err
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
			identityMatches := existing.WorkloadUID == string(workload.UID) &&
				existing.DispatchGeneration == state.DispatchGeneration
			if existing.IsActive() || !identityMatches {
				return r.acceptClaim(ctx, workload, state, existing)
			}
			// Revoking or Released cannot back this dispatch and will not become Active
			// again. A new claim id lets the same generation reserve fresh capacity.
			if state, err = r.remintClaim(ctx, workload, state); err != nil {
				return false, "", err
			}
		} else if !execution.IsCode(getErr, execution.CodeNotFound) {
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
	// reserved capacity for. The claim id is replaced so the next pass creates a new
	// reservation instead of reading the same one again.
	if claim.WorkloadUID != string(workload.UID) || claim.DispatchGeneration != state.DispatchGeneration {
		// A claim of this workload from another generation is ours to return. One naming
		// another workload is not, and releasing it would stop that workload's task.
		if claim.WorkloadUID == string(workload.UID) {
			if relErr := r.releaseUnusableClaim(ctx, workload, claim, "claim identity mismatch"); relErr != nil {
				klog.ErrorS(relErr, "failed to release mismatched claim",
					"workload", workload.Name, "claim", claim.ClaimID)
			}
		}
		if _, err := r.remintClaim(ctx, workload, state); err != nil {
			return false, "", err
		}
		return false, ExternalCapacityReason, fmt.Errorf(
			"claim %s belongs to workload %s generation %d, expected %s generation %d",
			claim.ClaimID, claim.WorkloadUID, claim.DispatchGeneration,
			workload.UID, state.DispatchGeneration)
	}

	// The dispatcher refuses the same shapes. Checking them here returns the claim and
	// tries again after a delay, instead of admitting a workload the dispatcher then
	// sends straight back.
	if claim.IsActive() {
		if problem := claimDispatchProblem(claim); problem != "" {
			klog.ErrorS(nil, "external claim cannot back a dispatch", "workload", workload.Name,
				"claim", claim.ClaimID, "problem", problem)
			if relErr := r.releaseUnusableClaim(ctx, workload, claim, problem); relErr != nil {
				klog.ErrorS(relErr, "failed to release unusable claim",
					"workload", workload.Name, "claim", claim.ClaimID)
			}
			if _, err := r.remintClaim(ctx, workload, state); err != nil {
				return false, "", err
			}
			return false, ExternalCapacityReason, nil
		}
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

// claimDispatchProblem reports why an Active claim cannot back a dispatch, or "" when it
// can: it must approve at least one node and pin the unit's image to a digest.
func claimDispatchProblem(claim *execution.ClaimResponse) string {
	hasNode := false
	for i := range claim.Placements {
		if claim.Placements[i].NodeName != "" {
			hasNode = true
			break
		}
	}
	if !hasNode {
		return "claim approved no nodes"
	}
	for i := range claim.Placements {
		p := &claim.Placements[i]
		if p.UnitKey != v1.ExternalSingleUnitKey {
			continue
		}
		if !isDigestPinnedImage(p.ImageRef) {
			return fmt.Sprintf("claim approved image %q is not digest pinned", p.ImageRef)
		}
		return ""
	}
	return "claim has no placement for unit " + v1.ExternalSingleUnitKey
}

// releaseUnusableClaim returns a reservation of this workload that cannot back a dispatch.
func (r *SchedulerReconciler) releaseUnusableClaim(ctx context.Context, workload *v1.Workload,
	claim *execution.ClaimResponse, reason string) error {
	client, err := execution.Shared()
	if err != nil {
		return err
	}
	revision := claim.Revision
	if revision < 1 {
		revision = 1
	}
	_, err = client.ReleaseClaim(ctx, claim.ClaimID, &execution.ReleaseRequest{
		RequestID:          uuid.NewString(),
		ExpectedRevision:   revision,
		DispatchGeneration: claim.DispatchGeneration,
		Reason:             reason,
	})
	if err != nil && !execution.IsCode(err, execution.CodeNotFound) {
		return err
	}
	klog.V(2).InfoS("released unusable external claim",
		"workload", workload.Name, "claim", claim.ClaimID, "reason", reason,
		"claimWorkload", claim.WorkloadUID, "claimGeneration", claim.DispatchGeneration)
	return nil
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
		if current.ClaimPhase != execution.ClaimPhaseReleased {
			return current.DeepCopy(), nil
		}
		// Still queued for this dispatch, but the prior claim is gone. Keep the demand id
		// so ensureExternalDemand can refresh it; mint a new claim id for CreateClaim.
		return r.remintClaim(ctx, workload, current)
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

// remintClaim replaces the claim identity within the current dispatch generation, keeping
// the demand, so the next CreateClaim reserves fresh capacity.
func (r *SchedulerReconciler) remintClaim(ctx context.Context, workload *v1.Workload,
	current *v1.WorkloadExternalExecution) (*v1.WorkloadExternalExecution, error) {
	state := current.DeepCopy()
	state.ClaimId = uuid.NewString()
	state.ClaimRequestId = uuid.NewString()
	state.ClaimPhase = ""
	state.ClaimRevision = 0
	clearReleaseRequest(state)
	state.Placements = nil
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
// metadata.resourceVersion. A failed test returns 422 Invalid (not Conflict); concurrent
// status writers return Conflict. Both are retried after a fresh Get.
func (r *SchedulerReconciler) patchExternalState(ctx context.Context, workload *v1.Workload,
	state *v1.WorkloadExternalExecution) error {
	var err error
	for attempt := 0; attempt < externalStatePatchAttempts; attempt++ {
		if attempt > 0 {
			if getErr := r.Get(ctx, client.ObjectKey{Name: workload.Name}, workload); getErr != nil {
				return getErr
			}
		}
		var raw []byte
		raw, err = json.Marshal(externalStatePatch(workload, state))
		if err != nil {
			return err
		}
		err = r.Status().Patch(ctx, workload, client.RawPatch(apitypes.JSONPatchType, raw))
		if err == nil {
			workload.Status.ExternalExecution = state
			return nil
		}
		if !apierrors.IsConflict(err) && !apierrors.IsInvalid(err) {
			klog.ErrorS(err, "failed to patch external execution state", "workload", workload.Name)
			return err
		}
		klog.V(2).InfoS("retrying external execution state patch",
			"workload", workload.Name, "attempt", attempt+1, "err", err)
	}
	klog.ErrorS(err, "failed to patch external execution state", "workload", workload.Name)
	return err
}

// externalStatePatch builds the JSON patch for patchExternalState. A workload whose status
// has never been written has no /status object for an add to land in, so the whole status
// is added instead; with nothing stored there, that replaces nothing.
func externalStatePatch(workload *v1.Workload, state *v1.WorkloadExternalExecution) []map[string]any {
	test := map[string]any{"op": "test", "path": "/metadata/resourceVersion", "value": workload.ResourceVersion}
	if reflect.DeepEqual(workload.Status, v1.WorkloadStatus{}) {
		return []map[string]any{test,
			{"op": "add", "path": "/status", "value": map[string]any{"externalExecution": state}}}
	}
	return []map[string]any{test,
		{"op": "add", "path": "/status/externalExecution", "value": state}}
}

// buildDemandUnits expands a workload into the contract's units.
//
// The first release supports single-pod workloads only. A multi-replica workload needs a
// stable role and index on every child pod, and the operators that create those pods do not
// expose one that this side can bind a reservation to. Declining is the contract's own
// instruction for that case: the alternative is every replica claiming the same unit key.
//
// CPU-only units and tag image references are allowed: the provider accepts GPUCount 0 and
// resolves a tag to a digest at claim time. Spec.Images[0] is the main-container image; a
// per-container image list (init + main) is a later contract extension.
//
// gpuModel is NodeFlavor.spec.gpu.product; the provider indexes free devices by that value.
func buildDemandUnits(workload *v1.Workload, gpuModel string) ([]execution.DemandUnit, error) {
	if len(workload.Spec.Resources) != 1 || workload.Spec.Resources[0].Replica != 1 {
		return nil, &unsupportedShapeError{"external capacity supports single replica workloads only"}
	}
	res := &workload.Spec.Resources[0]
	// The contract's resource vector has no RDMA field, so a request for it could only
	// be dropped and the task would start without the devices it asked for.
	if res.RdmaResource != "" && res.RdmaResource != "0" {
		return nil, &unsupportedShapeError{"external capacity does not support RDMA resources"}
	}
	resources, err := toResourceVector(res, gpuModel)
	if err != nil {
		return nil, err
	}
	if len(workload.Spec.Images) == 0 || workload.Spec.Images[0] == "" {
		return nil, &unsupportedShapeError{"external capacity requires an image reference"}
	}
	imageRef := workload.Spec.Images[0]

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
	imageDigest := ""
	if isDigestPinnedImage(imageRef) {
		imageDigest = imageRef[strings.LastIndex(imageRef, "@sha256:")+1:]
	}
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

// nodeFlavorGPUModel returns NodeFlavor.spec.gpu.product for the workspace flavor.
// An empty string means the flavor has no GPU product (CPU-only capacity).
func (r *SchedulerReconciler) nodeFlavorGPUModel(ctx context.Context, workspace *v1.Workspace) (string, error) {
	if workspace.Spec.NodeFlavor == "" {
		return "", &unsupportedShapeError{"external capacity requires a workspace node flavor"}
	}
	nf := &v1.NodeFlavor{}
	if err := r.Get(ctx, client.ObjectKey{Name: workspace.Spec.NodeFlavor}, nf); err != nil {
		return "", err
	}
	if nf.Spec.Gpu == nil || nf.Spec.Gpu.Product == "" {
		return "", nil
	}
	return string(nf.Spec.Gpu.Product), nil
}

// toResourceVector converts a workload resource request into the contract's units:
// millicores, bytes and whole devices.
//
// gpuModel is NodeFlavor.spec.gpu.product and must match the provider's device inventory.
func toResourceVector(res *v1.WorkloadResource, gpuModel string) (execution.ResourceVector, error) {
	list, err := commonquantity.CvtToResourceList(res.CPU, res.Memory, res.GPU, res.GPUName,
		res.EphemeralStorage, res.RdmaResource, 1)
	if err != nil {
		return execution.ResourceVector{}, err
	}
	cpu := list.Cpu()
	memory := list.Memory()
	scratch := list.StorageEphemeral()
	gpuCount := int32(0)
	gpuResource := ""
	if res.GPUName != "" {
		gpu := list[corev1.ResourceName(res.GPUName)]
		gpuCount = int32(gpu.Value())
		if gpuCount > 0 {
			gpuResource = res.GPUName
		}
	}
	if gpuCount > 0 && gpuModel == "" {
		return execution.ResourceVector{}, &unsupportedShapeError{
			"external capacity requires NodeFlavor gpu.product when requesting GPUs",
		}
	}
	return execution.ResourceVector{
		CPUMillis:    cpu.MilliValue(),
		MemoryBytes:  memory.Value(),
		ScratchBytes: scratch.Value(),
		GPUResource:  gpuResource,
		GPUModel:     gpuModel,
		GPUCount:     gpuCount,
	}, nil
}

// constraintsDigest is the sha256 over the canonical encoding of the supported constraint
// object: keys sorted, no whitespace, UTF-8. The provider recomputes it and compares, so
// the digest cannot stand in for the constraints themselves.
//
// encoding/json cannot be used here: it emits struct fields in declaration order and
// escapes U+2028/U+2029 differently from the contract's reference encoder
// (json.dumps(sort_keys=True, separators=(',', ':'), ensure_ascii=False)).
func constraintsDigest(constraints *execution.PlacementConstraints) (string, error) {
	if constraints == nil {
		constraints = &execution.PlacementConstraints{}
	}
	sum := sha256.Sum256(encodeConstraints(*constraints))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// encodeConstraints matches spur's protocol.encodeConstraints byte for byte.
func encodeConstraints(pc execution.PlacementConstraints) []byte {
	b := []byte(`{"allowed_node_names":[`)
	for i, name := range pc.AllowedNodeNames {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendReferenceString(b, name)
	}
	b = append(b, `],"node_selector":{`...)
	keys := make([]string, 0, len(pc.NodeSelector))
	for k := range pc.NodeSelector {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendReferenceString(b, k)
		b = append(b, ':')
		b = appendReferenceString(b, pc.NodeSelector[k])
	}
	return append(b, '}', '}')
}

const lowerHex = "0123456789abcdef"

// appendReferenceString escapes only what the contract reference encoder escapes.
func appendReferenceString(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			b = append(b, '\\', c)
		case c == '\b':
			b = append(b, '\\', 'b')
		case c == '\f':
			b = append(b, '\\', 'f')
		case c == '\n':
			b = append(b, '\\', 'n')
		case c == '\r':
			b = append(b, '\\', 'r')
		case c == '\t':
			b = append(b, '\\', 't')
		case c < 0x20:
			b = append(b, '\\', 'u', '0', '0', lowerHex[c>>4], lowerHex[c&0xf])
		default:
			b = append(b, c)
		}
	}
	return append(b, '"')
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
			CPUMillis:            p.Resources.CPUMillis,
			MemoryBytes:          p.Resources.MemoryBytes,
			ScratchBytes:         p.Resources.ScratchBytes,
			GPUResource:          p.Resources.GPUResource,
			GPUCount:             p.Resources.GPUCount,
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
		return withProviderMessage(ExternalConstraintReason, err)
	case execution.CodeInvalidRequest:
		return withProviderMessage(ExternalInvalidReason, err)
	case execution.CodeUnauthorized, execution.CodeForbidden:
		return withProviderMessage(ExternalAuthReason, err)
	case execution.CodeRateLimited:
		return ExternalRateLimitedReason
	default:
		return ExternalUnavailableReason
	}
}

// externalMessageLimit bounds the provider message copied into a workload status reason.
const externalMessageLimit = 512

// withProviderMessage appends the provider's refusal message to reason, so the user sees
// why the request was refused rather than only a category.
func withProviderMessage(reason string, err error) string {
	var apiErr *execution.APIError
	if !errors.As(err, &apiErr) || strings.TrimSpace(apiErr.Message) == "" {
		return reason
	}
	message := strings.TrimSpace(apiErr.Message)
	if len(message) > externalMessageLimit {
		message = message[:externalMessageLimit]
	}
	return reason + ": " + message
}
