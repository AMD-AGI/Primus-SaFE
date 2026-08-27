/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmmetrics "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

type WorkspaceReconciler struct {
	*ClusterBaseReconciler
	// apiReader reads straight from the apiserver, bypassing the manager's cache. Reserved
	// for updateSingleNodeBinding, where the read is both the state the ownership rule is
	// judged against and the resourceVersion the write is locked to.
	apiReader client.Reader
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

// SetupWorkspaceController initializes and registers the WorkspaceReconciler with the controller manager.
func SetupWorkspaceController(mgr manager.Manager, opt *WorkspaceReconcilerOption) error {
	baseReconciler, err := newClusterBaseReconciler(mgr)
	if err != nil {
		return err
	}
	r := &WorkspaceReconciler{
		ClusterBaseReconciler: baseReconciler,
		apiReader:             mgr.GetAPIReader(),
		clientManager:         commonutils.NewObjectManagerSingleton(),
		expectations:          make(map[string]sets.Set),
		option:                opt,
	}
	if r.clientManager == nil {
		return fmt.Errorf("failed to new clientManager")
	}
	err = ctrlruntime.NewControllerManagedBy(mgr).
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

// relevantChangePredicate defines which Workspace changes should trigger reconciliation.
func (r *WorkspaceReconciler) relevantChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(evt event.UpdateEvent) bool {
			oldWorkspace, ok1 := evt.ObjectOld.(*v1.Workspace)
			newWorkspace, ok2 := evt.ObjectNew.(*v1.Workspace)
			if !ok1 || !ok2 {
				return false
			}
			if v1.GetWorkspaceNodesAction(oldWorkspace) == "" && v1.GetWorkspaceNodesAction(newWorkspace) != "" ||
				(oldWorkspace.GetDeletionTimestamp().IsZero() && !newWorkspace.GetDeletionTimestamp().IsZero()) {
				return true
			}
			return false
		},
	}
}

// handleNodeEvent creates an event handler that enqueues Workspace requests when related Node resources change.
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
	enqueue := func(q v1.RequestWorkQueue, workspaceId string) {
		if workspaceId == "" {
			return
		}
		q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: workspaceId}})
	}
	// observe settles the node against the workspaces this event actually names, and only
	// those. A label move from one workspace to another passes through the empty string on
	// its way, and both halves are events: crediting every waiter on the empty one would tell
	// the incoming workspace its label had arrived while it was still on the way out from the
	// previous owner. It would then count a node it does not yet hold as missing, bind an
	// extra machine, and give it back on the following round.
	observe := func(nodeName string, workspaceIds ...string) {
		for _, workspaceId := range workspaceIds {
			if workspaceId == "" {
				continue
			}
			r.observeNode(workspaceId, nodeName)
		}
	}
	// observeGone settles the node against every workspace waiting on it. Only for deletion:
	// the waiting workspace may be one this event does not name -- a bind that lands and is
	// followed by the node's deletion leaves it waiting on a label that will never arrive --
	// and unlike a label change, no further event is coming to correct for over-crediting.
	observeGone := func(q v1.RequestWorkQueue, nodeName string) {
		for _, workspaceId := range r.observeNodeForAll(nodeName) {
			enqueue(q, workspaceId)
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			node, ok := evt.Object.(*v1.Node)
			if !ok {
				return
			}
			observe(node.Name, v1.GetWorkspaceId(node))
			enqueue(q, v1.GetWorkspaceId(node))
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 {
				return
			}
			if v1.GetWorkspaceId(oldNode) != v1.GetWorkspaceId(newNode) {
				observe(newNode.Name, v1.GetWorkspaceId(oldNode), v1.GetWorkspaceId(newNode))
				enqueue(q, v1.GetWorkspaceId(oldNode))
				enqueue(q, v1.GetWorkspaceId(newNode))
			} else if oldNode.GetSpecWorkspace() != "" && newNode.GetSpecWorkspace() == "" &&
				v1.GetWorkspaceId(newNode) == "" {
				// The claim was dropped without a label to drop with it. cleanupNodeAfterUnmanage
				// releases a node this way when it is unmanaged between a bind writing the claim
				// and the label making the round trip, and the owner is waiting on that label:
				// nothing else is coming, so the claim going is the event that settles it. The
				// label check is what keeps an ordinary unbind out of here -- there the label
				// goes too, and settling on the claim would credit the incoming workspace early.
				observe(newNode.Name, oldNode.GetSpecWorkspace())
				enqueue(q, oldNode.GetSpecWorkspace())
			} else if isRelevantFieldChanged(oldNode, newNode) {
				enqueue(q, v1.GetWorkspaceId(newNode))
			}
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			node, ok := evt.Object.(*v1.Node)
			if !ok {
				return
			}
			observeGone(q, node.Name)
			enqueue(q, v1.GetWorkspaceId(node))
		},
	}
}

// guaranteeDataPlaneResources creates required resources in the data plane for a workspace.
func (r *WorkspaceReconciler) guaranteeDataPlaneResources(ctx context.Context, workspace *v1.Workspace, clientSet kubernetes.Interface) error {
	// create namespace for data plane
	if err := createDataplaneNamespace(ctx, workspace.Name, clientSet); err != nil {
		klog.ErrorS(err, "failed to create namespace for data plane", "name", workspace.Name)
		return err
	}
	if err := createDataPlanePv(ctx, workspace, r.Client, clientSet); err != nil {
		klog.ErrorS(err, "failed to create pv for data plane", "name", workspace.Name)
		return err
	}
	if err := createDataPlanePvc(ctx, workspace, clientSet); err != nil {
		klog.ErrorS(err, "failed to create pvc for data plane", "name", workspace.Name)
		return err
	}
	if err := createCICDServiceAccount(ctx, workspace, clientSet); err != nil {
		klog.ErrorS(err, "failed to create service account for cicd", "name", workspace.Name)
		return err
	}
	if err := createMonarchServiceAccount(ctx, workspace, clientSet); err != nil {
		klog.ErrorS(err, "failed to create service account for monarch", "name", workspace.Name)
		return err
	}
	if err := syncDataPlanePVC(ctx, workspace, clientSet); err != nil {
		klog.ErrorS(err, "failed to sync pvc for data plane", "name", workspace.Name)
		return err
	}
	return nil
}

func (r *WorkspaceReconciler) getClientSetOfDataplane(ctx context.Context, clusterId string) (kubernetes.Interface, error) {
	if clusterId == "" {
		return nil, nil
	}
	cluster := &v1.Cluster{}
	if err := r.Get(ctx, client.ObjectKey{Name: clusterId}, cluster); err != nil {
		return nil, err
	}
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, clusterId)
	if err != nil || !k8sClients.IsValid() {
		return nil, fmt.Errorf("the cluster(%s) clients is not ready", clusterId)
	}
	return k8sClients.ClientSet(), nil
}

// deleteDataPlaneResources deletes data plane resources when a workspace is deleted.
func (r *WorkspaceReconciler) deleteDataPlaneResources(ctx context.Context, workspace *v1.Workspace) error {
	clientSet, err := r.getClientSetOfDataplane(ctx, workspace.Spec.Cluster)
	if err != nil || clientSet == nil {
		return client.IgnoreNotFound(err)
	}
	if err = deletePV(ctx, workspace, clientSet); err != nil {
		return err
	}
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		if err = deletePVC(ctx, vol.GenFullVolumeId(), workspace.Name, clientSet); err != nil {
			return err
		}
	}

	if err = deleteCICDServiceAccount(ctx, workspace, clientSet); err != nil {
		return err
	}
	if err = deleteMonarchServiceAccount(ctx, workspace, clientSet); err != nil {
		return err
	}
	if err = deleteWorkspaceSecrets(ctx, workspace, clientSet); err != nil {
		return err
	}
	if err = deleteDataplaneNamespace(ctx, workspace.Name, clientSet); err != nil {
		return err
	}
	return nil
}

// Reconcile is the main control loop for Workspace resources.
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	workspace := new(v1.Workspace)
	if err := r.Get(ctx, req.NamespacedName, workspace); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !workspace.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, workspace)
	}
	clientSet, err := r.getClientSetOfDataplane(ctx, workspace.Spec.Cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlruntime.Result{}, nil
		}
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	if clientSet == nil {
		return ctrlruntime.Result{}, nil
	}
	if err = r.guaranteeDataPlaneResources(ctx, workspace, clientSet); err != nil {
		return ctrlruntime.Result{}, err
	}
	result, err := r.processWorkspace(ctx, workspace)
	if err != nil {
		klog.ErrorS(err, "failed to process workspace", "name", workspace.Name)
	}
	return result, err
}

// delete handles the deletion of a Workspace resource by unbinding nodes and removing finalizers.
func (r *WorkspaceReconciler) delete(ctx context.Context, workspace *v1.Workspace) error {
	var err error
	if err = r.updatePhase(ctx, workspace, v1.WorkspaceDeleting); err != nil {
		klog.ErrorS(err, "failed to update phase for workspace", "name", workspace.Name)
		return err
	}

	nodeList := &v1.NodeList{}
	if err = r.List(ctx, nodeList); err != nil {
		return err
	}
	var nodes []*v1.Node
	for i, item := range nodeList.Items {
		if item.GetSpecWorkspace() == workspace.Name {
			nodes = append(nodes, &nodeList.Items[i])
		}
	}
	if err = r.updateNodesBinding(ctx, workspace, nodes, buildTargetList(nodes, "")); err != nil {
		return err
	}
	// Wait for all expected unbind operations to be observed before proceeding
	if !r.meetExpectations(workspace.Name) {
		klog.Infof("Workspace(%s) delete waiting for node unbinding to complete", workspace.Name)
		return nil
	}
	r.removeExpectations(workspace.Name)
	if err = r.deleteDataPlaneResources(ctx, workspace); err != nil {
		klog.ErrorS(err, "failed to delete data plane resources for workspace", "name", workspace.Name)
		return err
	}
	return utils.RemoveFinalizer(ctx, r.Client, workspace, v1.WorkspaceFinalizer)
}

// updatePhase updates the phase of a Workspace resource.
func (r *WorkspaceReconciler) updatePhase(ctx context.Context, workspace *v1.Workspace, phase v1.WorkspacePhase) error {
	if workspace.Status.Phase == phase {
		return nil
	}
	patch := client.MergeFrom(workspace.DeepCopy())
	workspace.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
	workspace.Status.Phase = phase
	if err := r.Status().Patch(ctx, workspace, patch); err != nil {
		return err
	}
	rmmetrics.WorkspacePhaseTotal.WithLabelValues(string(phase)).Inc()
	return nil
}

// setExpectations sets the expected node operations for a Workspace.
func (r *WorkspaceReconciler) setExpectations(workspaceId string, nodeNames sets.Set) {
	r.Lock()
	defer r.Unlock()
	r.expectations[workspaceId] = nodeNames
}

// meetExpectations checks if all expected node operations for a Workspace have been completed.
func (r *WorkspaceReconciler) meetExpectations(workspaceId string) bool {
	r.RLock()
	defer r.RUnlock()
	nodeNames, ok := r.expectations[workspaceId]
	return !ok || nodeNames.Len() == 0
}

// removeExpectations removes the expectations for a Workspace.
func (r *WorkspaceReconciler) removeExpectations(workspaceId string) {
	r.Lock()
	defer r.Unlock()
	delete(r.expectations, workspaceId)
}

// settle drops a node from a Workspace's outstanding expectations and reports whether it was
// there to drop. Callers hold the write lock.
//
// An emptied entry is deleted rather than left as an empty set: observeNodeForAll walks this
// map on every admin Node event, and NodeK8sReconciler writes those nodes every few seconds.
func (r *WorkspaceReconciler) settle(workspaceId, nodeName string) bool {
	leftNodeNames, ok := r.expectations[workspaceId]
	if !ok || !leftNodeNames.Has(nodeName) {
		return false
	}
	// sets.Set is a map, so this lands in the stored set; there is nothing to write back.
	leftNodeNames.Delete(nodeName)
	if leftNodeNames.Len() == 0 {
		delete(r.expectations, workspaceId)
	}
	return true
}

// observeNode marks a node operation as completed for a Workspace.
func (r *WorkspaceReconciler) observeNode(workspaceId, nodeName string) {
	r.Lock()
	defer r.Unlock()
	r.settle(workspaceId, nodeName)
}

// observeNodeForAll marks a node operation as completed for every Workspace waiting on that
// node, and returns the ids it settled so they can be re-queued.
func (r *WorkspaceReconciler) observeNodeForAll(nodeName string) []string {
	r.Lock()
	defer r.Unlock()
	var workspaceIds []string
	for workspaceId := range r.expectations {
		if r.settle(workspaceId, nodeName) {
			workspaceIds = append(workspaceIds, workspaceId)
		}
	}
	return workspaceIds
}

// reservedNodes returns the nodes another Workspace has already asked for by name, so that
// automatic scaling leaves them alone. An explicit request is something a user made and the
// admission webhook accepted; letting a scale-up take the node out from under it turns that
// into a failure the user cannot do anything about.
//
// The claim lives in the nodes-action annotation, which is cleared only once the binding has
// been written, so it covers the whole window. Nodes already bound need no reservation -- the
// caller's own spec.workspace filter catches those.
//
// Not filtered by cluster: the candidate list this feeds is already restricted to one
// cluster's nodes by label selector, and admin Node names are unique across the admin plane,
// so a name from another cluster matches nothing.
func (r *WorkspaceReconciler) reservedNodes(ctx context.Context, workspaceId string) (sets.Set, error) {
	workspaceList := &v1.WorkspaceList{}
	if err := r.List(ctx, workspaceList); err != nil {
		// Skipping a round of scaling up is the cheap failure here. Returning what was read
		// so far would do the opposite of what it reads like: every node claimed by an
		// annotation we could not see becomes fair game.
		return nil, err
	}
	reserved := sets.NewSet()
	for i := range workspaceList.Items {
		other := &workspaceList.Items[i]
		if other.Name == workspaceId {
			continue
		}
		// A workspace under deletion never processes its annotation -- Reconcile hands it
		// straight to delete() -- so its claims are abandoned, not pending. Left in, they
		// would reserve those nodes against everyone else for good.
		if !other.GetDeletionTimestamp().IsZero() {
			continue
		}
		for nodeName, action := range parseNodesAction(other) {
			if action != v1.NodeActionRemove {
				reserved.Insert(nodeName)
			}
		}
	}
	return reserved, nil
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
	if workspace.Spec.NodeFlavor == "" {
		return ctrlruntime.Result{}, nil
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

// scaleDown handles scaling down a Workspace by unbinding nodes.
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

// scaleUp handles scaling up a Workspace by binding new nodes.
func (r *WorkspaceReconciler) scaleUp(ctx context.Context,
	workspace *v1.Workspace, k8sClients *commonclient.ClientFactory, count int) (ctrlruntime.Result, error) {
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

// getNodesForScalingUp retrieves available nodes for scaling up a Workspace.
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
	candidates := make([]int, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		n := &nodeList.Items[i]
		if !n.IsMachineReady() || !n.IsManaged() {
			continue
		}
		if n.GetSpecWorkspace() != "" || v1.GetWorkspaceId(n) != "" {
			continue
		}
		if v1.GetNodeFlavorId(n) != workspace.Spec.NodeFlavor {
			continue
		}
		candidates = append(candidates, i)
	}
	// After the filtering, not before: scaling up re-queues for as long as it is short of
	// nodes, and on the rounds where there is nothing to take this saves a full list of every
	// Workspace plus a parse of each one's annotation.
	reserved := sets.NewSet()
	if len(candidates) > 0 {
		var err error
		if reserved, err = r.reservedNodes(ctx, workspace.Name); err != nil {
			klog.ErrorS(err, "failed to list workspaces, skipping this round of scaling up")
			return nil, err
		}
	}
	k8sNodes := make([]*corev1.Node, 0, len(candidates))
	adminNodeMap := make(map[string]*v1.Node)
	for _, i := range candidates {
		n := &nodeList.Items[i]
		if reserved.Has(n.Name) {
			klog.V(4).Infof("skip node %s for scaling up %s, another workspace asked for it",
				n.Name, workspace.Name)
			continue
		}
		k8sNode, err := getNodeByInformer(ctx, k8sClients, n.GetK8sNodeName())
		if err != nil {
			klog.ErrorS(err, "failed to get k8sNode")
			continue
		}
		adminNodeMap[k8sNode.Name] = n
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

// sortNodesForScalingUp sorts nodes based on priority for scaling up operations.
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

// syncWorkspace synchronizes the status of a Workspace with its bound nodes.
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
	nf := &v1.NodeFlavor{}
	err := r.Get(ctx, client.ObjectKey{Name: workspace.Spec.NodeFlavor}, nf)
	if err != nil {
		return err
	}

	nodes, err := commonnodes.GetNodesOfWorkspaces(ctx, r.Client, []string{workspace.Name}, commonnodes.FilterDeletingNode)
	if err != nil {
		return err
	}
	var availReplica, abnormalReplica int
	var totalResources, availResources, abnormalResources corev1.ResourceList
	for _, node := range nodes {
		if v1.GetNodeFlavorId(&node) != workspace.Spec.NodeFlavor {
			continue
		}
		if node.IsAvailable(false) {
			availResources = quantity.AddResource(availResources, node.Status.Resources)
			availReplica++
		} else {
			abnormalResources = quantity.AddResource(abnormalResources, nf.ToResourceList(commonconfig.GetRdmaName()))
			abnormalReplica++
		}
	}
	totalResources = quantity.AddResource(availResources, abnormalResources)
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
	if !quantity.Equal(abnormalResources, workspace.Status.AbnormalResources) {
		workspace.Status.AbnormalResources = abnormalResources
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

// parseNodesAction reads the nodes-action annotation off a Workspace. An unparseable value
// reads as no request at all -- the annotation is the only record of what was asked for, and
// nothing can be done with one that cannot be read.
//
// Silent on a bad value by design. Most callers read other workspaces' annotations on a hot
// path and would report the same broken value every round without being able to do anything
// about it; the workspace whose request it is logs it once, in processNodesAction, and clears
// it in the same round.
func parseNodesAction(workspace *v1.Workspace) map[string]string {
	raw := v1.GetWorkspaceNodesAction(workspace)
	if raw == "" {
		return nil
	}
	var actions map[string]string
	if err := json.Unmarshal([]byte(raw), &actions); err != nil {
		return nil
	}
	return actions
}

// processNodesAction processes node binding/unbinding actions for a Workspace.
//
// Every entry ends in one of three ways: it is applied, it is already true and needs no
// write, or it is refused and dropped. Nothing is left pending across reconciles on a
// judgement that will read the same way next time -- the annotation is what tells the rest
// of the controller a request is still in flight, and an entry that can never be applied
// would keep saying so forever, freezing this workspace's scaling behind a request that has
// already failed.
func (r *WorkspaceReconciler) processNodesAction(ctx context.Context, workspace *v1.Workspace) (bool, error) {
	actions := parseNodesAction(workspace)
	if len(actions) == 0 {
		if raw := v1.GetWorkspaceNodesAction(workspace); raw != "" {
			klog.Errorf("workspace %s: unusable nodes action %q. dropping it", workspace.Name, raw)
		}
		return false, r.removeNodesAction(ctx, workspace)
	}
	klog.Infof("process node action: %v", actions)

	newActions := make(map[string]string)
	adminNodes := make([]*v1.Node, 0, len(actions))
	refusals := make(map[string]string, len(actions))
	for key, val := range actions {
		node := &v1.Node{}
		// Uncached, like the read in updateSingleNodeBinding and for the same reason: a
		// refusal decided here is acted on rather than retried, so it must not be decided
		// against a cache that has not caught up with a release that already happened.
		if err := r.apiReader.Get(ctx, client.ObjectKey{Name: key}, node); err != nil {
			if apierrors.IsNotFound(err) {
				// A refusal, not a skip. The mutating webhook counted this entry into
				// Spec.Replica when it accepted the request, and skipping it here would leave
				// that count behind with nothing to spend it on: the annotation goes, the
				// replica stays, and the next sync buys a different machine to fill the gap --
				// the explicit bind quietly becoming the automatic scale-up this refuses.
				refusals[key] = "it no longer exists"
				continue
			}
			return false, err
		}
		target := workspace.Name
		if val == v1.NodeActionRemove {
			target = ""
		}
		verdict, reason := judgeNodeBinding(node, target, workspace.Name)
		switch verdict {
		case bindSettled:
			// Spec already reads the target; label sync is handled elsewhere. Not a refusal:
			// Spec.Replica must stay, because the mutating webhook skipped this entry too.
			continue
		case bindRefused:
			refusals[key] = reason
			continue
		}
		newActions[node.Name] = target
		adminNodes = append(adminNodes, node)
	}
	if len(refusals) > 0 {
		if err := r.dropRefusedActions(ctx, workspace, actions, refusals); err != nil {
			return false, err
		}
	}
	if len(adminNodes) == 0 {
		// Nothing is pending any more, whether the entries were applied already or withdrawn.
		// Saying otherwise makes processWorkspace stop for a requeue that may never be asked
		// for: a withdrawal need not change any spec field, and neither predicate on this
		// controller fires for an annotation that only goes away.
		return false, r.removeNodesAction(ctx, workspace)
	}
	if err := r.updateNodesBinding(ctx, workspace, adminNodes, newActions); err != nil {
		return false, err
	}
	return true, nil
}

// dropRefusedActions takes the refused entries back out of the request.
//
// A refusal here is permanent in the only sense that matters to a retry: the node belongs to
// somebody else, or is not in a state a workspace may hold it in, and no amount of looping by
// this controller changes that. So the request is withdrawn rather than repeated. The reason
// goes on the Workspace where whoever asked can read it; the controller log is not somewhere
// a workspace's owner can look.
//
// Spec.Replica goes back in the same write. The mutating webhook counted each of these adds
// in when it accepted the request, and leaving that count standing would turn a refused
// explicit bind into an automatic scale-up onto some other node -- a machine the caller did
// not ask for, in place of the one they named. commonnodes.WithdrawnReplica is the arithmetic,
// and it is the same function both webhooks use to recognise this write, so the number the
// controller writes and the number admission expects cannot drift apart.
//
// Here rather than in the webhook, and in one patch rather than two. Mutating admission runs
// before validating admission and hands it the mutated object, so a webhook that moved
// Spec.Replica itself would leave the validator judging a different object than the one that
// arrived -- including judging this withdrawal's own recognition against a replica count the
// writer never sent. The mutator therefore touches nothing on this path, and both webhooks
// read the controller's bytes.
//
// One patch is also what makes the refund happen exactly once under concurrency. Annotation
// and replica move together or not at all, and the optimistic lock below ties both to the
// resourceVersion this decision was made from: a competing write loses the patch entirely
// rather than half of it, and the requeue that follows re-reads a request whose refused
// entries are already gone, so there is nothing left to refund a second time.
//
// What lands here is only ever the race. A node already spoken for at admission time is
// refused there, by the mutating webhook, and never reaches this controller; what reaches it
// is a node that was free when the request was admitted and was taken before it could bind.
func (r *WorkspaceReconciler) dropRefusedActions(ctx context.Context,
	workspace *v1.Workspace, actions, refusals map[string]string) error {
	left := make(map[string]string, len(actions))
	reasons := make([]string, 0, len(refusals))
	for key, val := range actions {
		reason, refused := refusals[key]
		if !refused {
			left[key] = val
			continue
		}
		reasons = append(reasons, fmt.Sprintf("%s: %s", key, reason))
	}
	sort.Strings(reasons)
	klog.Errorf("workspace %s: dropping refused node actions: %s", workspace.Name, strings.Join(reasons, "; "))

	// Locked, like the ownership write on the Node side and for the same reason: this rewrites
	// the request as a whole, from a copy that came out of the cache, so a version of it this
	// read never saw would be overwritten rather than merged with. A conflict here costs a
	// requeue, and the reconcile that follows judges the request that actually exists.
	patch := client.MergeFromWithOptions(workspace.DeepCopy(), client.MergeFromWithOptimisticLock{})
	// Not optional, and not only for the reader: this annotation appearing is what both
	// webhooks take as the mark of a withdrawal, and without it they judge this patch as its
	// author shrinking a request the controller is still working through -- and reject it.
	v1.SetAnnotation(workspace, v1.WorkspaceNodesActionError, strings.Join(reasons, "; "))
	workspace.Spec.Replica = commonnodes.WithdrawnReplica(workspace.Spec.Replica, actions, left)
	if len(left) == 0 {
		v1.RemoveAnnotation(workspace, v1.WorkspaceNodesAction)
		v1.RemoveAnnotation(workspace, v1.WorkspaceForcedAction)
	} else {
		raw, err := json.Marshal(left)
		if err != nil {
			return err
		}
		v1.SetAnnotation(workspace, v1.WorkspaceNodesAction, string(raw))
	}
	return r.Patch(ctx, workspace, patch)
}

// removeNodesAction removes the node action annotation from a Workspace.
func (r *WorkspaceReconciler) removeNodesAction(ctx context.Context, workspace *v1.Workspace) error {
	if v1.GetWorkspaceNodesAction(workspace) == "" {
		return nil
	}
	patch := client.MergeFrom(workspace.DeepCopy())
	v1.RemoveAnnotation(workspace, v1.WorkspaceNodesAction)
	v1.RemoveAnnotation(workspace, v1.WorkspaceForcedAction)
	if err := r.Patch(ctx, workspace, patch); err != nil {
		return err
	}
	return nil
}

// updateNodesBinding updates the binding of nodes to a Workspace.
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
		target := targets[n.Name]
		ok, err := r.updateSingleNodeBinding(ctx, workspace.Name, n, target)
		// An expectation waits for the workspace label to make the round trip through the
		// data plane, and handleNodeEvent only credits it on a *change* of that label. A node
		// whose label already reads the target has nothing left to wait for, so waiting anyway
		// wedges the workspace: no admin Node write is coming to carry a transition that has
		// already happened. n is the object updateSingleNodeBinding read through, so its
		// labels are fresh.
		if !ok || err != nil || v1.GetWorkspaceId(n) == target {
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

// nodeBindVerdict is what the single-owner rule says about one requested change to a node's
// binding.
type nodeBindVerdict int

const (
	// bindProceed: the change is allowed, and the node does not read that way yet.
	bindProceed nodeBindVerdict = iota
	// bindSettled: spec already says what the request asks for; there is nothing to write.
	bindSettled
	// bindRefused: applying it would take a node away from the workspace that holds it.
	bindRefused
)

// judgeNodeBinding applies the admin plane's rules about node ownership -- a node belongs to
// at most one workspace, only its owner may release it, and a workspace may only take a node
// that has finished onboarding and is not on its way out -- to a single change taking `node`
// to `target` on behalf of `requester`.
//
// WorkspaceValidator.validateNodesAction already answers this at admission, which is where a
// user can see the answer. This is the copy that runs at the write, against state read a
// moment earlier, and it exists for the window between the two: a node can be taken by
// somebody else, or lose its managed state, after admission said yes.
//
// Every refusal here is one that reads the same way on the next reconcile, so callers act on
// it rather than retrying. Changing that -- adding a reason that clears on its own -- means
// giving the caller a way to tell the two apart first.
func judgeNodeBinding(node *v1.Node, target, requester string) (nodeBindVerdict, string) {
	current := node.GetSpecWorkspace()
	if current == target {
		return bindSettled, ""
	}
	if target == "" {
		// An unbind, and current is not "" or the case above would have caught it. A node
		// being deleted still goes through: delete() collects deleting nodes too, and
		// refusing here would let the finalizer come off with spec.workspace still naming a
		// Workspace that no longer exists -- which no later bind can clear, because only the
		// owner may release it.
		if current != requester {
			return bindRefused, fmt.Sprintf("it is bound to %s, which is not the workspace asking", current)
		}
		return bindProceed, ""
	}
	if target != requester {
		return bindRefused, fmt.Sprintf("%s may not bind it to %s", requester, target)
	}
	if current != "" {
		return bindRefused, fmt.Sprintf("it is already bound to %s", current)
	}
	// Binding a node on its way out hands the workspace a reference to something about to
	// stop existing.
	if !node.GetDeletionTimestamp().IsZero() {
		return bindRefused, "it is being deleted"
	}
	// Onboarding has to finish before a node can be handed to a workspace. Admission checks
	// this too, and this is the half that covers a node dropping out of Managed afterwards.
	if !node.IsManaged() {
		return bindRefused, fmt.Sprintf("it is not managed (phase %q, cluster %q)",
			node.Status.ClusterStatus.Phase, v1.GetClusterId(node))
	}
	return bindProceed, ""
}

// updateSingleNodeBinding updates the binding of a single node to a Workspace.
//
// This is the only place a Workspace's claim on a node is written, so it is where the
// single-owner rule is enforced: read the node fresh, judge, and patch under an optimistic
// lock so the judgement and the write cannot be separated. Losing the lock means somebody
// wrote first -- read again and judge again, which may well say the node now belongs to
// someone else.
//
// One other place clears spec.workspace: NodeReconciler.cleanupNodeAfterUnmanage, when a node
// leaves the fleet. That release is deliberately not subject to the rule -- an unmanaged node
// is gone whatever its owner thinks -- and deliberately leaves Spec.Replica alone, so the
// workspace scales a replacement in. See the comment there.
//
// The read is uncached and goes into a fresh object. Uncached because the manager's cache is
// routinely behind here -- NodeK8sReconciler mirrors the data plane's conditions onto every
// admin Node every few seconds -- so a cached copy would judge stale state and carry a
// resourceVersion the lock rejects. Fresh object because client.Get decodes into the target
// without zeroing it first, and Spec.Workspace is an omitempty pointer: re-reading a node
// that is not bound leaves the previous attempt's value in place, and the retry then judges
// its own writing as settled. (The fake client used in tests does zero the target, so this
// only ever shows up against a real apiserver.)
//
// A refusal is returned as an error rather than swallowed, and the caller keeps the
// nodes-action annotation, so the request stays visible. It is not retried indefinitely:
// judgeNodeBinding only refuses for reasons that read the same way next time, and the next
// pass through processNodesAction reaches the same verdict against its own fresh read and
// withdraws the entry, returning the Spec.Replica the mutating webhook added for it. What
// this call must not do is drop the request quietly -- that would strand the replica and turn
// a refused explicit bind into an automatic scale-up onto some other node.
func (r *WorkspaceReconciler) updateSingleNodeBinding(ctx context.Context,
	requester string, node *v1.Node, target string) (bool, error) {
	action := "bind"
	if target == "" {
		action = "unbind"
	}
	updated := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		updated = false
		fresh := &v1.Node{}
		if getErr := r.apiReader.Get(ctx, client.ObjectKeyFromObject(node), fresh); getErr != nil {
			// A node that no longer exists is an answer, not a failure: there is nothing left
			// for the workspace to hold a reference to. Anything else ends the retry as an
			// error -- RetryOnConflict stops on a non-conflict, and there is nothing to judge.
			if apierrors.IsNotFound(getErr) {
				klog.Warningf("skip %s of node %s requested by %s: it no longer exists",
					action, node.Name, requester)
				return nil
			}
			// Counted, so that a read that stops working -- throttling, an RBAC change --
			// shows up as binding failing rather than as binding traffic going to zero.
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "failed").Inc()
			return getErr
		}
		*node = *fresh
		verdict, reason := judgeNodeBinding(node, target, requester)
		switch verdict {
		case bindSettled:
			// Already where it should be. Note this covers an unbind against a nil
			// Spec.Workspace, which reads as "". Writing "" over the nil to make the release
			// explicit would return updated=true and stop the caller settling the
			// expectation, while changing nothing any reader can see.
			return nil
		case bindRefused:
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "refused").Inc()
			return fmt.Errorf("cannot %s node %s for %s: %s", action, node.Name, requester, reason)
		}
		patch := client.MergeFromWithOptions(node.DeepCopy(), client.MergeFromWithOptimisticLock{})
		node.Spec.Workspace = pointer.String(target)
		patchErr := r.Patch(ctx, node, patch)
		if patchErr == nil {
			updated = true
			klog.Infof("updateSingleNodeBinding, node: %s, target: %s", node.Name, target)
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "success").Inc()
			return nil
		}
		if !apierrors.IsConflict(patchErr) {
			klog.ErrorS(patchErr, "failed to update node", "target", target)
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "failed").Inc()
			return patchErr
		}
		klog.Warningf("conflict while trying to %s node %s, target: %s. it will be retried",
			action, node.Name, target)
		return patchErr
	})
	if err != nil && apierrors.IsConflict(err) {
		// Out of attempts. Counted once rather than once per attempt: a counter that climbs
		// with the retry budget measures the budget, not the contention.
		rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "conflict").Inc()
		klog.Warningf("gave up trying to %s node %s after repeated conflicts", action, node.Name)
	}
	return updated, err
}

// resetWorkspaceStatus resets the status of a Workspace when no node flavor is specified.
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

// buildTargetList builds a map of node names to their target Workspace names.
func buildTargetList(nodes []*v1.Node, target string) map[string]string {
	results := make(map[string]string)
	for _, n := range nodes {
		results[n.Name] = target
	}
	return results
}
