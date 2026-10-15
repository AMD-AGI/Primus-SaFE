/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// disruptedSuccess returns the DisruptionTarget of a single-pod workload whose
// object reports success because its pod, taken away by a disruption, ended
// Succeeded. It returns "" otherwise.
//
// A pod whose containers exit 0 on the SIGTERM of an eviction, a preemption or
// a node drain ends Succeeded, and an object that mirrors its pod's phase (a
// Sandbox, for one) then reports success. Kubernetes records the disruption on
// the pod itself as the DisruptionTarget condition, set before the pod is
// terminated.
//
// Only a workload of one replica whose records and cache name one pod is read:
// with more pods the object's success cannot be tied to the disrupted one. A
// TorchFT or MonarchJob workload is never read: it runs several objects, and a
// mesh pod carries the workload label only after its first sync, so the cache
// cannot be asked for its pods.
//
// The pod's current object in the informer cache decides, and must have ended
// Succeeded with a True DisruptionTarget: a pod of the same name created later
// that ended cleanly is a clean success, and a disrupted pod that ended Failed
// was not what the object counted as success. Only when the cache holds no pod
// of the current dispatch is its record read; the delete path records the
// condition there (see removeWorkloadPod) but not the phase the pod ended in.
//
// A cache that cannot answer (no informer yet, not synced, a failed list) is
// never read as "the pod is gone": the record would then decide for a pod that
// may still be in the cluster, as after a restart, when the object's informer
// replays its success before the pod informer has synced. The success then
// stands as reported.
func (r *SyncerReconciler) disruptedSuccess(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) string {
	if commonworkload.IsApplication(adminWorkload) || commonworkload.IsCICD(adminWorkload) ||
		commonworkload.IsTorchFT(adminWorkload) || commonworkload.IsMonarchJob(adminWorkload) ||
		commonworkload.GetTotalReplica(adminWorkload) != 1 {
		return ""
	}
	cached, ok := r.cachedWorkloadPods(ctx, clientSets, adminWorkload)
	if !ok {
		klog.Infof("workload %s: k8s object reports success, and the pod cache cannot tell "+
			"whether its pod was disrupted; the success stands", adminWorkload.Name)
		return ""
	}
	names := map[string]struct{}{}
	for i := range adminWorkload.Status.Pods {
		names[adminWorkload.Status.Pods[i].PodId] = struct{}{}
	}
	for _, obj := range cached {
		names[obj.GetName()] = struct{}{}
	}
	if len(names) != 1 {
		return ""
	}
	if len(cached) > 0 {
		phase, _, _ := unstructured.NestedString(cached[0].Object, "status", "phase")
		if corev1.PodPhase(phase) != corev1.PodSucceeded {
			return ""
		}
		return disruptionTargetOf(cached[0])
	}
	if len(adminWorkload.Status.Pods) == 0 {
		return ""
	}
	p := &adminWorkload.Status.Pods[0]
	// A record that ended Failed is not what the object counted as success; one
	// the delete path stopped did not see how its pod ended, and is the candidate
	// with one that ended Succeeded.
	if p.Phase != corev1.PodSucceeded && p.Phase != corev1.PodPhase(v1.WorkloadStopped) {
		return ""
	}
	if at := strings.Index(p.FailedMessage, disruptionTargetPrefix); at >= 0 {
		return p.FailedMessage[at:]
	}
	return ""
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

// joinMessage joins two status messages with "; ", dropping an empty one.
func joinMessage(a, b string) string {
	switch {
	case strings.TrimSpace(a) == "":
		return b
	case strings.TrimSpace(b) == "":
		return a
	}
	return a + "; " + b
}
