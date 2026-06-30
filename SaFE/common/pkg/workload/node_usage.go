/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
)

// ridKey encodes a resourceId as the decimal-string map key used by
// NodePodUsage (CRD/OpenAPI map keys must be strings).
func ridKey(rid int8) string {
	return strconv.Itoa(int(rid))
}

// parseRid decodes a NodePodUsage map key back into a resourceId index.
func parseRid(key string) (int, bool) {
	v, err := strconv.Atoi(key)
	if err != nil {
		return 0, false
	}
	return v, true
}

// BuildNodeUsage aggregates a workload's per-pod status (Status.Pods) into the
// O(node) NodePodUsage form stored in etcd. Terminated pods (Succeeded/Failed)
// are excluded. Active counts non-terminated pods per resourceId; Running counts
// pods in the actual Running phase. Unscheduled non-terminated pods (no admin
// node yet) are bucketed under the empty node key so resource totals stay exact.
// This is the canonical producer used by the syncer (P3) and by the read-path
// fallback so the NodeUsage-derived results match the legacy Pods-derived ones.
func BuildNodeUsage(w *v1.Workload) []v1.NodePodUsage {
	if w == nil || len(w.Status.Pods) == 0 {
		return nil
	}
	idx := make(map[string]*v1.NodePodUsage)
	order := make([]string, 0)
	for i := range w.Status.Pods {
		p := &w.Status.Pods[i]
		if v1.IsPodTerminated(p) {
			continue
		}
		u, ok := idx[p.AdminNodeName]
		if !ok {
			u = &v1.NodePodUsage{
				Node:    p.AdminNodeName,
				Active:  make(map[string]int),
				Running: make(map[string]int),
			}
			idx[p.AdminNodeName] = u
			order = append(order, p.AdminNodeName)
		}
		key := ridKey(p.ResourceId)
		u.Active[key]++
		if p.Phase == corev1.PodRunning {
			u.Running[key]++
		}
	}
	result := make([]v1.NodePodUsage, 0, len(order))
	for _, n := range order {
		result = append(result, *idx[n])
	}
	return result
}

// totalNodeCountFromUsage counts distinct nodes holding active pods.
func totalNodeCountFromUsage(usage []v1.NodePodUsage) int {
	return len(usage)
}

// resourcesPerNodeFromUsage reproduces GetResourcesPerNode from the aggregate:
// sum(active[rid] * perPodResource[rid]) per real node, with an optional node
// filter. Unscheduled pods (empty node) are skipped, matching IsPodRunning.
func resourcesPerNodeFromUsage(usage []v1.NodePodUsage, allPodResources []corev1.ResourceList,
	adminNodeName string) map[string]corev1.ResourceList {
	result := map[string]corev1.ResourceList{}
	for i := range usage {
		node := usage[i].Node
		if node == "" {
			continue
		}
		if adminNodeName != "" && adminNodeName != node {
			continue
		}
		for ridStr, count := range usage[i].Active {
			rid, ok := parseRid(ridStr)
			if !ok || rid >= len(allPodResources) || count <= 0 {
				continue
			}
			result[node] = addResourceNTimes(result[node], allPodResources[rid], count)
		}
	}
	return result
}

// addResourceNTimes adds podResource to acc n times. This mirrors the legacy
// per-pod accumulation (one AddResource call per pod) so the aggregate result
// is identical to iterating Status.Pods.
func addResourceNTimes(acc, podResource corev1.ResourceList, n int) corev1.ResourceList {
	for i := 0; i < n; i++ {
		acc = quantity.AddResource(acc, podResource)
	}
	return acc
}

// workloadResourceUsageFromUsage reproduces GetWorkloadResourceUsage from the
// aggregate: total over all active pods, available over the non-filtered nodes,
// and the distinct non-filtered node names.
func workloadResourceUsageFromUsage(usage []v1.NodePodUsage, allPodResources []corev1.ResourceList,
	filterNode func(nodeName string) bool) (corev1.ResourceList, corev1.ResourceList, []string) {
	totalResource := make(corev1.ResourceList)
	availableResource := make(corev1.ResourceList)
	availableNodes := make([]string, 0, len(usage))
	for i := range usage {
		node := usage[i].Node
		filtered := filterNode != nil && filterNode(node)
		for ridStr, count := range usage[i].Active {
			rid, ok := parseRid(ridStr)
			if !ok || rid >= len(allPodResources) || count <= 0 {
				continue
			}
			totalResource = addResourceNTimes(totalResource, allPodResources[rid], count)
			if !filtered {
				availableResource = addResourceNTimes(availableResource, allPodResources[rid], count)
			}
		}
		if !filtered {
			availableNodes = append(availableNodes, node)
		}
	}
	return totalResource, availableResource, availableNodes
}
