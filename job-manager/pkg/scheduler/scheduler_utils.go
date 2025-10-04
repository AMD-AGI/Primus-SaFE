/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/floatutil"
	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

type NodeWrapper struct {
	node     *v1.Node
	resource corev1.ResourceList
	// score: Gpu * 10 + Cpu/MaxCpu + Mem/MaxMem
	resourceScore float64
}

// Get the remaining resources on each node in the current workspace.
// The returned map has keys of node name and values representing the available resource.
func getAvailableResourcesPerNode(ctx context.Context, cli client.Client,
	requestWorkload *v1.Workload, runningWorkloads []*v1.Workload) ([]NodeWrapper, error) {
	filterFunc := func(n v1.Node) bool {
		if !n.IsAvailable(requestWorkload.Spec.IsTolerateAll) {
			return true
		}
		if len(requestWorkload.Spec.CustomerLabels) > 0 && !isMatchNodeLabel(&n, requestWorkload) {
			return true
		}
		return false
	}
	nodes, err := commonnodes.GetNodesOfWorkspaces(ctx, cli, []string{requestWorkload.Spec.Workspace}, filterFunc)
	if err != nil || len(nodes) == 0 {
		return nil, err
	}

	usedResources := make(map[string]corev1.ResourceList)
	for _, w := range runningWorkloads {
		// retrieve the workload again to get the latest resource
		if err = cli.Get(ctx, client.ObjectKey{Name: w.Name}, w); err != nil {
			return nil, err
		}
		resourcesPerNode, err := commonworkload.GetResourcesPerNode(w, "")
		if err != nil {
			return nil, err
		}
		for nodeName, resourceList := range resourcesPerNode {
			usedResources[nodeName] = quantity.AddResource(usedResources[nodeName], resourceList)
		}
	}
	result := make([]NodeWrapper, 0, len(nodes))
	for i, n := range nodes {
		wrapper := NodeWrapper{
			node: &nodes[i],
		}
		usedResource, ok := usedResources[n.Name]
		availResource := quantity.GetAvailableResource(n.Status.Resources)
		if ok {
			wrapper.resource = quantity.SubResource(availResource, usedResource)
		} else {
			wrapper.resource = availResource
		}
		result = append(result, wrapper)
	}
	return result, nil
}

func buildReason(workload *v1.Workload, podResources corev1.ResourceList, nodes []*NodeWrapper) string {
	reason := ""
	if len(nodes) == 0 {
		reason = "All nodes are unavailable"
	} else {
		sort.Slice(nodes, func(i, j int) bool {
			if floatutil.FloatEqual(nodes[i].resourceScore, nodes[j].resourceScore) {
				return nodes[i].node.Name < nodes[j].node.Name
			}
			return nodes[i].resourceScore > nodes[j].resourceScore
		})
		_, key := quantity.IsSubResource(podResources, nodes[0].resource)
		reason = fmt.Sprintf("Insufficient %s due to node fragmentation", formatResourceName(key))
	}
	if len(workload.Spec.CustomerLabels) > 0 {
		reason += "or not enough nodes match the specified label."
	}
	return reason
}

func formatResourceName(key string) string {
	if key == common.NvidiaGpu || key == common.AmdGpu {
		return "gpu"
	}
	return key
}

func isMatchNodeLabel(node *v1.Node, workload *v1.Workload) bool {
	for key, val := range workload.Spec.CustomerLabels {
		if key == common.K8sHostName {
			nodeNames := strings.Split(val, " ")
			if !sliceutil.Contains(nodeNames, v1.GetDisplayName(node)) {
				return false
			}
		} else if node.Labels[key] != val {
			return false
		}
	}
	return true
}

func buildResourceWeight(workload *v1.Workload, resources corev1.ResourceList, nf *v1.NodeFlavor) float64 {
	if workload == nil {
		return 0
	}
	var weight float64 = 0
	if workload.Spec.Resource.GPU != "" {
		if gpuQuantity, ok := resources[corev1.ResourceName(workload.Spec.Resource.GPUName)]; ok {
			weight += float64(gpuQuantity.Value() * 10)
		}
	}
	if workload.Spec.Resource.Memory != "" && nf != nil && !nf.Spec.Memory.IsZero() {
		if memoryQuantity := resources.Memory(); memoryQuantity != nil {
			weight += float64(memoryQuantity.Value()) / float64(nf.Spec.Memory.Value())
		}
	}
	if workload.Spec.Resource.CPU != "" && nf != nil && !nf.Spec.Cpu.Quantity.IsZero() {
		if cpuQuantity := resources.Cpu(); cpuQuantity != nil {
			weight += float64(cpuQuantity.Value()) / float64(nf.Spec.Cpu.Quantity.Value())
		}
	}
	return weight
}

type WorkloadList []*v1.Workload

func (ws WorkloadList) Len() int {
	return len(ws)
}

func (ws WorkloadList) Swap(i, j int) {
	ws[i], ws[j] = ws[j], ws[i]
}

func (ws WorkloadList) Less(i, j int) bool {
	if isReScheduledDueToFailover(ws[i]) && !isReScheduledDueToFailover(ws[j]) {
		return true
	} else if !isReScheduledDueToFailover(ws[i]) && isReScheduledDueToFailover(ws[j]) {
		return false
	}
	if ws[i].Spec.Priority > ws[j].Spec.Priority {
		return true
	} else if ws[i].Spec.Priority < ws[j].Spec.Priority {
		return false
	}
	if ws[i].CreationTimestamp.Time.Before(ws[j].CreationTimestamp.Time) {
		return true
	}
	if ws[i].CreationTimestamp.Time.Equal(ws[j].CreationTimestamp.Time) && ws[i].Name < ws[j].Name {
		return true
	}
	return false
}

func isReScheduledDueToFailover(workload *v1.Workload) bool {
	if v1.IsWorkloadReScheduled(workload) && !v1.IsWorkloadPreempted(workload) {
		return true
	}
	return false
}
