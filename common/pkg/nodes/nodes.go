/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package nodes

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

// Get the resources requested by all running Pods on the specified node and namespace
// The output is a map with node names as keys and the total Pod resource as values.
func GetPodResources(ctx context.Context, k8sClient kubernetes.Interface,
	k8sNodeNames []string, namespace string) (map[string]corev1.ResourceList, error) {
	result := make(map[string]corev1.ResourceList)
	pods, err := ListPods(ctx, k8sClient, k8sNodeNames, namespace)
	if err != nil {
		return result, err
	}
	for _, p := range pods {
		nodeName := p.Spec.NodeName
		resourceList := result[nodeName]
		for _, c := range p.Spec.Containers {
			if len(c.Resources.Requests) == 0 {
				continue
			}
			resourceList = quantity.AddResource(resourceList, c.Resources.Requests)
		}
		result[nodeName] = resourceList
	}
	return result, nil
}

// Retrieve all running Pods under the given namespace and node
func ListPods(ctx context.Context, k8sClient kubernetes.Interface, k8sNodeNames []string, namespace string) ([]corev1.Pod, error) {
	if len(k8sNodeNames) == 0 {
		podList, err := k8sClient.CoreV1().Pods(namespace).List(ctx,
			metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return filter(podList.Items), nil
	}

	var results []corev1.Pod
	for _, n := range k8sNodeNames {
		listOptions := metav1.ListOptions{FieldSelector: common.NodeNameSelector + n}
		podList, err := k8sClient.CoreV1().Pods(namespace).List(ctx, listOptions)
		if err != nil {
			return nil, err
		}
		results = append(results, filter(podList.Items)...)
	}
	return results, nil
}

func filter(podList []corev1.Pod) []corev1.Pod {
	results := make([]corev1.Pod, 0, len(podList))
	for i := range podList {
		if !IsPodRunning(podList[i]) {
			continue
		}
		results = append(results, podList[i])
	}
	return results
}

func FilterDeletingNode(n v1.Node) bool {
	if !n.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

func IsPodRunning(p corev1.Pod) bool {
	return corev1.PodSucceeded != p.Status.Phase &&
		corev1.PodFailed != p.Status.Phase &&
		p.DeletionTimestamp.IsZero() &&
		p.Spec.NodeName != ""
}

// Retrieve all nodes under the given workspaces
func GetNodesOfWorkspaces(ctx context.Context, cli client.Client,
	workspaceNames []string, filterFunc func(v1.Node) bool) ([]v1.Node, error) {
	var labelSelector = labels.NewSelector()
	req, _ := labels.NewRequirement(v1.WorkspaceIdLabel, selection.In, workspaceNames)
	labelSelector = labelSelector.Add(*req)

	nodeList := &v1.NodeList{}
	err := cli.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		klog.ErrorS(err, "failed to list nodes", "selector", labelSelector.String())
		return nil, err
	}
	results := make([]v1.Node, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		if filterFunc != nil && filterFunc(nodeList.Items[i]) {
			continue
		}
		results = append(results, nodeList.Items[i])
	}
	return results, nil
}

func GetInternalIp(node *corev1.Node) string {
	internalIp := ""
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			internalIp = addr.Address
			break
		}
	}
	return internalIp
}

func BuildAction(action string, keys ...string) string {
	result := make(map[string]string)
	for _, k := range keys {
		result[k] = action
	}
	return string(jsonutils.MarshalSilently(result))
}

// returns nodes that are eligible for scale-down.
// Only idle nodes (with no running pods) are considered.
// Faulty nodes are prioritized in the result.
func GetNodesForScalingDown(ctx context.Context, cli client.Client, workspace string, count int) ([]*v1.Node, error) {
	if count <= 0 {
		return nil, fmt.Errorf("the count is less equal 0")
	}
	nodes, err := GetIdleNodesOfWorkspace(ctx, cli, workspace)
	if err != nil || len(nodes) == 0 {
		return nil, err
	}
	if count < len(nodes) {
		sort.Sort(NodeSlice(nodes))
		nodes = nodes[0:count]
	}
	return Nodes2PointerSlice(nodes), nil
}

func GetIdleNodesOfWorkspace(ctx context.Context, cli client.Client, name string) ([]v1.Node, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.WorkspaceIdLabel: name})
	workloadList := &v1.WorkloadList{}
	err := cli.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		klog.ErrorS(err, "failed to list nodes", "name", name)
		return nil, err
	}
	usedNodesSet := sets.NewSet()
	for _, w := range workloadList.Items {
		if w.IsEnd() {
			continue
		}
		for _, p := range w.Status.Pods {
			if v1.IsPodRunning(&p) {
				usedNodesSet.Insert(p.AdminNodeName)
			}
		}
	}
	filterFunc := func(n v1.Node) bool {
		if FilterDeletingNode(n) {
			return true
		}
		return usedNodesSet.Has(n.Name)
	}
	return GetNodesOfWorkspaces(ctx, cli, []string{name}, filterFunc)
}

func GetUsingNodesOfCluster(ctx context.Context, cli client.Client, clusterId string) (sets.Set, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: clusterId})
	workloadList := &v1.WorkloadList{}
	err := cli.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	result := sets.NewSet()
	for _, w := range workloadList.Items {
		if w.IsEnd() {
			continue
		}
		for _, p := range w.Status.Pods {
			result.Insert(p.AdminNodeName)
		}
	}
	return result, nil
}

func Nodes2PointerSlice(nodes []v1.Node) (result []*v1.Node) {
	for i := range nodes {
		result = append(result, &nodes[i])
	}
	return
}

type NodeSlice []v1.Node

func (ns NodeSlice) Len() int {
	return len(ns)
}

func (ns NodeSlice) Swap(i, j int) {
	ns[i], ns[j] = ns[j], ns[i]
}

func (ns NodeSlice) Less(i, j int) bool {
	ni, nj := ns[i], ns[j]
	if !ni.IsAvailable(false) && nj.IsAvailable(false) {
		return true
	}
	if !nj.IsAvailable(false) && ni.IsAvailable(false) {
		return false
	}
	return !ni.ObjectMeta.CreationTimestamp.Before(&nj.ObjectMeta.CreationTimestamp)
}
