/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package nodes

import (
	"context"
	"encoding/json"
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

// GetPodResources retrieves the resources requested by all running Pods on the specified nodes and namespace.
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

// ListPods retrieves all running Pods under the given namespace and nodes.
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

// filter filters out non-running pods from the provided pod list.
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

// FilterDeletingNode checks if a node is being deleted.
func FilterDeletingNode(n v1.Node) bool {
	if !n.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

// FilterUnclaimedNode returns a node filter that keeps only the nodes the named workspace
// actually holds, on top of the deleting-node filter.
//
// A workspace's nodes are listed by WorkspaceIdLabel because that is what the admin plane
// indexes on, but the label is a mirror of the data plane and lags Node.Spec.Workspace by a
// whole round trip -- admin spec.workspace, then updateK8sNodeWorkspace writes the k8s Node
// label, then syncK8sMetadata mirrors it back onto the admin Node. The claim is the
// authoritative half, and it is the same field WorkspaceReconciler.judgeNodeBinding decides an
// unbind on.
//
// Every caller that has to agree with another caller about "which nodes does this workspace
// have" needs this filter, not the label alone. Two of them do, and they have to agree with
// each other: syncWorkspace, which turns the list into CurrentReplica, and
// GetIdleNodesOfWorkspace, which turns it into scale-down candidates. Counting on the label
// while choosing on the claim is not a smaller version of the same bug -- it is a worse one,
// because the arithmetic then asks for more nodes than the candidate list can honestly offer.
func FilterUnclaimedNode(workspace string) func(v1.Node) bool {
	return func(n v1.Node) bool {
		if FilterDeletingNode(n) {
			return true
		}
		return n.GetSpecWorkspace() != workspace
	}
}

// IsPodRunning returns true if the pod is running
func IsPodRunning(p corev1.Pod) bool {
	return corev1.PodSucceeded != p.Status.Phase &&
		corev1.PodFailed != p.Status.Phase &&
		p.DeletionTimestamp.IsZero() &&
		p.Spec.NodeName != ""
}

// OccupiedNodes returns the admin nodes a workload currently holds, without
// duplicates.
//
// The NodePodUsage aggregate answers whenever it carries entries, and the
// per-pod array answers otherwise. Both sources describe one placement and must
// report it alike, so the pod branch mirrors workload.BuildNodeUsage: pods that
// v1.IsPodTerminated accepts are skipped, and a pod holding no admin node yet
// contributes nothing. This is deliberately not v1.IsPodRunning, which admits a
// Stopped pod because it tests only Succeeded and Failed.
//
// An empty result does not distinguish a workload that occupies nothing from one
// whose aggregate is missing, and etcd alone cannot settle which it is: an
// offloaded workload's per-pod array is cleared there by design. A caller that
// must not act on the wrong reading has to take the offload annotation and the
// db config into account as well.
func OccupiedNodes(w *v1.Workload) []string {
	if w == nil {
		return nil
	}
	nodes := make([]string, 0, len(w.Status.NodeUsage))
	seen := make(map[string]struct{}, len(w.Status.NodeUsage))
	add := func(node string) {
		if node == "" {
			return
		}
		if _, dup := seen[node]; dup {
			return
		}
		seen[node] = struct{}{}
		nodes = append(nodes, node)
	}

	if len(w.Status.NodeUsage) > 0 {
		for i := range w.Status.NodeUsage {
			add(w.Status.NodeUsage[i].Node)
		}
		return nodes
	}
	for i := range w.Status.Pods {
		if v1.IsPodTerminated(&w.Status.Pods[i]) {
			continue
		}
		add(w.Status.Pods[i].AdminNodeName)
	}
	return nodes
}

// GetNodesOfWorkspaces retrieves all nodes under the given workspaces(as namespaces).
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

// GetNodesOfCluster retrieves all nodes belonging to a specific cluster.
func GetNodesOfCluster(ctx context.Context, cli client.Client,
	clusterId string, filterFunc func(v1.Node) bool) ([]v1.Node, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: clusterId})
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

// GetInternalIp extracts the internal IP address from a node.
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

// BuildAction builds and returns the constructed object.
func BuildAction(action string, keys ...string) string {
	result := make(map[string]string)
	for _, k := range keys {
		result[k] = action
	}
	return string(jsonutils.MarshalSilently(result))
}

// ParseAction reads the nodes-action annotation off a Workspace into the map it encodes.
//
// An empty annotation and an annotation holding an empty object both read as no request: the
// controller clears a finished request by emptying the annotation, and BuildAction of nothing
// writes "{}", so the two spellings have to mean the same thing to every reader.
//
// It lives here for the reason WithdrawnReplica does. Both webhooks and the controller read
// this annotation, and they have to agree about what it says -- what counts as no request at
// all most of all, because that is the answer the in-flight check turns into "another job is
// processing" or not. Callers that cannot act on a malformed value discard the error; see
// the controller's parseNodesAction for why.
func ParseAction(w *v1.Workspace) (map[string]string, error) {
	raw := v1.GetWorkspaceNodesAction(w)
	if raw == "" {
		return nil, nil
	}
	var actions map[string]string
	if err := json.Unmarshal([]byte(raw), &actions); err != nil {
		return nil, err
	}
	if len(actions) == 0 {
		return nil, nil
	}
	return actions, nil
}

// GetNodesForScalingDown returns nodes eligible for scale-down operations.
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

// GetIdleNodesOfWorkspace retrieves idle nodes (nodes with no running workloads) in a workspace.
//
// "In a workspace" is answered by FilterUnclaimedNode, not by the label the List selects on --
// see the note there. The only consumer of this function is scale-down, where a stale label
// costs real machines: a node this workspace has already released, or that another workspace
// has since taken, would otherwise be offered as a candidate and then refused at the write.
func GetIdleNodesOfWorkspace(ctx context.Context, cli client.Client, name string) ([]v1.Node, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.WorkspaceIdLabel: name})
	workloadList := &v1.WorkloadList{}
	err := cli.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		klog.ErrorS(err, "failed to list nodes", "name", name)
		return nil, err
	}
	usedNodesSet := sets.NewSet()
	for i := range workloadList.Items {
		w := &workloadList.Items[i]
		if w.IsEnd() {
			continue
		}
		for _, node := range OccupiedNodes(w) {
			usedNodesSet.Insert(node)
		}
	}
	claimed := FilterUnclaimedNode(name)
	filterFunc := func(n v1.Node) bool {
		if claimed(n) {
			return true
		}
		return usedNodesSet.Has(n.Name)
	}
	return GetNodesOfWorkspaces(ctx, cli, []string{name}, filterFunc)
}

// GetUsingNodesOfCluster retrieves nodes that are currently in use by workloads in a cluster.
func GetUsingNodesOfCluster(ctx context.Context, cli client.Client, clusterId string) (sets.Set, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: clusterId})
	workloadList := &v1.WorkloadList{}
	err := cli.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	result := sets.NewSet()
	for i := range workloadList.Items {
		w := &workloadList.Items[i]
		if w.IsEnd() {
			continue
		}
		for _, node := range OccupiedNodes(w) {
			result.Insert(node)
		}
	}
	return result, nil
}

// WithdrawnReplica returns the only Spec.Replica a withdrawal of these entries may carry.
//
// The mutating webhook moves the count by one for every add it accepts -- from 0 it sets 1,
// otherwise it increments -- so undoing one add is a decrement either way. A withdrawn remove
// is deliberately not undone: the controller only ever refuses a remove for a node that has
// since been bound to a different workspace, so this workspace has lost the node whether or
// not it asked to, and the decrement that already applied describes where it ended up.
// Restoring it would be asking for a machine to replace one that was never released.
//
// A withdrawn migration is undone, and that is the difference between the two. Admission
// counts one out for a migration exactly as it does for a removal, but a migration that is
// withdrawn is one that did not happen: either the node never left, in which case the
// workspace is holding a machine it has stopped counting and will release a healthy one to
// get back down, or it left and has ended up unassigned, in which case the workspace lost
// capacity to a failure it did not cause. Both are put right by counting it back in, and only
// the first of the two is put right by anything else.
//
// Undone from the action value alone, not from where the node happens to be. Every caller has
// to arrive at the same number and only one of them can see the node, so a rule that consults
// it is a rule the others cannot apply: the withdrawal then fails to be recognised as one,
// and the workspace is left with a request it can neither finish nor drop.
//
// It lives here rather than beside either caller because there are three of them: the
// controller computes it to write the withdrawal, and both webhooks compute it to recognise
// one. The whole mechanism rests on all three arriving at the same number, so there is one
// function and no copies to keep in step.
func WithdrawnReplica(replica int, oldActions, newActions map[string]string) int {
	for key, val := range oldActions {
		if _, kept := newActions[key]; kept {
			continue
		}
		switch {
		case val == v1.NodeActionAdd:
			if replica > 0 {
				replica--
			}
		default:
			if _, isMigration := v1.ParseMigrateAction(val); isMigration {
				replica++
			}
		}
	}
	return replica
}

// Nodes2PointerSlice converts a slice of nodes to a slice of node pointers.
func Nodes2PointerSlice(nodes []v1.Node) (result []*v1.Node) {
	for i := range nodes {
		result = append(result, &nodes[i])
	}
	return
}

// NodeSlice implements sort.Interface for sorting nodes
// Provides custom sorting logic prioritizing unavailable nodes and sorting by creation timestamp
type NodeSlice []v1.Node

// Len implements sort.Interface by returning the length of the slice.
func (ns NodeSlice) Len() int {
	return len(ns)
}

// Swap implements sort.Interface by swapping elements at the given indices.
func (ns NodeSlice) Swap(i, j int) {
	ns[i], ns[j] = ns[j], ns[i]
}

// Less implements sort.Interface for sorting.
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
