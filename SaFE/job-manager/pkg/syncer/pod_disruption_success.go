/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"errors"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// disruptedSuccess returns the pod and the DisruptionTarget of a single-pod
// workload whose object reports success because its pod, taken away by a
// disruption, ended Succeeded; "" for both when the success is the pod's own.
// known is false when the pod cache and records cannot tell yet, and the
// caller must not let the success settle: an ended workload is not revisited.
//
// A pod whose containers exit 0 on the SIGTERM of an eviction, a preemption or
// a node drain ends Succeeded, and an object that mirrors its pod's phase (a
// Sandbox, for one) then reports success. Kubernetes records the disruption on
// the pod itself as the DisruptionTarget condition, set before the pod is
// terminated; one set after the pod had already ended is not read (see
// podDisruption).
//
// Only a workload of one replica whose records and cache name one pod is read:
// with more pods the object's success cannot be tied to the disrupted one. A
// TorchFT or MonarchJob workload is never read: it runs several objects, and a
// mesh pod carries the workload label only after its first sync, so the cache
// cannot be asked for its pods.
//
// The pod's current object in the informer cache decides. One that ended
// Failed was not what the object counted as success. One that carries the
// condition was disrupted, whether or not the cache has seen it end. One that
// ended Succeeded without it is a clean success, a later pod of the same name
// included. One that has not ended and carries no condition is a cache behind
// the object's, which reports success only after its pod ended: the condition
// may not have reached the cache yet.
//
// The cached pod is taken for the one the object counted. Were the object's
// success that of a later pod of the same name, the cache would hold that pod:
// the informer keys pods by name, and pod records come from the same informer.
// Only a pod informer that missed the later pod's whole life would show the
// earlier one, and nothing this process holds knows that pod.
//
// Only when the cache holds no pod of the current dispatch is its record read;
// the delete path records the condition there (see removeWorkloadPod) but not
// the phase the pod ended in. The record must have started in the current
// dispatch, as the cache's pods must carry its dispatch count, and must have
// ended: a record left from an earlier dispatch, or one whose pod's delete has
// not been recorded yet, does not say how the current pod ended.
//
// A cache that cannot answer (no informer yet, not synced, a failed list, only
// pods of an earlier dispatch) is never read as "the pod is gone": the record
// would then decide for a pod that may still be in the cluster, as after a
// restart, when the object's informer replays its success before the pod
// informer has synced.
func (r *SyncerReconciler) disruptedSuccess(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) (pod, disruption string, known bool) {
	if commonworkload.IsApplication(adminWorkload) || commonworkload.IsCICD(adminWorkload) ||
		commonworkload.IsTorchFT(adminWorkload) || commonworkload.IsMonarchJob(adminWorkload) ||
		commonworkload.GetTotalReplica(adminWorkload) != 1 {
		return "", "", true
	}
	cached, ok := r.cachedWorkloadPods(ctx, clientSets, adminWorkload)
	if !ok {
		return "", "", false
	}
	names := map[string]struct{}{}
	for i := range adminWorkload.Status.Pods {
		names[adminWorkload.Status.Pods[i].PodId] = struct{}{}
	}
	for _, obj := range cached {
		names[obj.GetName()] = struct{}{}
	}
	switch {
	case len(names) > 1:
		return "", "", true
	case len(names) == 0:
		// Nothing of the pod has been recorded or cached yet.
		return "", "", false
	}
	if len(cached) > 0 {
		phase, _, _ := unstructured.NestedString(cached[0].Object, "status", "phase")
		if corev1.PodPhase(phase) == corev1.PodFailed {
			return "", "", true
		}
		if disruption = disruptionTargetOf(cached[0]); disruption != "" {
			return cached[0].GetName(), disruption, true
		}
		return "", "", corev1.PodPhase(phase) == corev1.PodSucceeded
	}
	p := &adminWorkload.Status.Pods[0]
	if !startedInCurrentDispatch(adminWorkload, p) {
		return "", "", false
	}
	switch p.Phase {
	case corev1.PodFailed:
		// Not what the object counted as success.
		return "", "", true
	case corev1.PodSucceeded, corev1.PodPhase(v1.WorkloadStopped):
		// The delete path stops a record without seeing how its pod ended.
	default:
		return "", "", false
	}
	// Only the pod's own condition: one carried from an earlier pod of the same
	// name says nothing about how this pod ended.
	own, _ := podDisruptions(p.FailedMessage)
	if own == "" {
		return "", "", true
	}
	return p.PodId, own, true
}

// While the pod cache and records cannot tell whether a disruption caused an
// object's success (see disruptedSuccess), the success is read again every
// successVerdictRetry, for at most maxSuccessVerdictWait of the workload's
// dispatch; after that it stands as reported. A pod informer behind the
// object's is seconds behind, and one syncing after a restart, a minute or so.
const (
	successVerdictRetry   = 3 * time.Second
	maxSuccessVerdictWait = 2 * time.Minute
)

// errSuccessVerdictPending is returned by updateAdminWorkloadByJob when it left
// an object's success unapplied, to be read again after successVerdictRetry.
var errSuccessVerdictPending = errors.New("whether a disruption caused the success is not known yet")

// successVerdictNow is the clock of awaitSuccessVerdict.
var successVerdictNow = time.Now

// successVerdictWait is when this process began waiting to tell whether a
// disruption caused the success of a workload's dispatch. The workload's uid
// tells a workload recreated under the same name from the one that waited.
type successVerdictWait struct {
	uid      types.UID
	dispatch int
	since    time.Time
}

// awaitSuccessVerdict reports whether the success of the workload's dispatch
// is still to wait for disruptedSuccess to tell, starting the wait on the
// first call. It stops waiting after maxSuccessVerdictWait. The wait is
// dropped once a job event of the workload no longer waits, and when the
// workload has ended (see reconcileVanishedPods).
func (r *SyncerReconciler) awaitSuccessVerdict(adminWorkload *v1.Workload, dispatch int) bool {
	now := successVerdictNow()
	started := successVerdictWait{uid: adminWorkload.UID, dispatch: dispatch, since: now}
	v, loaded := r.successVerdictWaits.LoadOrStore(adminWorkload.Name, started)
	if !loaded {
		return true
	}
	if w := v.(successVerdictWait); w.uid != started.uid || w.dispatch != dispatch {
		r.successVerdictWaits.Store(adminWorkload.Name, started)
		return true
	} else if now.Sub(w.since) < maxSuccessVerdictWait {
		return true
	}
	r.successVerdictWaits.Delete(adminWorkload.Name)
	return false
}

// startedInCurrentDispatch reports whether the pod record describes a pod that
// started after the workload's current dispatch. A record with no start time,
// or a workload with no dispatch time, does not.
//
// The dispatch time is written in UTC. A record's start time carries no zone,
// and is the pod's start time as this process's local clock reads it (see
// podStartTime), so it is parsed as local time. It holds whole seconds, and a
// pod that started in the second it was dispatched in has started after the
// dispatch.
func startedInCurrentDispatch(adminWorkload *v1.Workload, p *v1.WorkloadPod) bool {
	dispatchedAt, err := timeutil.CvtStrToRFC3339Milli(
		v1.GetAnnotation(adminWorkload, v1.WorkloadDispatchedAnnotation))
	if err != nil {
		return false
	}
	started, err := time.ParseInLocation(timeutil.TimeRFC3339Short, p.StartTime, time.Local)
	if err != nil {
		return false
	}
	return !started.Before(dispatchedAt.Truncate(time.Second))
}

// disruptedSuccessMessage is the failure message of an object that reported
// success, reported, because its pod was taken away by a disruption: the
// disruption first, then what the object reported.
func disruptedSuccessMessage(pod, disruption, reported string) string {
	message := "pod " + pod + " was taken away by a disruption, " + disruption
	if strings.TrimSpace(reported) != "" {
		message += "; the object reported success: " + reported
	}
	return message
}

// cachedWorkloadPods returns the workload's pods of its current dispatch from the
// pod informer cache, and whether the cache could answer. Pods of an earlier
// workload generation or an earlier dispatch are not the workload's current pod;
// when the cache holds only such pods it cannot tell whether the current pod has
// left it, and the pod records may still describe the earlier pods.
func (r *SyncerReconciler) cachedWorkloadPods(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) ([]*unstructured.Unstructured, bool) {
	objs, _, ok := listCachedWorkloadPods(ctx, clientSets, adminWorkload)
	if !ok {
		return nil, false
	}
	dispatchCount := v1.GetWorkloadDispatchCnt(adminWorkload)
	pods := make([]*unstructured.Unstructured, 0, len(objs))
	skipped := false
	for _, obj := range objs {
		pod, ok := obj.(*unstructured.Unstructured)
		if !ok {
			klog.Errorf("unexpected object type %T in the pod informer cache of workload %s",
				obj, adminWorkload.Name)
			return nil, false
		}
		if isStaleWorkloadGeneration(adminWorkload, v1.GetLabel(pod, v1.WorkloadUidLabel)) ||
			(dispatchCount > 0 && v1.GetWorkloadDispatchCnt(pod) != dispatchCount) {
			skipped = true
			continue
		}
		pods = append(pods, pod)
	}
	if skipped && len(pods) == 0 {
		return nil, false
	}
	return pods, true
}
