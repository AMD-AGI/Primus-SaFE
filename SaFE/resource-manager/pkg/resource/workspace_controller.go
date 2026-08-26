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
	"k8s.io/client-go/tools/record"
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
	// for the one place a stale read would defeat the point of reading at all: re-checking
	// ownership after losing an optimistic-lock conflict, where the whole question is what
	// the writer that just beat us stored.
	apiReader client.Reader
	// recorder puts the refusals on the Workspace itself. Every one of them is a request a
	// user made through the API that this controller then declined -- the REST call has long
	// since returned 200, the annotation carrying it is cleared on the way out, and a line in
	// the resource-manager log is not somewhere a user can look.
	recorder record.EventRecorder
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

// workspaceClusterIndex indexes Workspaces by the cluster they belong to, so reservedNodes can
// ask the cache for one cluster's workspaces instead of receiving every workspace in the admin
// plane and discarding all but one cluster's worth on the way out.
const workspaceClusterIndex = "spec.cluster"

// indexWorkspaceByCluster is the index function behind workspaceClusterIndex. It is a named
// function because tests have to register the same one on their fake client: an unregistered
// index there is an error, not a silent fall back to listing everything.
func indexWorkspaceByCluster(object client.Object) []string {
	workspace, ok := object.(*v1.Workspace)
	if !ok {
		return nil
	}
	// The empty cluster is indexed rather than skipped. The loop this replaced compared the
	// field directly, so a workspace with no cluster matched another with no cluster, and an
	// index that dropped them would quietly change that.
	return []string{workspace.Spec.Cluster}
}

// SetupWorkspaceController initializes and registers the WorkspaceReconciler with the controller manager.
func SetupWorkspaceController(ctx context.Context, mgr manager.Manager, opt *WorkspaceReconcilerOption) error {
	if err := mgr.GetFieldIndexer().IndexField(ctx,
		&v1.Workspace{}, workspaceClusterIndex, indexWorkspaceByCluster); err != nil {
		return fmt.Errorf("failed to setup field indexer for workspace cluster: %v", err)
	}
	baseReconciler, err := newClusterBaseReconciler(mgr)
	if err != nil {
		return err
	}
	r := &WorkspaceReconciler{
		ClusterBaseReconciler: baseReconciler,
		apiReader:             mgr.GetAPIReader(),
		recorder:              mgr.GetEventRecorderFor("workspace-controller"),
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
	// observe settles the node against every workspace waiting on it, not just the one the
	// node currently points at. Ownership can change hands between the moment an expectation
	// is registered and the moment the event arrives, and the event only carries the new (or
	// an empty) workspace id, so crediting the expectation by that id alone would leave the
	// original workspace waiting on a node it no longer owns, forever.
	observe := func(q v1.RequestWorkQueue, nodeName string) {
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
			observe(q, node.Name)
			enqueue(q, v1.GetWorkspaceId(node))
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 {
				return
			}
			if v1.GetWorkspaceId(oldNode) != v1.GetWorkspaceId(newNode) {
				observe(q, newNode.Name)
				enqueue(q, v1.GetWorkspaceId(oldNode))
				enqueue(q, v1.GetWorkspaceId(newNode))
			} else if isRelevantFieldChanged(oldNode, newNode) {
				enqueue(q, v1.GetWorkspaceId(newNode))
			}
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			node, ok := evt.Object.(*v1.Node)
			if !ok {
				return
			}
			observe(q, node.Name)
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
// An emptied entry is deleted rather than left as an empty set. `meetExpectations` reads the
// two the same way, so nothing downstream can tell them apart -- but observeNodeForAll walks
// this map under the write lock on every admin Node event, and NodeK8sReconciler mirrors
// kubelet's conditions onto those nodes every few seconds. Kept, the empties accumulate one
// per workspace ever scaled, and every heartbeat in the cluster pays to walk past all of
// them. Only the delete path ever removed an entry, and a workspace that is merely running
// never reaches it.
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

// observeNodeForAll marks a node operation as completed for every Workspace that is waiting
// on that node, and returns the ids of the workspaces it settled so they can be re-queued.
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

// reservedNodes returns the nodes another Workspace is already laying claim to: nodes with a
// binding still in flight, and nodes named by a nodes-action annotation that has not been
// processed yet. Explicit bindings are what users asked for, so they take precedence over
// automatic scaling and scale-up must leave these nodes alone.
func (r *WorkspaceReconciler) reservedNodes(ctx context.Context, workspace *v1.Workspace) (sets.Set, error) {
	workspaceId := workspace.Name
	reserved := sets.NewSet()

	workspaceList := &v1.WorkspaceList{}
	if err := r.List(ctx, workspaceList,
		client.MatchingFields{workspaceClusterIndex: workspace.Spec.Cluster}); err != nil {
		// Better to skip a round of scaling up than to race an explicit binding. Returning
		// the partial set instead would have done the opposite of what that reads like:
		// every node claimed by an annotation we could not read becomes fair game.
		klog.ErrorS(err, "failed to list workspaces, skipping this round of scaling up")
		return nil, err
	}
	deleting := sets.NewSet()
	for i := range workspaceList.Items {
		other := &workspaceList.Items[i]
		if other.Name == workspaceId {
			continue
		}
		// A workspace under deletion never processes its annotation -- Reconcile hands it
		// straight to delete() -- so its claims are not pending, they are abandoned. Left
		// in, they would reserve those nodes against every other workspace forever.
		if !other.GetDeletionTimestamp().IsZero() {
			deleting.Insert(other.Name)
			continue
		}
		raw := v1.GetWorkspaceNodesAction(other)
		if raw == "" {
			continue
		}
		var actions map[string]string
		if err := json.Unmarshal([]byte(raw), &actions); err != nil {
			continue
		}
		for nodeName, action := range actions {
			if action != v1.NodeActionRemove {
				reserved.Insert(nodeName)
			}
		}
	}

	r.RLock()
	for id, nodeNames := range r.expectations {
		if id == workspaceId {
			continue
		}
		// Same reasoning as the annotation scan, and it has to be applied here too or the
		// two disagree: a workspace being deleted holds unbinds, not binds, and reserving
		// the nodes it is releasing keeps them out of everyone's reach for as long as the
		// entry survives -- which, if its deletion is itself stuck behind an expectation
		// that never settles, is the life of the process.
		//
		// `deleting` only covers this cluster, because the list it is built from does. That
		// makes no difference for the same reason the loop below is not cluster-filtered:
		// another cluster's names match nothing in this cluster's candidate list.
		if deleting.Has(id) {
			continue
		}
		// Not filtered by cluster, unlike the list above: an expectation is keyed by
		// workspace alone, and looking each one's cluster up costs more than carrying the
		// few node names an in-flight bind can hold. It cannot over-reserve either way --
		// admin Node names are unique across the admin plane, and the candidate list is
		// already restricted to this cluster, so a name from elsewhere matches nothing.
		//
		// A workspace absent from the list altogether stays reserved: the list is served
		// from the manager's cache, so absence is as likely to mean "created a moment ago"
		// as "gone", and the safe reading of an in-flight bind we cannot attribute is that
		// somebody owns it.
		reserved.Insert(nodeNames.UnsortedList()...)
	}
	r.RUnlock()
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
		isUpdated, err := r.processNodesAction(ctx, workspace, k8sClients)
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
	reserved, err := r.reservedNodes(ctx, workspace)
	if err != nil {
		return nil, err
	}
	k8sNodes := make([]*corev1.Node, 0, len(nodeList.Items))
	adminNodeMap := make(map[string]*v1.Node)
	for i, n := range nodeList.Items {
		if !n.IsMachineReady() || !n.IsManaged() {
			continue
		}
		if n.GetSpecWorkspace() != "" || v1.GetWorkspaceId(&n) != "" {
			continue
		}
		if reserved.Has(n.Name) {
			klog.V(4).Infof("skip node %s for scaling up %s, another workspace is processing it", n.Name, workspace.Name)
			continue
		}
		if v1.GetNodeFlavorId(&n) != workspace.Spec.NodeFlavor {
			continue
		}
		k8sNode, err := getCachedDataPlaneNode(ctx, k8sClients, n.GetK8sNodeName())
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

// k8sNodeUnavailableReason says why the node cannot take a binding, or "" if it can. The
// string is user-facing: it goes into the Workspace event that explains the dropped request.
//
// A transient failure to answer counts as available. Refusing is permanent here -- the add is
// dropped and the annotation carrying it cleared -- so it must only happen on a definite no.
//
// Which is also why this one reads the apiserver rather than the informer cache, unlike the
// scan in getNodesForScalingUp. A node the cache has not caught up on -- one that just joined,
// or one whose watch dropped -- would make a user's binding request disappear.
//
// The definite noes are kept apart, because they send whoever reads the event to different
// places. An empty k8s node name is about the admin plane not knowing what to look for yet,
// because neither status.machineStatus.hostName nor spec.hostname is set. "Not in the cluster"
// is about the data plane not having the node at all. "Being deleted" is about it having it
// for a little longer -- reporting that one as either of the others sends the reader looking
// for a registration problem that is not there.
//
// Deleting counts as a definite no for the same reason updateSingleNodeBinding refuses one:
// the bind would hand this workspace a reference to something that is on its way out, and the
// admin Node follows the k8s Node down. Checking only NotFound here caught the node one moment
// too late -- after the object was gone, but not while it was going.
func (r *WorkspaceReconciler) k8sNodeUnavailableReason(ctx context.Context,
	k8sClients *commonclient.ClientFactory, node *v1.Node) string {
	k8sNodeName := node.GetK8sNodeName()
	if k8sNodeName == "" {
		return "its k8s node name is not known yet"
	}
	k8sNode, err := getDataPlaneNode(ctx, k8sClients, k8sNodeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("its k8s node %s is not in the cluster", k8sNodeName)
		}
		klog.ErrorS(err, "failed to get k8s node, assuming it is present",
			"node", node.Name, "k8sNode", k8sNodeName)
		return ""
	}
	if !k8sNode.GetDeletionTimestamp().IsZero() {
		return fmt.Sprintf("its k8s node %s is being deleted", k8sNodeName)
	}
	return ""
}

// processNodesAction processes node binding/unbinding actions for a Workspace.
func (r *WorkspaceReconciler) processNodesAction(ctx context.Context, workspace *v1.Workspace,
	k8sClients *commonclient.ClientFactory) (bool, error) {
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
		// Uncached, for the same reason updateSingleNodeBinding re-reads: what this loop
		// decides is irreversible. A refusal drops the entry, clears the annotation and
		// leaves an event -- there is no retry. Judging that off a cache still showing a
		// release that already happened turns a valid request into a permanent no.
		if err := r.apiReader.Get(ctx, client.ObjectKey{Name: key}, node); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, err
		}
		if !node.GetDeletionTimestamp().IsZero() {
			continue
		}
		target := workspace.Name
		if val == v1.NodeActionRemove {
			target = ""
		}
		switch verdict, reason := judgeNodeBinding(node.GetSpecWorkspace(), target, workspace.Name); verdict {
		case bindSettled:
			// Desired state (spec) already reads the way the action asks for; the data
			// plane's label sync is handled elsewhere.
			continue
		case bindRefused:
			// Dropped rather than retried, so the annotation clears instead of asking again
			// forever for a change that must not happen; the node has to be released by the
			// workspace holding it first.
			klog.Warningf("skip %s of node %s requested by %s: %s",
				val, node.Name, workspace.Name, reason)
			r.recorder.Eventf(workspace, corev1.EventTypeWarning, eventNodeBindRefused,
				"cannot %s node %s: %s", val, node.Name, reason)
			continue
		}
		// The binding is only settled once the workspace label makes the round trip through
		// the data plane, so a node whose k8s Node is gone would leave this workspace
		// waiting on it forever. Drop the request; the annotation clears and the binding can
		// be asked for again once the node is back in the cluster.
		if target != "" {
			if reason := r.k8sNodeUnavailableReason(ctx, k8sClients, node); reason != "" {
				klog.Warningf("skip adding node %s to %s: %s", node.Name, workspace.Name, reason)
				r.recorder.Eventf(workspace, corev1.EventTypeWarning, eventNodeUnavailable,
					"cannot add node %s: %s", node.Name, reason)
				continue
			}
		}
		newActions[node.Name] = target
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

const (
	// eventNodeBindRefused: the single-owner rule said no.
	eventNodeBindRefused = "NodeBindRefused"
	// eventNodeUnavailable: the node cannot take a binding right now -- it is on its way out,
	// or the data plane does not have it.
	eventNodeUnavailable = "NodeUnavailable"
)

// nodeBindVerdict is what the admin plane's single-owner rule says about one requested
// change to a node's binding.
type nodeBindVerdict int

const (
	// bindProceed: the change is allowed, and the node does not read that way yet.
	bindProceed nodeBindVerdict = iota
	// bindSettled: spec already says what the request asks for; there is nothing to write.
	bindSettled
	// bindRefused: applying it would take a node away from the workspace that holds it.
	bindRefused
)

// judgeNodeBinding applies the one rule the admin plane has about node ownership -- a node
// belongs to at most one workspace, and only its owner may release it -- to a single change
// requested by `requester`, taking the node from `current` to `target`.
//
// It lives here rather than at each call site because the rule was previously written out
// three times, in three shapes, and the shapes did not agree: the bind checks all keyed on a
// non-empty target, so an unbind -- whose target is always "" -- slipped past every one of
// them, and posting {"n":"remove"} for someone else's node was a supported way to take it.
// One predicate is also the only way the unbind half gets enforced at the layer that writes,
// rather than only at the layer that happened to remember it.
func judgeNodeBinding(current, target, requester string) (nodeBindVerdict, string) {
	if current == target {
		return bindSettled, ""
	}
	if target != "" {
		if current != "" {
			return bindRefused, fmt.Sprintf("it is already bound to %s", current)
		}
		return bindProceed, ""
	}
	// An unbind, and current is not "" or the case above would have caught it.
	if current != requester {
		return bindRefused, fmt.Sprintf("it is bound to %s, which is not the workspace asking", current)
	}
	return bindProceed, ""
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
		ok, err := r.updateSingleNodeBinding(ctx, workspace, n, target)
		// An expectation is a wait for the workspace label to make the round trip through
		// the data plane, and handleNodeEvent only credits it on a *change* of that label.
		// So a node whose label already reads the target has nothing left to wait for, and
		// waiting anyway wedges the workspace forever: no admin Node write is coming to
		// carry a transition that has already happened.
		//
		// The way in is a workspace deleted while a bind is still in flight. The bind set
		// spec.workspace and NodeReconciler pushed the label down, but before syncK8sMetadata
		// copied it back up the workspace was deleted, so delete() cleared spec.workspace
		// again. Now every party agrees the node is unbound -- admin label empty, spec
		// empty -- and the only thing that could have moved the label is syncK8sMetadata,
		// which correctly refuses to copy a data-plane value that disagrees with spec. The
		// Workspace then sits in Terminating for the life of the process.
		//
		// n is the object updateSingleNodeBinding read through, so its labels are fresh.
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

// updateSingleNodeBinding updates the binding of a single node to a Workspace.
//
// Every attempt starts from an uncached read. What the callers hand over came out of the
// manager's cache, and the patch below carries its resourceVersion under an optimistic lock,
// so a cached copy is two problems at once: the ownership check runs against state that may
// already have changed, and the patch is rejected for a version that was stale before it was
// sent. Admin Nodes are not quiet objects either -- NodeK8sReconciler mirrors the data plane's
// conditions, heartbeat timestamp and all, onto every one of them every few seconds -- so
// stale-by-a-moment is the ordinary case here, not the unlucky one. One GET per node buys the
// check real state to judge and keeps the retry budget for actual contention.
//
// The optimistic lock still earns its place on top of that: it closes the window between this
// read and the patch. Losing it means somebody else got there first, which is what the retry
// is for -- read again, judge again, and the second look may well say the work is already
// done, or that the node now belongs to someone else.
//
// Settled and refused are not errors; nothing is wrong, the answer is just no. Running out of
// retries is one, deliberately: the caller settles the node's expectation on !updated, so a
// silent give-up would let a workspace deletion read "we stopped trying" as "the node is
// released" and drop its finalizer with the node still pointing at it -- a node no later
// binding can rescue, because the only workspace allowed to release it no longer exists.
// concurrent.Exec runs every node in the batch whatever any one of them returns, so the error
// costs nothing beyond the rate-limited requeue it is asking for.
func (r *WorkspaceReconciler) updateSingleNodeBinding(ctx context.Context,
	workspace *v1.Workspace, node *v1.Node, target string) (bool, error) {
	requester := workspace.Name
	action := "bind"
	if target == "" {
		action = "unbind"
	}
	updated := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		updated = false
		// A failed read ends the retry: returning a non-conflict error is how RetryOnConflict
		// is told to give up, and there is nothing left to judge against anyway.
		//
		// NotFound ends it without an error. A node that no longer exists is an answer, not a
		// failure: nothing is left for this workspace to hold a reference to, so updated=false
		// is already the honest report, and the caller settles the expectation on it either
		// way. This is not the silent give-up the doc comment above warns about -- that one
		// leaves a node still naming the workspace, and here there is no node. Reporting it as
		// an error only made concurrent.Exec fail the batch, buying a rate-limited requeue and
		// an error log every round for a request the next pass drops anyway.
		if getErr := r.apiReader.Get(ctx, client.ObjectKeyFromObject(node), node); getErr != nil {
			if apierrors.IsNotFound(getErr) {
				klog.Warningf("skip %s of node %s requested by %s: it no longer exists",
					action, node.Name, requester)
				return nil
			}
			klog.ErrorS(getErr, "failed to read node before updating its binding", "node", node.Name)
			return getErr
		}
		// The callers filter deleting nodes out before handing them over, but the read above
		// can bring one back, and the ownership rule below is only about ownership. Binding a
		// node on its way out leaves the workspace holding a reference to something that is
		// about to stop existing.
		//
		// Binds only. An unbind of a deleting node is the one thing that must still go
		// through: delete() collects every node whose spec.workspace names the workspace,
		// deleting ones included, and settles each node's expectation on !updated. Refusing
		// here answered "nothing to do", which let meetExpectations pass and the finalizer
		// come off with the node's spec.workspace still naming a Workspace that no longer
		// exists -- and no later bind can clear it, because judgeNodeBinding refuses every
		// workspace except the one that is gone. The same unrescuable state the give-up path
		// above exists to prevent, reached by the other door.
		//
		// Letting it through does not reintroduce the wedge on the other side. The unbind
		// patch touches spec, so the expectation does not settle on it -- the workspace waits
		// for the workspace label, which is not coming -- but handleNodeEvent's DeleteFunc
		// observes the node when the object finally goes, and the wait ends there. Worst case
		// the Workspace outlives the node it was holding, which is what it is supposed to do.
		if target != "" && !node.GetDeletionTimestamp().IsZero() {
			klog.Warningf("skip %s of node %s requested by %s: it is being deleted",
				action, node.Name, requester)
			r.recorder.Eventf(workspace, corev1.EventTypeWarning, eventNodeUnavailable,
				"cannot %s node %s: it is being deleted", action, node.Name)
			return nil
		}
		verdict, reason := judgeNodeBinding(node.GetSpecWorkspace(), target, requester)
		if verdict == bindSettled {
			// Already where it should be, including the case of an unbind against a nil
			// Spec.Workspace -- which reads as "" and so judges as settled. Writing "" over
			// the nil anyway, to make the release explicit in spec, is the tempting move
			// and it wedges the workspace. Nothing reads that pointer except
			// GetSpecWorkspace(), which maps nil to "", so the write changes no behaviour;
			// what it does change is that this returns updated=true, which stops the caller
			// from settling the expectation. And the write cannot settle it either --
			// handleNodeEvent only observes on a change of the workspace *label*, and this
			// touches spec alone. The expectation would then never clear, meetExpectations
			// never pass, and every later reconcile of that workspace return early,
			// including the one that would have deleted it. That is precisely the wedge
			// this path exists to avoid.
			return nil
		}
		if verdict == bindRefused {
			klog.Warningf("skip %s of node %s requested by %s: %s", action, node.Name, requester, reason)
			r.recorder.Eventf(workspace, corev1.EventTypeWarning, eventNodeBindRefused,
				"cannot %s node %s: %s", action, node.Name, reason)
			// Its own bucket rather than "conflict": this is a permanent answer about who
			// owns the node, not a race a retry could win. Folded together, the two were
			// indistinguishable -- a workspace losing a race read the same as one reaching
			// for a node that belongs to somebody else.
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "refused").Inc()
			return nil
		}
		patch := client.MergeFromWithOptions(node.DeepCopy(), client.MergeFromWithOptimisticLock{})
		node.Spec.Workspace = pointer.String(target)
		err := r.Patch(ctx, node, patch)
		if err == nil {
			updated = true
			klog.Infof("updateSingleNodeBinding, node: %s, target: %s", node.Name, target)
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "success").Inc()
			return nil
		}
		if !apierrors.IsConflict(err) {
			klog.ErrorS(err, "failed to update node", "target", target)
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "failed").Inc()
			return err
		}
		klog.Warningf("conflict while trying to %s node %s, target: %s. it will be retried", action, node.Name, target)
		return err
	})
	if err != nil && apierrors.IsConflict(err) {
		// Out of attempts. Counted once here rather than once per attempt: a counter that
		// climbs with the retry budget measures the budget, not the contention.
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
