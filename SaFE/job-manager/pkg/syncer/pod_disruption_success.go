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
	"k8s.io/apimachinery/pkg/labels"
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
// with more pods the object's success cannot be tied to the disrupted one. The
// pod's current object in the informer cache decides, and must have ended
// Succeeded with a True DisruptionTarget: a pod of the same name created later
// that ended cleanly is a clean success, and a disrupted pod that ended Failed
// was not what the object counted as success. Only when the cache no longer
// holds the pod is its record read; the delete path records the condition there
// (see removeWorkloadPod) but not the phase the pod ended in.
func (r *SyncerReconciler) disruptedSuccess(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) string {
	if commonworkload.IsApplication(adminWorkload) || commonworkload.IsCICD(adminWorkload) ||
		commonworkload.GetTotalReplica(adminWorkload) != 1 {
		return ""
	}
	names := map[string]struct{}{}
	for i := range adminWorkload.Status.Pods {
		names[adminWorkload.Status.Pods[i].PodId] = struct{}{}
	}
	cached := r.cachedWorkloadPods(ctx, clientSets, adminWorkload)
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
	// Only the pod's own condition: one carried from an earlier pod of the same
	// name says nothing about how this pod ended.
	if own, _ := podDisruptions(p.FailedMessage); own != "" && v1.IsPodTerminated(p) {
		return own
	}
	return ""
}

// cachedWorkloadPods lists the workload's pods from the pod informer cache, or
// nothing when the cache cannot answer.
func (r *SyncerReconciler) cachedWorkloadPods(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) []*unstructured.Unstructured {
	if clientSets == nil || v1.GetClusterId(adminWorkload) != clientSets.name {
		return nil
	}
	informer, err := clientSets.GetResourceInformer(ctx, podResourceGVK)
	if err != nil || informer == nil || !informer.Informer().HasSynced() {
		return nil
	}
	objs, err := informer.Lister().List(
		labels.SelectorFromSet(labels.Set{v1.WorkloadIdLabel: adminWorkload.Name}))
	if err != nil {
		klog.ErrorS(err, "failed to list pods from informer cache", "name", adminWorkload.Name)
		return nil
	}
	pods := make([]*unstructured.Unstructured, 0, len(objs))
	for _, obj := range objs {
		pod, ok := obj.(*unstructured.Unstructured)
		if !ok || isStaleWorkloadGeneration(adminWorkload, v1.GetLabel(pod, v1.WorkloadUidLabel)) {
			continue
		}
		pods = append(pods, pod)
	}
	return pods
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
