/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package enricher

import (
	"context"
	"fmt"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkloadInfo is the resolved owner of a GPU-using pod.
type WorkloadInfo struct {
	UID       string
	Name      string
	Namespace string
	User      string
}

// gpuResourceName is the AMD GPU extended resource requested by GPU workloads.
const gpuResourceName = "amd.com/gpu"

// PodAttribution links one GPU-workload pod to its owning workload. Used to
// emit the workload_pod_info join metric so dashboards can filter existing
// cAdvisor container_* series by workload_uid.
type PodAttribution struct {
	Namespace string
	Name      string
	Node      string
	Info      WorkloadInfo
}

// Mapping holds the resolved attribution tables produced each pass.
type Mapping struct {
	// ByPod maps namespace/pod -> workload (used when the AMD exporter attaches
	// pod/namespace labels to GPU series).
	ByPod map[string]WorkloadInfo
	// ByNode maps node hostname -> workload, but ONLY for nodes running exactly
	// one GPU workload. This is the fallback for clusters where the exporter
	// doesn't populate pod labels: whole-node GPU jobs (the common case) can
	// still be attributed via the always-present `hostname` label.
	ByNode map[string]WorkloadInfo
	// Pods lists every resolvable GPU-workload pod, for the workload_pod_info
	// join metric.
	Pods []PodAttribution
}

// Mapper builds attribution tables from the cluster.
//
// The join uses SaFE's pod convention: workload pods carry a label
// (default primus-safe.workload.id) whose value is the owning Workload name;
// that name resolves to the Workload CR UID (what the Grafana dashboards
// filter on via var-workload_uid).
type Mapper struct {
	c        client.Client
	podLabel string
}

func NewMapper(c client.Client, podLabel string) *Mapper {
	return &Mapper{c: c, podLabel: podLabel}
}

// podKey returns the map key used to look a GPU sample's pod up.
func podKey(namespace, pod string) string {
	return namespace + "/" + pod
}

// Build returns the current attribution mapping. It is called once per enrich
// pass so newly scheduled workloads are picked up promptly.
func (m *Mapper) Build(ctx context.Context) (*Mapping, error) {
	// Workload name -> info (UID/user). Workloads are resolved by name because
	// that is what the pod label carries.
	wlList := &v1.WorkloadList{}
	if err := m.c.List(ctx, wlList); err != nil {
		return nil, fmt.Errorf("list workloads: %w", err)
	}
	byName := make(map[string]WorkloadInfo, len(wlList.Items))
	for i := range wlList.Items {
		wl := &wlList.Items[i]
		byName[wl.Name] = WorkloadInfo{
			UID:       string(wl.UID),
			Name:      wl.Name,
			Namespace: wl.Spec.Workspace,
			User:      workloadUser(wl),
		}
	}

	// Pods carrying the workload label -> workload name, keyed by namespace/pod.
	podList := &corev1.PodList{}
	if err := m.c.List(ctx, podList, client.HasLabels{m.podLabel}); err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	byPod := make(map[string]WorkloadInfo, len(podList.Items))
	var pods []PodAttribution
	// nodeUIDs tracks the distinct GPU workloads seen per node so we only build a
	// node->workload fallback when attribution is unambiguous.
	nodeUIDs := map[string]map[string]WorkloadInfo{}
	for i := range podList.Items {
		pod := &podList.Items[i]
		wlName := pod.Labels[m.podLabel]
		if wlName == "" {
			continue
		}
		info, ok := byName[wlName]
		if !ok {
			// Pod references a workload we can't resolve to a UID yet; fall back
			// to the name so series are still queryable, UID left empty.
			info = WorkloadInfo{Name: wlName}
		}
		// Prefer the pod's actual namespace for the workload_namespace label.
		info.Namespace = pod.Namespace
		byPod[podKey(pod.Namespace, pod.Name)] = info

		if info.UID != "" {
			pods = append(pods, PodAttribution{
				Namespace: pod.Namespace,
				Name:      pod.Name,
				Node:      pod.Spec.NodeName,
				Info:      info,
			})
		}

		if info.UID != "" && podRequestsGPU(pod) && pod.Spec.NodeName != "" {
			if nodeUIDs[pod.Spec.NodeName] == nil {
				nodeUIDs[pod.Spec.NodeName] = map[string]WorkloadInfo{}
			}
			nodeUIDs[pod.Spec.NodeName][info.UID] = info
		}
	}

	byNode := make(map[string]WorkloadInfo)
	for node, uids := range nodeUIDs {
		if len(uids) == 1 {
			for _, info := range uids {
				byNode[node] = info
			}
		}
	}

	return &Mapping{ByPod: byPod, ByNode: byNode, Pods: pods}, nil
}

// podRequestsGPU reports whether any container requests the AMD GPU resource.
func podRequestsGPU(pod *corev1.Pod) bool {
	for i := range pod.Spec.Containers {
		res := pod.Spec.Containers[i].Resources
		if _, ok := res.Requests[gpuResourceName]; ok {
			return true
		}
		if _, ok := res.Limits[gpuResourceName]; ok {
			return true
		}
	}
	return false
}

// workloadUser extracts a best-effort owner/user for the workload_user label.
func workloadUser(wl *v1.Workload) string {
	return v1.GetUserId(wl)
}
