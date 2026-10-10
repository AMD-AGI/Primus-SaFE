/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// Waiting reasons surfaced for workloads in an external workspace.
const (
	ExternalCapacityReason    = "In queue - waiting for external capacity"
	ExternalImageReason       = "In queue - external image is being prepared"
	ExternalProfileReason     = "In queue - execution profile is not validated"
	ExternalConstraintReason  = "Rejected - constraints cannot be satisfied by external capacity"
	ExternalUnsupportedReason = "Rejected - workload shape is not supported by external capacity"
	ExternalInvalidReason     = "Rejected - request refused by external capacity"
	// ExternalBudgetMissingReason is terminal when the request names a resource the
	// external-budget quota does not declare (for example rdma/hca or ephemeral-storage).
	ExternalBudgetMissingReason = "Rejected - external budget does not include required resource"
	ExternalUnavailableReason   = "In queue - external capacity service is unavailable"
	ExternalRateLimitedReason = "In queue - external capacity controller rate limited"
	ExternalAuthReason        = "In queue - not authorized by external capacity"
)

// externalReleaseRetry paces a retry while kube-scheduler ProvisioningRequest
// objects are still being deleted.
const externalReleaseRetry = 15 * time.Second

// externalExchangeRetry re-stages the workspace schedule after a failed
// kube-scheduler admit that carried no Retry-After (for example a stale status patch).
const externalExchangeRetry = 10 * time.Second

// externalWaitRetry re-stages the workspace schedule while a workload waits
// on quota or ProvisioningRequest status.
const externalWaitRetry = 30 * time.Second

// isTerminalExternalReason reports whether an admission reason rules the workload
// out for good. Terminal reasons may carry a detail after the prefix.
func isTerminalExternalReason(reason string) bool {
	for _, prefix := range []string{
		ExternalUnsupportedReason, ExternalConstraintReason, ExternalInvalidReason,
		ExternalBudgetMissingReason, ExternalPRFailedReason, ExternalImageResolveReason,
	} {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}

// externalStatePatchAttempts covers create-time races where another writer bumps
// resourceVersion between the cached read and the JSON-patch test.
const externalStatePatchAttempts = 5

// isExternalReclaiming reports whether a finished workload still holds a
// ProvisioningRequest booking that must count against workspace quota until deleted.
func isExternalReclaiming(workload *v1.Workload) bool {
	if workload == nil || workload.Status.ExternalExecution == nil {
		return false
	}
	state := workload.Status.ExternalExecution
	return state.PlacementMode == v1.ExternalPlacementKubeScheduler &&
		state.ProvisioningRequest != ""
}

// releaseBeforeDelete used to wait for an HTTP claim release. Kube-scheduler
// objects are deleted separately; this always allows the finalizer to proceed.
func (r *SchedulerReconciler) releaseBeforeDelete(ctx context.Context, workload *v1.Workload) (bool, error) {
	return false, nil
}

// reconcileExternalRelease deletes ProvisioningRequest/PodTemplate once a workload
// has ended or is being deleted so bookings do not linger after the job finishes.
func (r *SchedulerReconciler) reconcileExternalRelease(ctx context.Context,
	workload *v1.Workload) (bool, error) {
	if workload == nil || workload.Status.ExternalExecution == nil {
		return false, nil
	}
	if !workload.IsEnd() && workload.DeletionTimestamp.IsZero() {
		return false, nil
	}
	state := workload.Status.ExternalExecution
	if state.PlacementMode != v1.ExternalPlacementKubeScheduler ||
		(state.ProvisioningRequest == "" && state.DispatchGeneration == 0) {
		return false, nil
	}
	if err := r.deleteExternalProvisioningObjects(ctx, workload); err != nil {
		// Return (true, nil) so the caller requeues after externalReleaseRetry instead
		// of treating the error as an immediate reconcile failure that skips pacing.
		klog.ErrorS(err, "failed to delete external provisioning objects",
			"workload", workload.Name)
		return true, nil
	}
	updated := state.DeepCopy()
	updated.ProvisioningRequest = ""
	updated.ProvisioningAttempt = 0
	updated.ProvisioningCondition = ""
	// Clear DispatchGeneration so an ended workload becomes a no-op on the next pass.
	updated.DispatchGeneration = 0
	if err := r.patchExternalState(ctx, workload, updated); err != nil {
		klog.ErrorS(err, "failed to clear external execution after release",
			"workload", workload.Name)
		return true, nil
	}
	return false, nil
}

// patchExternalState writes the bookkeeping back to the workload status.
//
// A JSON patch, not a merge patch. Every field of the stored object is omitempty, so under
// merge semantics a field returning to its zero value simply vanishes from the payload and
// the old value survives.
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
