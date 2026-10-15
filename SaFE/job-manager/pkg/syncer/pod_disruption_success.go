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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
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
// Only a workload of one replica is read, and only the attempt the object
// counted as success: the one pod in the cache that has not failed, or, when
// the cache holds none, the attempt whose record shows it started last and
// ended other than Failed (see lastEndedAttempt). A failed pod was not what
// the object counted as success; while the cache holds more pods the success
// is not tied to one, and waits. A TorchFT or MonarchJob workload is never
// read: it runs several objects, and a mesh pod carries the workload label
// only after its first sync, so the cache cannot be asked for its pods. Nor
// is a RayJob: its success is its submitter's, not its head pod's.
//
// The pod's current object in the informer cache decides, once the API server
// holds no other pod of the dispatch (see cacheHoldsLivePods).
// One that carries the condition was disrupted, whether or not the cache has
// seen it end. One that ended Succeeded without it is a clean success. One
// that has not ended and carries no condition is a cache behind the object's,
// which reports success only after its pod ended: the condition may not have
// reached the cache yet.
//
// Only when the cache holds no pod of the current dispatch is a record read;
// the delete path records the condition there (see removeWorkloadPod) but not
// the phase the pod ended in. The record must have started in the current
// dispatch, as the cache's pods must carry its dispatch count, and every
// record must have ended: a record left from an earlier dispatch, or one whose
// pod's delete has not been recorded yet, does not say how the current pod
// ended.
//
// A cache that cannot answer (no informer yet, not synced, a failed list, only
// pods of an earlier dispatch) is not read as "the pod is gone" on its own: the
// record would then decide for a pod that may still be in the cluster, as
// after a restart, when the object's informer replays its success before the
// pod informer has synced. The API server is asked instead, and the records
// decide only when it holds no pod of the dispatch.
func (r *SyncerReconciler) disruptedSuccess(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) (pod, disruption string, known bool) {
	if commonworkload.IsApplication(adminWorkload) || commonworkload.IsCICD(adminWorkload) ||
		commonworkload.IsTorchFT(adminWorkload) || commonworkload.IsMonarchJob(adminWorkload) ||
		commonworkload.IsRayJob(adminWorkload) || commonworkload.GetTotalReplica(adminWorkload) != 1 {
		return "", "", true
	}
	// A cache that cannot answer is taken as holding no pod of the dispatch:
	// the API server is then asked (see cacheHoldsLivePods), and only when it
	// holds none either are the records read.
	cached, _ := r.cachedWorkloadPods(ctx, clientSets, adminWorkload)
	// The attempt the object counted as success. A pod that ended Failed was
	// not: one replica's earlier attempt, retried by its controller under
	// another name (a Job does), leaves the attempt that succeeded to decide.
	// The cache holds the attempts that still exist; a record of another name
	// is of an attempt that is gone, its delete recorded or not yet.
	candidates := make([]*unstructured.Unstructured, 0, len(cached))
	for _, obj := range cached {
		if phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase"); corev1.PodPhase(phase) != corev1.PodFailed {
			candidates = append(candidates, obj)
		}
	}
	var record *v1.WorkloadPod
	switch len(candidates) {
	case 0:
		var settled bool
		if record, settled = lastEndedAttempt(adminWorkload); !settled {
			return "", "", false
		}
		if record == nil {
			// Two attempts ended in the same second: not told apart.
			return "", "", true
		}
	case 1:
		record = recordOf(adminWorkload, candidates[0].GetName())
	default:
		// More than one pod, as an attempt still terminating beside its
		// replacement: the success is not tied to either until one is gone.
		return "", "", false
	}
	if !r.cacheHoldsLivePods(ctx, clientSets, adminWorkload, candidates) {
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
	if at := strings.Index(p.FailedMessage, disruptionTargetPrefix); at >= 0 {
		return p.PodId, p.FailedMessage[at:], true
	}
	return "", "", true
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

// lastEndedAttempt returns, when no pod is cached, the record of the attempt
// of the current dispatch that started last among those that ended other than
// Failed. settled is false while none has, or while an attempt that started
// no earlier has no recorded end: the delete that ends its record may carry
// the disruption. An attempt that started earlier and has no recorded end is
// an older one whose delete was lost or is still queued; it does not hold the
// verdict. A record left from an earlier dispatch, or of a pod that never
// started, is not an attempt of this dispatch's success. The record is nil
// when two attempts that ended started in the same second.
func lastEndedAttempt(adminWorkload *v1.Workload) (record *v1.WorkloadPod, settled bool) {
	var latest, unended time.Time
	tied, anyUnended := false, false
	for i := range adminWorkload.Status.Pods {
		p := &adminWorkload.Status.Pods[i]
		if p.Phase == corev1.PodFailed || !startedInCurrentDispatch(adminWorkload, p) {
			continue
		}
		started, err := time.ParseInLocation(timeutil.TimeRFC3339Short, p.StartTime, time.Local)
		if err != nil {
			continue
		}
		switch {
		case !v1.IsPodTerminated(p):
			if !anyUnended || started.After(unended) {
				unended = started
			}
			anyUnended = true
		case record == nil || started.After(latest):
			record, latest, tied = p, started, false
		case started.Equal(latest):
			tied = true
		}
	}
	if record == nil || (anyUnended && !unended.Before(latest)) {
		return nil, false
	}
	if tied {
		return nil, true
	}
	return record, true
}

// recordOf returns the workload's record of the named pod, nil when none.
func recordOf(adminWorkload *v1.Workload, name string) *v1.WorkloadPod {
	for i := range adminWorkload.Status.Pods {
		if adminWorkload.Status.Pods[i].PodId == name {
			return &adminWorkload.Status.Pods[i]
		}
	}
	return nil
}

// cacheHoldsLivePods reports whether the pod informer cache holds the pods the
// API server holds for the workload's current dispatch, failed ones aside:
// the one cached candidate, or none when none is cached.
//
// The object's status is read from the API server, and the pods from the
// cache, which can be behind it: an attempt its controller created (under the
// same name or another) may have run and succeeded before the cache saw it,
// and the cache, or a record, still describes the earlier attempt. A pod the
// API server holds that the cache does not leaves the verdict to wait for the
// cache; so does a list that fails. A pod the API server no longer holds is
// read from the cache or its record, as the delete path left it.
func (r *SyncerReconciler) cacheHoldsLivePods(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload, candidates []*unstructured.Unstructured) bool {
	namespace := adminWorkload.Spec.Workspace
	if len(candidates) > 0 {
		namespace = candidates[0].GetNamespace()
	}
	live, ok := liveWorkloadPods(ctx, clientSets, namespace, adminWorkload.Name)
	if !ok {
		return false
	}
	dispatchCount := v1.GetWorkloadDispatchCnt(adminWorkload)
	for i := range live {
		pod := &live[i]
		if pod.Status.Phase == corev1.PodFailed ||
			isStaleWorkloadGeneration(adminWorkload, v1.GetLabel(pod, v1.WorkloadUidLabel)) ||
			(dispatchCount > 0 && v1.GetWorkloadDispatchCnt(pod) != dispatchCount) {
			continue
		}
		if len(candidates) == 0 || pod.UID != candidates[0].GetUID() {
			return false
		}
	}
	return true
}

// liveWorkloadPods lists the workload's pods in the namespace as the API server
// holds them, and reports whether they could be listed.
var liveWorkloadPods = func(ctx context.Context, clientSets *ClusterClientSets,
	namespace, workloadId string) ([]corev1.Pod, bool) {
	if clientSets == nil || clientSets.ClientFactory() == nil || clientSets.ClientFactory().ClientSet() == nil {
		return nil, false
	}
	list, err := clientSets.ClientFactory().ClientSet().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{v1.WorkloadIdLabel: workloadId}).String(),
	})
	if err != nil {
		// Listed again every retry while a success waits: not logged as an error.
		klog.V(2).InfoS("failed to list pods", "namespace", namespace, "workload", workloadId, "err", err)
		return nil, false
	}
	return list.Items, true
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
// buildWorkloadPodInfo), so it is parsed as local time. It holds whole
// seconds, and a pod that started in the second it was dispatched in has
// started after the dispatch.
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
