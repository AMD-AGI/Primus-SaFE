/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

// IsUnrecoverableError checks if an error is non-retryable based on error type.
func IsUnrecoverableError(err error) bool {
	if err == nil {
		return false
	}
	if commonerrors.IsBadRequest(err) || commonerrors.IsInternal(err) || commonerrors.IsNotFound(err) {
		return true
	}
	// K8s API errors that are unrecoverable
	if apierrors.IsNotFound(err) || apierrors.IsInvalid(err) || apierrors.IsForbidden(err) {
		return true
	}
	// "etcdserver: request is too large" (HTTP 413): the object has outgrown
	// etcd's max request size, so retrying the same oversized write can never
	// succeed and only hot-loops. Treat it as unrecoverable.
	if apierrors.IsRequestEntityTooLargeError(err) || strings.Contains(err.Error(), "request is too large") {
		return true
	}
	return false
}

// FindCondition finds the condition of the workload and checks if there is one with the same type and reason.
func FindCondition(workload *v1.Workload, condition *metav1.Condition) *metav1.Condition {
	for i, currentCondition := range workload.Status.Conditions {
		if currentCondition.Type == condition.Type && currentCondition.Reason == condition.Reason {
			return &workload.Status.Conditions[i]
		}
	}
	return nil
}

// NewCondition creates a new condition with the specified type, message, and reason.
func NewCondition(conditionType, message, reason string) *metav1.Condition {
	return &metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionTrue,
		Message:            message,
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now().UTC()),
	}
}

// SetWorkloadFailed sets the workload to failed state and updates its status.
// It adds a failure condition and sets the end time if not already set.
func SetWorkloadFailed(ctx context.Context, cli client.Client, workload *v1.Workload, message string) error {
	workload.Status.Phase = v1.WorkloadFailed
	if workload.Status.EndTime == nil {
		workload.Status.EndTime = &metav1.Time{Time: time.Now().UTC()}
	}

	dispatchCount := v1.GetWorkloadDispatchCnt(workload)
	if dispatchCount == 0 {
		// Default to 1 for initial failure
		dispatchCount = 1
	}
	condition := NewCondition(string(v1.AdminFailed), message, commonworkload.GenerateDispatchReason(dispatchCount))
	workload.Status.Conditions = append(workload.Status.Conditions, *condition)
	commonworkload.StripOffloadedStatus(workload)
	if err := UpdateWorkloadStatusWithRetry(ctx, cli, workload); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", workload.Name)
		return err
	}
	return nil
}

// UpdateWorkloadStatusWithRetry writes workload.Status to etcd, retrying on
// optimistic-lock conflicts by re-reading the latest object and re-applying the
// computed status. Without a retry, unrelated concurrent writes to .metadata/
// .spec bump the resourceVersion and turn every status update into a conflict,
// which on hot objects (e.g. CICD runner sets) livelocks the single-worker
// syncer queue.
//
// Workload .status is written mostly by the syncer, but a few other controllers
// (e.g. failover's AdminFailover condition) also append conditions. Re-applying
// the computed status wholesale would drop such concurrently-added conditions,
// so the freshly-read conditions are merged back in by (Type, Reason). Other
// status fields (phase, pods, nodes, ...) are owned by the syncer and are
// overwritten with the computed value.
func UpdateWorkloadStatusWithRetry(ctx context.Context, cli client.Client, workload *v1.Workload) error {
	key := client.ObjectKeyFromObject(workload)
	status := workload.Status
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &v1.Workload{}
		if err := cli.Get(ctx, key, latest); err != nil {
			return err
		}
		// Snapshot conditions already in etcd (may include ones added
		// concurrently by other controllers) before overwriting status.
		concurrentConditions := latest.Status.Conditions
		status.DeepCopyInto(&latest.Status)
		latest.Status.Conditions = mergeConditions(latest.Status.Conditions, concurrentConditions)
		if err := cli.Status().Update(ctx, latest); err != nil {
			return err
		}
		workload.ResourceVersion = latest.ResourceVersion
		return nil
	})
}

// mergeConditions returns base plus any conditions from extra not already
// present by (Type, Reason). It preserves conditions added concurrently by other
// controllers when a status write overwrites the condition list.
func mergeConditions(base, extra []metav1.Condition) []metav1.Condition {
	for i := range extra {
		found := false
		for j := range base {
			if base[j].Type == extra[i].Type && base[j].Reason == extra[i].Reason {
				found = true
				break
			}
		}
		if !found {
			base = append(base, extra[i])
		}
	}
	return base
}

// StopReason describes why a workload was forcibly transitioned to the
// Stopped phase. Surfaced in the AdminStopped condition message and the
// klog line so operators can grep the reason without parsing free-form text.
type StopReason string

const (
	StopReasonTimeout       StopReason = "timeout"
	StopReasonOwnerCascade  StopReason = "owner_cascade"
	StopReasonManual        StopReason = "manual"
	StopReasonUnspecified   StopReason = "unspecified"
)

// MarkWorkloadStopped transitions a workload to the Stopped phase and
// appends an AdminStopped condition. Idempotent: a no-op if the workload
// is already Stopped.
//
// Use cases:
//   - Timeout (ttl_controller.IsTimeout) — reason=StopReasonTimeout.
//   - Owner cascade (scheduler.cascadeStopChildren) — reason=StopReasonOwnerCascade.
//   - Future manual / API-driven stops — reason=StopReasonManual.
//
// Reason is also embedded in the log line so a single grep across cluster
// logs can answer "why did workload X stop?" without crossreferencing
// callsites.
func MarkWorkloadStopped(
	ctx context.Context, cli client.Client, workload *v1.Workload,
	reason StopReason, message string,
) error {
	if workload.Status.Phase == v1.WorkloadStopped {
		return nil
	}

	statusPatch := map[string]any{}
	statusPatch["phase"] = v1.WorkloadStopped
	if workload.Status.EndTime == nil {
		statusPatch["endTime"] = &metav1.Time{Time: time.Now().UTC()}
	}
	cond := metav1.Condition{
		Type:    string(v1.AdminStopped),
		Status:  metav1.ConditionTrue,
		Reason:  commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload)),
		Message: message,
	}
	if meta.SetStatusCondition(&workload.Status.Conditions, cond) {
		statusPatch["conditions"] = workload.Status.Conditions
	}

	patchObj := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": workload.ResourceVersion,
		},
		"status": statusPatch,
	}
	p := jsonutils.MarshalSilently(patchObj)
	if err := cli.Status().Patch(ctx, workload, client.RawPatch(apitypes.MergePatchType, p)); err != nil {
		klog.ErrorS(err, "failed to patch workload phase", "workload", workload.Name)
		return err
	}
	klog.Infof("workload %s stopped: reason=%s msg=%q", workload.Name, reason, message)
	return nil
}

// SetWorkloadTimeout is the legacy entrypoint for the timeout path. Kept for
// callers outside this package; new code should prefer MarkWorkloadStopped
// with an explicit StopReason.
//
// Deprecated: use MarkWorkloadStopped(ctx, cli, w, StopReasonTimeout, msg).
func SetWorkloadTimeout(ctx context.Context, cli client.Client, workload *v1.Workload, message string) error {
	return MarkWorkloadStopped(ctx, cli, workload, StopReasonTimeout, message)
}
