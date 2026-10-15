/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// disruptedSuccess returns the pod and the DisruptionTarget of a single-pod
// workload whose object reports success because its pod, taken away by a
// disruption, ended Succeeded. It returns "" for both otherwise.
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
// Failed was not what the object counted as success. One that has not ended
// in the cache is a cache behind the object's: the object reports success only
// after its pod ended, so the pod's DisruptionTarget, set while it ran, is the
// disruption that ended it. A pod of the same name created later that ended
// cleanly carries no condition and is a clean success. Only when the cache
// holds no pod of the current dispatch is its record read; the delete path
// records the condition there (see removeWorkloadPod) but not the phase the
// pod ended in. The record must have started in the current dispatch, as the
// cache's pods must carry its dispatch count: a record left from an earlier
// dispatch does not say how the current pod ended.
//
// A cache that cannot answer (no informer yet, not synced, a failed list) is
// never read as "the pod is gone": the record would then decide for a pod that
// may still be in the cluster, as after a restart, when the object's informer
// replays its success before the pod informer has synced. The success then
// stands as reported.
func (r *SyncerReconciler) disruptedSuccess(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) (pod, disruption string) {
	if commonworkload.IsApplication(adminWorkload) || commonworkload.IsCICD(adminWorkload) ||
		commonworkload.IsTorchFT(adminWorkload) || commonworkload.IsMonarchJob(adminWorkload) ||
		commonworkload.GetTotalReplica(adminWorkload) != 1 {
		return "", ""
	}
	cached, ok := r.cachedWorkloadPods(ctx, clientSets, adminWorkload)
	if !ok {
		klog.Infof("workload %s: k8s object reports success, and the pod cache cannot tell "+
			"whether its pod was disrupted; the success stands", adminWorkload.Name)
		return "", ""
	}
	names := map[string]struct{}{}
	for i := range adminWorkload.Status.Pods {
		names[adminWorkload.Status.Pods[i].PodId] = struct{}{}
	}
	for _, obj := range cached {
		names[obj.GetName()] = struct{}{}
	}
	if len(names) != 1 {
		return "", ""
	}
	if len(cached) > 0 {
		phase, _, _ := unstructured.NestedString(cached[0].Object, "status", "phase")
		if corev1.PodPhase(phase) == corev1.PodFailed {
			return "", ""
		}
		if disruption = disruptionTargetOf(cached[0]); disruption == "" {
			return "", ""
		}
		return cached[0].GetName(), disruption
	}
	if len(adminWorkload.Status.Pods) == 0 {
		return "", ""
	}
	p := &adminWorkload.Status.Pods[0]
	// A record that ended Failed is not what the object counted as success; one
	// the delete path stopped did not see how its pod ended, and is the candidate
	// with one that ended Succeeded.
	if p.Phase != corev1.PodSucceeded && p.Phase != corev1.PodPhase(v1.WorkloadStopped) {
		return "", ""
	}
	if !startedInCurrentDispatch(adminWorkload, p) {
		return "", ""
	}
	// Only the pod's own condition: one carried from an earlier pod of the same
	// name says nothing about how this pod ended.
	if own, _ := podDisruptions(p.FailedMessage); own != "" {
		return p.PodId, own
	}
	return "", ""
}

// startedInCurrentDispatch reports whether the pod record describes a pod that
// started after the workload's current dispatch. A record with no start time,
// or a workload with no dispatch time, does not.
//
// The dispatch time is written in UTC. A record's start time carries no zone,
// and is the pod's start time as this process's local clock reads it (see
// podStartTime), so it is parsed as local time.
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
	return !started.Before(dispatchedAt)
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
