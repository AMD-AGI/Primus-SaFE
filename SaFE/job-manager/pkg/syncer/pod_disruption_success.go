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

// disruptedSuccess returns the DisruptionTarget of a workload whose k8s object
// reports success although no pod of it ended cleanly: every pod that ended
// was taken away by a disruption. It returns "" otherwise.
//
// A pod whose containers exit 0 on the SIGTERM of an eviction, a preemption or
// a node drain ends Succeeded, and an object that mirrors its pod's phase then
// reports success. Kubernetes records the disruption on the pod itself as the
// DisruptionTarget condition, set before the pod is terminated; SaFE carries
// it into the pod's FailedMessage (see podDisruptionTarget). A pod that did
// end Succeeded without one is a clean success, and then the object's success
// stands: a Job whose evicted pod was replaced by one that completed did
// succeed.
//
// The evidence is the workload's pod records, and the pods still in the
// informer cache: the object may report success before the pod's last event
// has been recorded.
func (r *SyncerReconciler) disruptedSuccess(ctx context.Context, clientSets *ClusterClientSets,
	adminWorkload *v1.Workload) string {
	if commonworkload.IsApplication(adminWorkload) || commonworkload.IsCICD(adminWorkload) {
		return ""
	}
	disrupted := map[string]string{}
	succeeded := map[string]bool{}
	for i := range adminWorkload.Status.Pods {
		p := &adminWorkload.Status.Pods[i]
		if at := strings.Index(p.FailedMessage, disruptionTargetPrefix); at >= 0 {
			disrupted[p.PodId] = p.FailedMessage[at:]
		} else if p.Phase == corev1.PodSucceeded {
			succeeded[p.PodId] = true
		}
	}
	for _, obj := range r.cachedWorkloadPods(ctx, clientSets, adminWorkload) {
		if disruption := disruptionTargetOf(obj); disruption != "" {
			if _, ok := disrupted[obj.GetName()]; !ok {
				disrupted[obj.GetName()] = disruption
			}
			continue
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		if corev1.PodPhase(phase) == corev1.PodSucceeded {
			succeeded[obj.GetName()] = true
		}
	}
	first := ""
	for name, disruption := range disrupted {
		delete(succeeded, name)
		if first == "" || disruption < first {
			first = disruption
		}
	}
	if len(succeeded) > 0 {
		return ""
	}
	return first
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
