/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

type WorkspaceReconciler struct {
	*ClusterBaseReconciler
	// holds all data-plane Kubernetes clients, with the key being cluster.id
	clientManager *commonutils.ObjectManager
	sync.RWMutex
	// Maintain a map of ongoing operations
	// key is workspace ID, value is the list of node IDs involved in the operation
	expectations map[string]sets.Set
	option       *WorkspaceReconcilerOption
}

type WorkspaceReconcilerOption struct {
	processWait time.Duration
	nodeWait    time.Duration
}

// SetupWorkspaceController initializes and registers the WorkspaceReconciler with the controller manager
func SetupWorkspaceController(mgr manager.Manager, opt *WorkspaceReconcilerOption) error {
	r := &WorkspaceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		clientManager: commonutils.NewObjectManagerSingleton(),
		expectations:  make(map[string]sets.Set),
		option:        opt,
	}
	if r.clientManager == nil {
		return fmt.Errorf("failed to new clientManager")
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workspace{}, builder.WithPredicates(predicate.Or(
			r.relevantChangePredicate(), predicate.GenerationChangedPredicate{}))).
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Workspace Controller successfully")
	return nil
}

// relevantChangePredicate defines which Workspace changes should trigger reconciliation
func (r *WorkspaceReconciler) relevantChangePredicate() predicate.Predicate {
	isRelevantFieldChanged := func(oldWorkspace, newWorkspace *v1.Workspace) bool {
		if oldWorkspace.Spec.Replica != newWorkspace.Spec.Replica ||
			v1.GetWorkspaceNodesAction(oldWorkspace) == "" && v1.GetWorkspaceNodesAction(newWorkspace) != "" ||
			(oldWorkspace.GetDeletionTimestamp().IsZero() && !newWorkspace.GetDeletionTimestamp().IsZero()) {
			return true
		}
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			oldWorkspace, ok1 := evt.ObjectOld.(*v1.Workspace)
			newWorkspace, ok2 := evt.ObjectNew.(*v1.Workspace)
			if !ok1 || !ok2 {
				return false
			}
			return isRelevantFieldChanged(oldWorkspace, newWorkspace)
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}
}

// handleNodeEvent creates an event handler that enqueues Workspace requests when related Node resources change
func (r *WorkspaceReconciler) handleNodeEvent() handler.EventHandler {
	isRelevantFieldChanged := func(oldNode, newNode *v1.Node) bool {
		if !reflect.DeepEqual(oldNode.Status.Resources, newNode.Status.Resources) ||
			oldNode.IsAvailable(false) != newNode.IsAvailable(false) ||
			v1.GetClusterId(oldNode) != v1.GetClusterId(newNode) ||
			oldNode.GetSpecCluster() != "" && newNode.GetSpecCluster() == "" ||
			(oldNode.GetDeletionTimestamp().IsZero() && !newNode.GetDeletionTimestamp().IsZero()) {
			return true
		}
		return false
	}
	enqueue := func(q v1.RequestWorkQueue, nodeName, workspaceId string, doObserve bool) {
		if workspaceId == "" {
			return
		}
		if doObserve {
			r.observeNode(workspaceId, nodeName)
		}
		q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: workspaceId}})
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			node, ok := evt.Object.(*v1.Node)
			if !ok {
				return
			}
			enqueue(q, node.Name, v1.GetWorkspaceId(node), true)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 {
				return
			}
			if v1.GetWorkspaceId(oldNode) != v1.GetWorkspaceId(newNode) {
				enqueue(q, newNode.Name, v1.GetWorkspaceId(oldNode), true)
				enqueue(q, newNode.Name, v1.GetWorkspaceId(newNode), true)
			} else if isRelevantFieldChanged(oldNode, newNode) {
				enqueue(q, newNode.Name, v1.GetWorkspaceId(newNode), false)
			}
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			node, ok := evt.Object.(*v1.Node)
			if !ok {
				return
			}
			enqueue(q, node.Name, v1.GetWorkspaceId(node), true)
		},
	}
}

// Reconcile is the main control loop for Workspace resources
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished reconcile %s %s cost (%v)", v1.WorkspaceKind, req.Name, time.Since(startTime))
	}()

	workspace := new(v1.Workspace)
	if err := r.Get(ctx, req.NamespacedName, workspace); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !workspace.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, workspace)
	}
	result, err := r.processWorkspace(ctx, workspace)
	if err != nil {
		klog.ErrorS(err, "failed to process workspace", "name", workspace.Name)
	}
	return result, err
}

// delete handles the deletion of a Workspace resource by unbinding nodes and removing finalizers
func (r *WorkspaceReconciler) delete(ctx context.Context, workspace *v1.Workspace) error {
	var err error
	nodeList := &v1.NodeList{}
	labelSelector := labels.SelectorFromSet(map[string]string{v1.WorkspaceIdLabel: workspace.Name})
	if err = r.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return err
	}
	nodes := commonnodes.Nodes2PointerSlice(nodeList.Items)
	if err = r.updateNodesBinding(ctx, workspace, nodes, buildTargetList(nodes, "")); err != nil {
		return err
	}
	r.removeExpectations(workspace.Name)
	if err = r.updatePhase(ctx, workspace, v1.WorkspaceDeleting); err != nil {
		klog.ErrorS(err, "failed to update phase for workspace")
		return err
	}
	return utils.RemoveFinalizer(ctx, r.Client, workspace, v1.WorkspaceFinalizer)
}

// updatePhase updates the phase of a Workspace resource
func (r *WorkspaceReconciler) updatePhase(ctx context.Context, workspace *v1.Workspace, phase v1.WorkspacePhase) error {
	if workspace.Status.Phase == phase {
		return nil
	}
	originalWorkspace := client.MergeFrom(workspace.DeepCopy())
	workspace.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
	workspace.Status.Phase = phase
	if err := r.Status().Patch(ctx, workspace, originalWorkspace); err != nil {
		return err
	}
	return nil
}

// setExpectations sets the expected node operations for a Workspace
func (r *WorkspaceReconciler) setExpectations(workspaceId string, nodeNames sets.Set) {
	r.Lock()
	defer r.Unlock()
	r.expectations[workspaceId] = nodeNames
}

// meetExpectations checks if all expected node operations for a Workspace have been completed
func (r *WorkspaceReconciler) meetExpectations(workspaceId string) bool {
	r.RLock()
	defer r.RUnlock()
	nodeNames, ok := r.expectations[workspaceId]
	return !ok || nodeNames.Len() == 0
}

// removeExpectations removes the expectations for a Workspace
func (r *WorkspaceReconciler) removeExpectations(workspaceId string) {
	r.Lock()
	defer r.Unlock()
	delete(r.expectations, workspaceId)
}

// observeNode marks a node operation as completed for a Workspace
func (r *WorkspaceReconciler) observeNode(workspaceId, nodeName string) {
	r.Lock()
	defer r.Unlock()
	leftNodeNames, ok := r.expectations[workspaceId]
	if !ok || !leftNodeNames.Has(nodeName) {
		return
	}
	leftNodeNames.Delete(nodeName)
	r.expectations[workspaceId] = leftNodeNames
}

// processWorkspace handles the main processing logic for a Workspace resource
// include scaling up and scaling down nodes.
func (r *WorkspaceReconciler) processWorkspace(ctx context.Context, workspace *v1.Workspace) (ctrlruntime.Result, error) {
	if !r.meetExpectations(workspace.Name) {
		return ctrlruntime.Result{}, nil
	}
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, workspace.Spec.Cluster)
	if err != nil || !k8sClients.IsValid() {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	if v1.GetWorkspaceNodesAction(workspace) != "" {
		isUpdated, err := r.processNodesAction(ctx, workspace)
		if err != nil || isUpdated {
			return ctrlruntime.Result{}, err
		}
	}
	if err = r.syncWorkspace(ctx, workspace); err != nil {
		return ctrlruntime.Result{}, err
	}

	totalStatusCount := workspace.CurrentReplica()
	var result ctrlruntime.Result
	switch {
	case totalStatusCount > workspace.Spec.Replica:
		count := totalStatusCount - workspace.Spec.Replica
		result, err = r.scaleDown(ctx, workspace, count)
	case totalStatusCount < workspace.Spec.Replica:
		count := workspace.Spec.Replica - totalStatusCount
		result, err = r.scaleUp(ctx, workspace, k8sClients, count)
	default:
		phase := v1.WorkspaceRunning
		if workspace.Status.AvailableReplica == 0 {
			phase = v1.WorkspaceAbnormal
		}
		if phase != workspace.Status.Phase {
			err = r.updatePhase(ctx, workspace, phase)
		}
	}
	return result, err
}

// scaleDown handles scaling down a Workspace by unbinding nodes
func (r *WorkspaceReconciler) scaleDown(ctx context.Context, workspace *v1.Workspace, count int) (ctrlruntime.Result, error) {
	nodes, err := commonnodes.GetNodesForScalingDown(ctx, r.Client, workspace.Name, count)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if len(nodes) > 0 {
		targets := buildTargetList(nodes, "")
		klog.Infof("The workspace(%s) is starting to scale down. targets: %v, targets.len: %d", workspace.Name, targets, len(targets))
		if err = r.updateNodesBinding(ctx, workspace, nodes, targets); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	if len(nodes) < count {
		return ctrlruntime.Result{RequeueAfter: r.option.nodeWait}, nil
	}
	return ctrlruntime.Result{}, nil
}

// scaleUp handles scaling up a Workspace by binding new nodes
func (r *WorkspaceReconciler) scaleUp(ctx context.Context, workspace *v1.Workspace, k8sClients *commonclient.ClientFactory, count int) (ctrlruntime.Result, error) {
	if workspace.Status.Phase == "" {
		if err := r.updatePhase(ctx, workspace, v1.WorkspaceCreating); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	nodes, err := r.getNodesForScalingUp(ctx, workspace, k8sClients, count)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if len(nodes) == 0 {
		klog.Infof("no nodes available to add. Waiting for %s seconds and then retrying.", r.option.nodeWait.String())
		return ctrlruntime.Result{RequeueAfter: r.option.nodeWait}, nil
	}
	targets := buildTargetList(nodes, workspace.Name)
	klog.Infof("The workspace(%s) is starting to scale up. targets: %v, targets.len: %d", workspace.Name, targets, len(targets))
	if err = r.updateNodesBinding(ctx, workspace, nodes, targets); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// getNodesForScalingUp retrieves available nodes for scaling up a Workspace
func (r *WorkspaceReconciler) getNodesForScalingUp(ctx context.Context, workspace *v1.Workspace, k8sClients *commonclient.ClientFactory, count int) ([]*v1.Node, error) {
	if workspace.Spec.NodeFlavor == "" {
		return nil, nil
	}
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: workspace.Spec.Cluster})
	nodeList := &v1.NodeList{}
	err := r.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	k8sNodes := make([]*corev1.Node, 0, len(nodeList.Items))
	adminNodeMap := make(map[string]*v1.Node)
	for i, n := range nodeList.Items {
		if !n.IsMachineReady() || !n.IsManaged() {
			continue
		}
		if v1.GetWorkspaceId(&n) != "" {
			continue
		}
		if v1.GetNodeFlavorId(&n) != workspace.Spec.NodeFlavor {
			continue
		}
		k8sNode, err := getNodeByInformer(ctx, k8sClients, n.GetK8sNodeName())
		if err != nil {
			klog.ErrorS(err, "failed to get k8sNode")
			continue
		}
		adminNodeMap[k8sNode.Name] = &nodeList.Items[i]
		k8sNodes = append(k8sNodes, k8sNode)
	}
	sortNodesForScalingUp(k8sNodes)
	result := make([]*v1.Node, 0, count)
	for i := range k8sNodes {
		node, _ := adminNodeMap[k8sNodes[i].Name]
		result = append(result, node)
		if len(result) >= count {
			break
		}
	}
	return result, nil
}

// sortNodesForScalingUp sorts nodes based on priority for scaling up operations
func sortNodesForScalingUp(k8sNodes []*corev1.Node) {
	sort.Slice(k8sNodes, func(i, j int) bool {
		nodeI, nodeJ := k8sNodes[i], k8sNodes[j]
		if !nodeI.GetDeletionTimestamp().IsZero() && nodeJ.GetDeletionTimestamp().IsZero() {
			return false
		}
		if !nodeJ.GetDeletionTimestamp().IsZero() && nodeI.GetDeletionTimestamp().IsZero() {
			return true
		}
		if nodeI.Spec.Unschedulable && !nodeJ.Spec.Unschedulable {
			return false
		}
		if nodeJ.Spec.Unschedulable && !nodeI.Spec.Unschedulable {
			return true
		}
		if len(nodeI.Spec.Taints) > 0 && len(nodeJ.Spec.Taints) == 0 {
			return false
		}
		if len(nodeJ.Spec.Taints) > 0 && len(nodeI.Spec.Taints) == 0 {
			return true
		}
		if v1.IsControlPlane(nodeI) && !v1.IsControlPlane(nodeJ) {
			return false
		}
		if !v1.IsControlPlane(nodeI) && v1.IsControlPlane(nodeJ) {
			return true
		}
		return strings.Compare(nodeI.Name, nodeJ.Name) < 0
	})
}

// syncWorkspace synchronizes the status of a Workspace with its bound nodes
func (r *WorkspaceReconciler) syncWorkspace(ctx context.Context, workspace *v1.Workspace) error {
	if workspace.Spec.NodeFlavor == "" {
		if isChanged := resetWorkspaceStatus(workspace); isChanged {
			workspace.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
			if err := r.Status().Update(ctx, workspace); err != nil {
				return err
			}
		}
		return nil
	}

	nodes, err := commonnodes.GetNodesOfWorkspaces(ctx, r.Client, []string{workspace.Name}, commonnodes.FilterDeletingNode)
	if err != nil {
		return err
	}
	var availReplica, abnormalReplica int
	var totalResources, availResources corev1.ResourceList
	for _, node := range nodes {
		if v1.GetNodeFlavorId(&node) != workspace.Spec.NodeFlavor {
			continue
		}
		if node.IsAvailable(false) {
			availResources = quantity.AddResource(availResources, node.Status.Resources)
			availReplica++
		} else {
			abnormalReplica++
		}
		totalResources = quantity.AddResource(totalResources, node.Status.Resources)
	}
	availResources = quantity.GetAvailableResource(availResources)
	isChanged := false
	if !quantity.Equal(totalResources, workspace.Status.TotalResources) {
		workspace.Status.TotalResources = totalResources
		isChanged = true
	}
	if !quantity.Equal(availResources, workspace.Status.AvailableResources) {
		workspace.Status.AvailableResources = availResources
		isChanged = true
	}
	if workspace.Status.AvailableReplica != availReplica {
		workspace.Status.AvailableReplica = availReplica
		isChanged = true
	}
	if workspace.Status.AbnormalReplica != abnormalReplica {
		workspace.Status.AbnormalReplica = abnormalReplica
		isChanged = true
	}
	if isChanged {
		workspace.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		if err = r.Status().Update(ctx, workspace); err != nil {
			return err
		}
	}
	return nil
}

// processNodesAction processes node binding/unbinding actions for a Workspace
func (r *WorkspaceReconciler) processNodesAction(ctx context.Context, workspace *v1.Workspace) (bool, error) {
	var actions map[string]string
	if err := json.Unmarshal([]byte(v1.GetWorkspaceNodesAction(workspace)), &actions); err != nil || len(actions) == 0 {
		if err != nil {
			klog.ErrorS(err, "failed to unmarshal json. skip it",
				"data", v1.GetWorkspaceNodesAction(workspace))
		}
		return false, r.removeNodesAction(ctx, workspace)
	}
	klog.Infof("process node action: %v", actions)

	newActions := make(map[string]string)
	adminNodes := make([]*v1.Node, 0, len(actions))
	for key, val := range actions {
		node := &v1.Node{}
		if err := r.Get(ctx, client.ObjectKey{Name: key}, node); client.IgnoreNotFound(err) != nil {
			return false, err
		}
		if node == nil || !node.GetDeletionTimestamp().IsZero() {
			continue
		}
		if val == v1.NodeActionRemove {
			if v1.GetWorkspaceId(node) == "" {
				continue
			}
			newActions[node.Name] = ""
		} else {
			if v1.GetWorkspaceId(node) == workspace.Name {
				continue
			}
			newActions[node.Name] = workspace.Name
		}
		adminNodes = append(adminNodes, node)
	}
	if len(adminNodes) == 0 {
		return false, r.removeNodesAction(ctx, workspace)
	}
	if err := r.updateNodesBinding(ctx, workspace, adminNodes, newActions); err != nil {
		return false, err
	}
	return true, nil
}

// removeNodesAction removes the node action annotation from a Workspace
func (r *WorkspaceReconciler) removeNodesAction(ctx context.Context, workspace *v1.Workspace) error {
	if v1.GetWorkspaceNodesAction(workspace) == "" {
		return nil
	}
	delete(workspace.Annotations, v1.WorkspaceNodesAction)
	if err := r.Update(ctx, workspace); err != nil {
		return err
	}
	return nil
}

// updateNodesBinding updates the binding of nodes to a Workspace
func (r *WorkspaceReconciler) updateNodesBinding(ctx context.Context,
	workspace *v1.Workspace, nodes []*v1.Node, targets map[string]string) error {
	count := len(nodes)
	if count == 0 {
		return nil
	}
	ch := make(chan *v1.Node, count)
	defer close(ch)

	nodeNames := sets.NewSet()
	for i := 0; i < count; i++ {
		nodeNames.Insert(nodes[i].Name)
		ch <- nodes[i]
	}
	r.setExpectations(workspace.Name, nodeNames)
	success, err := concurrent.Exec(count, func() error {
		n := <-ch
		ok, err := r.updateSingleNodeBinding(ctx, n, targets[n.Name])
		if !ok || err != nil {
			r.observeNode(workspace.Name, n.Name)
		}
		return err
	})
	if err != nil {
		klog.Errorf("failed to update workspace binding, name: %s, success: %d, fail: %d",
			workspace.Name, success, count-success)
		return err
	}
	return nil
}

// updateSingleNodeBinding updates the binding of a single node to a Workspace
func (r *WorkspaceReconciler) updateSingleNodeBinding(ctx context.Context, node *v1.Node, target string) (bool, error) {
	if node.Spec.Workspace != nil && *node.Spec.Workspace == target {
		return false, nil
	}
	node.Spec.Workspace = pointer.String(target)
	klog.Infof("updateSingleNodeBinding, node: %s, target: %s", node.Name, target)
	if err := r.Update(ctx, node); err != nil {
		klog.ErrorS(err, "failed to update node", "target", target)
		return false, err
	}
	return true, nil
}

// resetWorkspaceStatus resets the status of a Workspace when no node flavor is specified
func resetWorkspaceStatus(workspace *v1.Workspace) bool {
	isChanged := false
	if workspace.Status.AvailableReplica != 0 {
		workspace.Status.AvailableReplica = 0
		isChanged = true
	}
	if workspace.Status.AbnormalReplica != 0 {
		workspace.Status.AbnormalReplica = 0
		isChanged = true
	}
	if len(workspace.Status.AvailableResources) > 0 {
		workspace.Status.AvailableResources = nil
		isChanged = true
	}
	if len(workspace.Status.TotalResources) > 0 {
		workspace.Status.TotalResources = nil
		isChanged = true
	}
	if workspace.Status.Phase != v1.WorkspaceAbnormal {
		workspace.Status.Phase = v1.WorkspaceAbnormal
		isChanged = true
	}
	return isChanged
}

// buildTargetList builds a map of node names to their target Workspace names
func buildTargetList(nodes []*v1.Node, target string) map[string]string {
	results := make(map[string]string)
	for _, n := range nodes {
		results[n.Name] = target
	}
	return results
}
