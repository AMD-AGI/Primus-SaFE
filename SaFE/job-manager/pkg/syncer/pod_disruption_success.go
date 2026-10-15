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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
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
// Only a workload of one replica whose records and cache name one pod that has
// not failed is read: a failed pod was not what the object counted as success,
// and with more pods the success cannot be tied to the disrupted one. A
// TorchFT or MonarchJob workload is never read: it runs several objects, and a
// mesh pod carries the workload label only after its first sync, so the cache
// cannot be asked for its pods.
//
// The pod's current object in the informer cache decides, once it is the pod
// the API server holds, or the API server holds none (see cacheHoldsLivePod).
// One that carries the condition was disrupted, whether or not the cache has
// seen it end. One that ended Succeeded without it is a clean success. One
// that has not ended and carries no condition is a cache behind the object's,
// which reports success only after its pod ended: the condition may not have
// reached the cache yet.
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
	// A pod that ended Failed was not what the object counted as success: one
	// replica's earlier attempt, retried by its controller under another name,
	// leaves the attempt that succeeded to decide.
	names := map[string]struct{}{}
	var record *v1.WorkloadPod
	for i := range adminWorkload.Status.Pods {
		if p := &adminWorkload.Status.Pods[i]; p.Phase != corev1.PodFailed {
			names[p.PodId] = struct{}{}
			record = p
		}
	}
	candidates := make([]*unstructured.Unstructured, 0, len(cached))
	for _, obj := range cached {
		if phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase"); corev1.PodPhase(phase) == corev1.PodFailed {
			continue
		}
		names[obj.GetName()] = struct{}{}
		candidates = append(candidates, obj)
	}
	switch {
	case len(names) > 1:
		return "", "", true
	case len(names) == 0:
		// The attempt the object counted as success has not been recorded or
		// cached yet: only failed ones, or none, have.
		return "", "", false
	}
	if !r.cacheHoldsLivePod(ctx, clientSets, adminWorkload, names, candidates) {
		return "", "", false
	}
	if len(candidates) > 0 {
		phase, _, _ := unstructured.NestedString(candidates[0].Object, "status", "phase")
		disruption = disruptionTargetOf(candidates[0])
		if disruption != "" && !seenSucceededClean(record, convertPodFromUnstructured(candidates[0])) {
			return candidates[0].GetName(), disruption, true
		}
		return "", "", corev1.PodPhase(phase) == corev1.PodSucceeded
	}
	if record == nil || record.PodId != onlyName(names) {
		// The pod is cached under a name no record holds; handled above.
		return "", "", false
	}
	p := record
	if !startedInCurrentDispatch(adminWorkload, p) {
		return "", "", false
	}
	switch p.Phase {
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

// seenSucceededClean reports whether the record shows the pod ended Succeeded
// with no DisruptionTarget: the pod was seen to succeed before any such
// condition. A condition once set stays on the pod, and one that ends a pod is
// set before its containers are signalled, so a condition seen only later was
// set after the pod had ended (as the taint manager or a drain does to a
// completed pod), whatever its time says to the second. The record must
// describe the pod: the same name and start time.
func seenSucceededClean(record *v1.WorkloadPod, pod *corev1.Pod) bool {
	return record != nil && pod != nil && record.PodId == pod.Name &&
		record.Phase == corev1.PodSucceeded && !strings.Contains(record.FailedMessage, disruptionTargetPrefix) &&
		record.StartTime != "" && record.StartTime == podStartTime(pod)
}

// onlyName returns the one name names holds.
func onlyName(names map[string]struct{}) string {
	for name := range names {
		return name
	}
	return ""
}

// cacheHoldsLivePod reports whether the pod informer cache holds the pod the
// API server holds under the workload's one pod name, or neither holds one.
//
// The object's status is read from the API server, and the pod's from the
// cache, which can be behind it: a controller that recreated the pod under the
// same name may have had the replacement run and succeed before the cache saw
// it, and the cache, or the record, still describes the earlier pod. A pod the
// API server holds that the cache does not, or holds as another pod, leaves the
// verdict to wait for the cache. One the API server no longer holds is read
// from the cache or its record, as the delete path left it.
func (r *SyncerReconciler) cacheHoldsLivePod(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload, names map[string]struct{}, cached []*unstructured.Unstructured) bool {
	namespace := adminWorkload.Spec.Workspace
	if len(cached) > 0 {
		namespace = cached[0].GetNamespace()
	}
	uid, ok := livePodUID(ctx, clientSets, namespace, onlyName(names))
	switch {
	case !ok:
		return false
	case uid == "":
		return true
	default:
		return len(cached) > 0 && cached[0].GetUID() == uid
	}
}

// livePodUID returns the uid of the named pod as the API server holds it, ""
// when it holds none, and whether it could be read.
var livePodUID = func(ctx context.Context, clientSets *ClusterClientSets, namespace, name string) (types.UID, bool) {
	if clientSets == nil || clientSets.ClientFactory() == nil || clientSets.ClientFactory().ClientSet() == nil {
		return "", false
	}
	pod, err := clientSets.ClientFactory().ClientSet().CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		return "", true
	case err != nil:
		klog.ErrorS(err, "failed to read pod", "namespace", namespace, "pod", name)
		return "", false
	}
	return pod.UID, true
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
// disruption caused the success of a workload's dispatch, and the success the
// object reported. The workload's uid tells a workload recreated under the same
// name from the one that waited, and the object's uid an object recreated
// under the same name from the one that reported the success.
type successVerdictWait struct {
	uid      types.UID
	dispatch int
	since    time.Time
	object   types.UID
	reported jobutils.K8sObjectStatus
}

// awaitSuccessVerdict reports whether the success the object reported for the
// workload's dispatch is still to wait for disruptedSuccess to tell, starting
// the wait on the first call. It stops waiting after maxSuccessVerdictWait.
// The wait is dropped once a job event of the workload no longer waits, and
// when the workload has ended (see reconcileVanishedPods).
func (r *SyncerReconciler) awaitSuccessVerdict(adminWorkload *v1.Workload, dispatch int,
	object types.UID, reported *jobutils.K8sObjectStatus) bool {
	now := successVerdictNow()
	started := successVerdictWait{uid: adminWorkload.UID, dispatch: dispatch, since: now,
		object: object, reported: *reported}
	v, loaded := r.successVerdictWaits.LoadOrStore(adminWorkload.Name, started)
	if !loaded {
		return true
	}
	if w := v.(successVerdictWait); w.uid != started.uid || w.dispatch != dispatch || w.object != object {
		r.successVerdictWaits.Store(adminWorkload.Name, started)
		return true
	} else if now.Sub(w.since) < maxSuccessVerdictWait {
		return true
	}
	r.successVerdictWaits.Delete(adminWorkload.Name)
	return false
}

// waitingSuccess returns the success the object reported for the workload's
// dispatch while this process waits to tell whether a disruption caused it.
// Only the object that reported it, by uid, has it.
//
// An object deleted during the wait (by its owner, or a user who saw it
// succeed) has still ended in that success: its deletion must not stop the
// workload, nor have it scheduled again as an object deleted while running.
func (r *SyncerReconciler) waitingSuccess(adminWorkload *v1.Workload, dispatch int,
	object types.UID) (*jobutils.K8sObjectStatus, bool) {
	v, ok := r.successVerdictWaits.Load(adminWorkload.Name)
	if !ok {
		return nil, false
	}
	w := v.(successVerdictWait)
	if w.uid != adminWorkload.UID || w.dispatch != dispatch || w.object != object {
		return nil, false
	}
	reported := w.reported
	return &reported, true
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
