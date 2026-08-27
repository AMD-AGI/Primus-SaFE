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
	// recorder puts what this controller decides on its own onto the Workspace. A migration
	// is carried out long after the request that started it returned 200, so a handover that
	// cannot land, or a migration given up on, has nowhere else to be reported: a line in the
	// resource-manager log is not somewhere the user who asked for it can look.
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
	// migrateTimeout bounds how long a node may sit released for a migration that never
	// completes. Past it the reservation is dropped and the node becomes an ordinary
	// unassigned node -- it does not return to the source workspace, which gave it up when
	// the migration was admitted and had its replica lowered to match.
	migrateTimeout time.Duration
}

// SetupWorkspaceController initializes and registers the WorkspaceReconciler with the controller manager.
func SetupWorkspaceController(mgr manager.Manager, opt *WorkspaceReconcilerOption) error {
	baseReconciler, err := newClusterBaseReconciler(mgr)
	if err != nil {
		return err
	}
	r := &WorkspaceReconciler{
		ClusterBaseReconciler: baseReconciler,
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
	enqueue := func(q v1.RequestWorkQueue, nodeName, workspaceId string, doObserve bool) {
		if workspaceId == "" {
			return
		}
		if doObserve {
			r.observeNode(workspaceId, nodeName)
		}
		q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: workspaceId}})
	}
	// enqueueMigration wakes both ends of a migration the node is part of.
	//
	// A migrating node's two ends are not both in its labels, and for one of the two moments
	// that matter neither is. The source workspace drives the migration and has to see the
	// node land to clear the action it is driving with -- but on landing the label goes from
	// empty to the target, so enqueueing by label wakes the target only, and the source waits
	// for its next resync before noticing that a migration which took milliseconds is over.
	// It holds the workspace's one action slot for that whole wait, and every node operation
	// a user asks of that workspace in the meantime is refused as already busy.
	//
	// The record the source is driving with names both ends, and the node still carries it in
	// the update's old object -- it is cleared by the same patch that binds the node.
	enqueueMigration := func(q v1.RequestWorkQueue, node *v1.Node) {
		info := v1.GetNodeMigrateInfo(node)
		if info == nil {
			return
		}
		enqueue(q, node.Name, info.From, false)
		enqueue(q, node.Name, info.Target, false)
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			node, ok := evt.Object.(*v1.Node)
			if !ok {
				return
			}
			enqueue(q, node.Name, v1.GetWorkspaceId(node), true)
			enqueueMigration(q, node)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 {
				return
			}
			// Both objects: the old one names the migration a landing node has just finished,
			// the new one the migration a released node is starting.
			enqueueMigration(q, oldNode)
			enqueueMigration(q, newNode)
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
	var migrating []*v1.Node
	for i, item := range nodeList.Items {
		if item.GetSpecWorkspace() == workspace.Name {
			nodes = append(nodes, &nodeList.Items[i])
			continue
		}
		// A node this workspace released and has not finished handing over is bound to
		// nobody, so the loop above does not see it -- and once the finalizer goes there is
		// no workspace left to carry the migration or to give up on it. The reservation would
		// outlive everything that could act on it.
		if info := v1.GetNodeMigrateInfo(&nodeList.Items[i]); info != nil && info.From == workspace.Name {
			migrating = append(migrating, &nodeList.Items[i])
		}
	}
	if err = r.abandonMigrations(ctx, workspace, migrating); err != nil {
		return err
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
	// Merged, not replaced. Two things bind nodes for one workspace -- the scaling loop and
	// the node action a user asked for -- and replacing drops whatever the other one is still
	// waiting on. The workspace then reads as settled while a binding is still in flight, and
	// the next scaling decision is taken on counts that have not caught up: a scale-down
	// already under way is counted as not yet done and a further node is let go.
	left, ok := r.expectations[workspaceId]
	if !ok {
		r.expectations[workspaceId] = nodeNames
		return
	}
	for nodeName := range nodeNames {
		left.Insert(nodeName)
	}
	r.expectations[workspaceId] = left
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

// observeNode marks a node operation as completed for a Workspace.
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

// migrateTimeout is how long this controller lets a crossing run before giving up on it.
//
// The node webhook stops honouring a reservation at v1.DefaultNodeMigrateTimeout, and it has
// no way to read this controller's setting. Anything longer here would leave a window where
// the migration is still being driven and the node is no longer protected, so an unset or
// longer value is brought back to the one both sides can agree on.
func (r *WorkspaceReconciler) migrateTimeout() time.Duration {
	if r.option == nil || r.option.migrateTimeout <= 0 ||
		r.option.migrateTimeout > v1.DefaultNodeMigrateTimeout {
		return v1.DefaultNodeMigrateTimeout
	}
	return r.option.migrateTimeout
}

// recordf puts an event on the workspace when a recorder is configured. Tests build the
// reconciler without one, and an unreported event must not take the reconcile down with it.
func (r *WorkspaceReconciler) recordf(workspace *v1.Workspace, eventType, reason, format string, args ...interface{}) {
	if r.recorder == nil || workspace == nil {
		return
	}
	r.recorder.Eventf(workspace, eventType, reason, format, args...)
}

// processWorkspace handles the main processing logic for a Workspace resource
// include scaling up and scaling down nodes.
func (r *WorkspaceReconciler) processWorkspace(ctx context.Context, workspace *v1.Workspace) (ctrlruntime.Result, error) {
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, workspace.Spec.Cluster)
	if err != nil || !k8sClients.IsValid() {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	// Ahead of the expectations gate on purpose. That gate waits on bindings to be observed
	// through the node labels, which the node controller writes from the data plane -- so a
	// data plane that cannot be reached holds it shut. Scaling has to wait for it, because
	// scaling decides from counts that would otherwise be stale. A node action does not: it
	// re-reads every node it names and skips the ones already where they belong. Behind the
	// gate, a migration stalled that way would never be carried on and never even time out,
	// leaving the node released and reserved with nothing driving it.
	var actionResult ctrlruntime.Result
	if v1.GetWorkspaceNodesAction(workspace) != "" {
		var isUpdated bool
		if actionResult, isUpdated, err = r.processNodesAction(ctx, workspace); err != nil || isUpdated {
			return actionResult, err
		}
	}
	if !r.meetExpectations(workspace.Name) {
		return actionResult, nil
	}
	if err = r.syncWorkspace(ctx, workspace); err != nil {
		return ctrlruntime.Result{}, err
	}
	if workspace.Spec.NodeFlavor == "" {
		// A workspace with no flavor does no scaling, but it can still be in the middle of
		// handing a node over -- it is what a workspace looks like before its first node
		// arrives -- and the requeue is the only thing bringing that back.
		return actionResult, nil
	}

	totalStatusCount := workspace.CurrentReplica()
	result := actionResult
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
	// A scaling requeue is never longer than the action's, but keep whichever comes first so
	// a pending handover is not left waiting on the slower of the two.
	if actionResult.RequeueAfter > 0 &&
		(result.RequeueAfter == 0 || actionResult.RequeueAfter < result.RequeueAfter) {
		result.RequeueAfter = actionResult.RequeueAfter
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
	k8sNodes := make([]*corev1.Node, 0, len(nodeList.Items))
	adminNodeMap := make(map[string]*v1.Node)
	for i, n := range nodeList.Items {
		if !r.isNodeEligibleForScalingUp(&nodeList.Items[i], workspace) {
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

// isNodeEligibleForScalingUp reports whether an unbound node in the workspace's cluster may
// be picked up by it. Callers have already narrowed the list to that cluster.
//
// The workspace argument is what keeps a migration from being undone by the scaling loop.
// A node released by its source workspace is unbound and of a matching flavor, which is
// every quality scale-up looks for, so between the two halves of a migration any workspace
// in the cluster short of a replica would take it -- including the source workspace itself,
// were its replica not decremented first. The node carries the target it was released for,
// and only that target may claim it.
func (r *WorkspaceReconciler) isNodeEligibleForScalingUp(node *v1.Node, workspace *v1.Workspace) bool {
	if !node.IsMachineReady() || !node.IsManaged() {
		return false
	}
	if node.GetSpecWorkspace() != "" || v1.GetWorkspaceId(node) != "" {
		return false
	}
	if v1.GetNodeFlavorId(node) != workspace.Spec.NodeFlavor {
		return false
	}
	// The target is allowed through rather than skipped over: the handover writes an add
	// action for it, but should that write be lost, scaling up is what finishes the
	// migration instead of leaving the node parked until the migration times out.
	//
	// A stale reservation is ignored, because this is the only place a reservation can do
	// harm on its own and the things that clear one can all go away: the source workspace
	// can be deleted mid-migration, and a node can be taken out of the cluster and brought
	// back still carrying the annotation. Without the age check any of those parks a node
	// for good, and nothing left in the system would explain why.
	if info := v1.GetNodeMigrateInfo(node); info != nil && info.Target != workspace.Name &&
		!v1.IsNodeMigrationExpired(info, r.migrateTimeout()) {
		return false
	}
	return true
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

// processNodesAction processes node binding/unbinding actions for a Workspace.
//
// A migration is the one action that does not finish in a single pass. It is carried out from
// the source workspace, whose annotation stays put for the whole crossing and is the record
// that one is under way: release the node here, hand it to the target, and clear the
// annotation once it has landed. Every pass re-reads the nodes and works out which of those
// three it is looking at, so an interrupted migration resumes rather than restarts.
func (r *WorkspaceReconciler) processNodesAction(ctx context.Context,
	workspace *v1.Workspace) (ctrlruntime.Result, bool, error) {
	var actions map[string]string
	if err := json.Unmarshal([]byte(v1.GetWorkspaceNodesAction(workspace)), &actions); err != nil || len(actions) == 0 {
		if err != nil {
			klog.ErrorS(err, "failed to unmarshal json. skip it",
				"data", v1.GetWorkspaceNodesAction(workspace))
		}
		return ctrlruntime.Result{}, false, r.removeNodesAction(ctx, workspace)
	}
	klog.Infof("process node action: %v", actions)

	newActions := make(map[string]nodeBinding)
	adminNodes := make([]*v1.Node, 0, len(actions))
	// Nodes already released and waiting for the target to take them, by target, and nodes
	// whose migration has to be given up on.
	pendingHandover := make(map[string][]*v1.Node)
	var abandoned []*v1.Node
	for key, val := range actions {
		node := &v1.Node{}
		if err := r.Get(ctx, client.ObjectKey{Name: key}, node); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return ctrlruntime.Result{}, false, err
		}
		if !node.GetDeletionTimestamp().IsZero() {
			continue
		}
		if target, ok := v1.ParseMigrateAction(val); ok {
			switch binding, migration := r.classifyMigration(node, workspace.Name, target); binding {
			case migrationRelease:
				newActions[node.Name] = nodeBinding{migration: migration}
				adminNodes = append(adminNodes, node)
			case migrationPending:
				pendingHandover[target] = append(pendingHandover[target], node)
			case migrationAbandoned:
				abandoned = append(abandoned, node)
			case migrationDone:
			}
			continue
		}
		if val == v1.NodeActionRemove {
			// Desired state (spec) already unbound; label sync is handled elsewhere.
			if node.GetSpecWorkspace() == "" {
				continue
			}
			newActions[node.Name] = nodeBinding{}
		} else if val == v1.NodeActionAdd {
			// Desired state (spec) already targets this workspace; no further action here.
			if node.GetSpecWorkspace() == workspace.Name {
				continue
			}
			newActions[node.Name] = nodeBinding{workspace: workspace.Name}
		} else {
			// Anything else is not an action this understands, and the one thing not to do
			// with it is guess. Treating it as an add -- which is what falling through to the
			// branch above amounts to -- binds the node with none of the replica accounting
			// that a real add gets admitted with, so a migrate that lost its target on the
			// way in would quietly claim the node instead of being refused.
			klog.Errorf("unknown node action(%s) for node(%s) on workspace(%s), ignoring it",
				val, node.Name, workspace.Name)
			r.recordf(workspace, corev1.EventTypeWarning, "UnknownNodeAction",
				"ignoring unknown action(%s) for node(%s)", val, node.Name)
			continue
		}
		adminNodes = append(adminNodes, node)
	}
	if len(abandoned) > 0 {
		// Dropping the reservations first: the annotation is what brings this code back, so
		// clearing it while a node still carries a reservation nobody drives would leave that
		// node reserved for a workspace that is not coming for it.
		if err := r.abandonMigrations(ctx, workspace, abandoned); err != nil {
			return ctrlruntime.Result{}, false, err
		}
	}
	if len(adminNodes) > 0 {
		if err := r.updateNodesBinding(ctx, workspace, adminNodes, newActions); err != nil {
			return ctrlruntime.Result{}, false, err
		}
		return ctrlruntime.Result{}, true, nil
	}
	if len(pendingHandover) > 0 {
		// The requeue is the backstop for a handover the node events cannot carry -- a target
		// busy with another action changes nothing about the node, so nothing else brings us
		// back to ask again.
		//
		// Reported as not updated, so the rest of the reconcile still runs. A crossing is
		// normally over in well under a second, but a handover that cannot land holds this
		// state for as long as the timeout allows, and the workspace still has to have its
		// status refreshed and its own replicas kept up in the meantime.
		r.handOverMigrations(ctx, workspace, pendingHandover)
		return ctrlruntime.Result{RequeueAfter: r.option.nodeWait}, false, nil
	}
	return ctrlruntime.Result{}, false, r.removeNodesAction(ctx, workspace)
}

// migrationState is which part of a migration a node is in the middle of.
type migrationState int

const (
	// migrationRelease: the node is still bound to the source and has to be let go.
	migrationRelease migrationState = iota
	// migrationPending: released, waiting for the target to take it.
	migrationPending
	// migrationDone: the node is in the target workspace.
	migrationDone
	// migrationAbandoned: the migration cannot be completed and has to be given up on.
	migrationAbandoned
)

// classifyMigration works out where in a migration a node is, and for a node about to be
// released, the crossing to stamp on it.
func (r *WorkspaceReconciler) classifyMigration(node *v1.Node,
	source, target string) (migrationState, *v1.NodeMigrateInfo) {
	if node.GetSpecWorkspace() == target {
		return migrationDone, nil
	}
	if node.GetSpecWorkspace() == source {
		// A release that had to be retried keeps the clock it started with. Re-stamping it
		// each pass would mean a release that keeps failing never ages, and the workspace's
		// one action slot is held for as long as the failure lasts.
		start := &metav1.Time{Time: time.Now().UTC()}
		if info := v1.GetNodeMigrateInfo(node); info != nil && info.Target == target && info.StartTime != nil {
			start = info.StartTime
		}
		return migrationRelease, &v1.NodeMigrateInfo{From: source, Target: target, StartTime: start}
	}
	if node.GetSpecWorkspace() != "" {
		// Bound to a workspace that is neither end of this migration. The node webhook
		// refuses that write, so reaching here means it happened before this version was
		// running, or by a route that does not pass admission at all. Either way the node is
		// not ours to hand over, and going on would have us asking the target to take a node
		// that belongs to someone else, once every pass until the timeout.
		klog.Infof("node(%s) now belongs to workspace(%s), giving up its migration to workspace(%s)",
			node.Name, node.GetSpecWorkspace(), target)
		return migrationAbandoned, nil
	}
	info := v1.GetNodeMigrateInfo(node)
	if info == nil || info.Target != target {
		// The node is neither here nor there and is not reserved for this migration: either
		// someone took the reservation off, or the node was bound elsewhere behind our back.
		// Either way this workspace no longer has a node to migrate.
		klog.Infof("node(%s) is no longer reserved for the migration to workspace(%s), giving up",
			node.Name, target)
		return migrationAbandoned, nil
	}
	if v1.IsNodeMigrationExpired(info, r.migrateTimeout()) {
		klog.Infof("the migration of node(%s) to workspace(%s) timed out after %s, giving up",
			node.Name, target, r.migrateTimeout().String())
		return migrationAbandoned, nil
	}
	return migrationPending, nil
}

// abandonMigrations drops the reservation from nodes whose migration will not complete. The
// node becomes an ordinary unassigned node: it does not go back to the source workspace,
// which gave it up when the migration was admitted and had its replica lowered to match.
func (r *WorkspaceReconciler) abandonMigrations(ctx context.Context,
	workspace *v1.Workspace, nodes []*v1.Node) error {
	for _, node := range nodes {
		if !v1.HasAnnotation(node, v1.NodeMigrateAnnotation) {
			continue
		}
		info := v1.GetNodeMigrateInfo(node)
		target := ""
		if info != nil {
			target = info.Target
		}
		// The node does not go back: this workspace gave it up when the migration was
		// admitted and had its replica lowered to match. That is a loss of capacity nobody
		// asked for, so it is said out loud rather than left to a log line.
		r.recordf(workspace, corev1.EventTypeWarning, "NodeMigrationAbandoned",
			"gave up migrating node(%s) to workspace(%s); the node is now unassigned", node.Name, target)
		patch := client.MergeFrom(node.DeepCopy())
		v1.RemoveAnnotation(node, v1.NodeMigrateAnnotation)
		if err := r.Patch(ctx, node, patch); err != nil {
			klog.ErrorS(err, "failed to drop the migration reservation",
				"node", node.Name, "workspace", workspace.Name)
			return err
		}
	}
	return nil
}

// handOverMigrations asks each target workspace to take the nodes released for it, by writing
// the add action it would have received had a user asked for those nodes directly. That is
// what raises the target's replica, so the handover has to go through the annotation rather
// than binding the nodes here.
//
// Failures are logged and left for the next pass. A target already carrying an action is the
// ordinary case, not an error: a workspace takes one node action at a time, and the nodes
// stay reserved in the meantime.
func (r *WorkspaceReconciler) handOverMigrations(ctx context.Context,
	workspace *v1.Workspace, byTarget map[string][]*v1.Node) {
	for target, nodes := range byTarget {
		nodeNames := make([]string, 0, len(nodes))
		for _, node := range nodes {
			nodeNames = append(nodeNames, node.Name)
		}
		if err := r.handOverToTarget(ctx, target, nodeNames); err != nil {
			klog.ErrorS(err, "failed to hand the migrated nodes over",
				"source", workspace.Name, "target", target, "nodes", nodeNames)
			// On both workspaces: the nodes have left one and have not reached the other, and
			// whoever is watching either one should be able to see why.
			r.recordf(workspace, corev1.EventTypeWarning, "NodeMigrationHandoverFailed",
				"failed to hand nodes %v over to workspace(%s): %v", nodeNames, target, err)
			if targetWorkspace, getErr := r.getWorkspace(ctx, target); getErr == nil {
				r.recordf(targetWorkspace, corev1.EventTypeWarning, "NodeMigrationHandoverFailed",
					"failed to take nodes %v migrated from workspace(%s): %v", nodeNames, workspace.Name, err)
			}
		}
	}
}

// getWorkspace reads a workspace, or reports why it could not be read.
func (r *WorkspaceReconciler) getWorkspace(ctx context.Context, name string) (*v1.Workspace, error) {
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}

// handOverToTarget writes the add action for the migrated nodes on the target workspace.
func (r *WorkspaceReconciler) handOverToTarget(ctx context.Context, target string, nodeNames []string) error {
	action := commonnodes.BuildAction(v1.NodeActionAdd, nodeNames...)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		targetWorkspace := &v1.Workspace{}
		if err := r.Get(ctx, client.ObjectKey{Name: target}, targetWorkspace); err != nil {
			return err
		}
		if !targetWorkspace.GetDeletionTimestamp().IsZero() {
			return fmt.Errorf("the target workspace(%s) is being deleted", target)
		}
		if current := v1.GetWorkspaceNodesAction(targetWorkspace); current != "" {
			if current == action {
				// Already asked; the target has not finished with it yet.
				return nil
			}
			return fmt.Errorf("the target workspace(%s) is processing another job(%s)", target, current)
		}
		patch := client.MergeFrom(targetWorkspace.DeepCopy())
		v1.SetAnnotation(targetWorkspace, v1.WorkspaceNodesAction, action)
		if err := r.Patch(ctx, targetWorkspace, patch); err != nil {
			return err
		}
		klog.Infof("handed nodes %v over to workspace(%s)", nodeNames, target)
		return nil
	})
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
	workspace *v1.Workspace, nodes []*v1.Node, targets map[string]nodeBinding) error {
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

// updateSingleNodeBinding updates the binding of a single node to a Workspace.
func (r *WorkspaceReconciler) updateSingleNodeBinding(ctx context.Context, node *v1.Node, target nodeBinding) (bool, error) {
	// A node arriving at the workspace it was migrating to has finished crossing, so the
	// reservation comes off in the same patch that binds it. This is deliberately keyed on
	// the node's own record rather than on the caller: the handover is one way the target
	// claims it, and scaling up is another.
	// Binding ends the crossing whichever workspace takes the node: the one it was released
	// for, or -- once the reservation has expired and stopped being honoured -- whoever picks
	// it up next. Either way the reservation has done its work and has to come off, or it
	// goes on naming workspaces that have no part in this node and waking them for it.
	claimed := target.workspace != "" && v1.HasAnnotation(node, v1.NodeMigrateAnnotation)
	if node.Spec.Workspace != nil && *node.Spec.Workspace == target.workspace && !claimed {
		return false, nil
	}
	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Workspace = pointer.String(target.workspace)
	if target.migration != nil {
		v1.SetNodeMigrateInfo(node, target.migration)
	} else if claimed {
		v1.RemoveAnnotation(node, v1.NodeMigrateAnnotation)
	}
	action := "bind"
	if target.workspace == "" {
		action = "unbind"
	}
	if err := r.Patch(ctx, node, patch); err != nil {
		klog.ErrorS(err, "failed to update node", "target", target.workspace)
		rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "failed").Inc()
		return false, err
	}
	klog.Infof("updateSingleNodeBinding, node: %s, target: %s", node.Name, target.workspace)
	rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "success").Inc()
	return true, nil
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

// nodeBinding is where a node is being sent, and -- when it is being released for a
// migration -- the crossing to stamp on it as it goes. The two travel together because they
// have to be written together: a node that is unbound without the stamp, even for the moment
// between two patches, is an unassigned node of a matching flavor that any workspace in the
// cluster short of a replica may take.
type nodeBinding struct {
	// workspace to bind the node to; empty releases it.
	workspace string
	// migration to record on the node, or nil to leave its migration state alone.
	migration *v1.NodeMigrateInfo
}

// buildTargetList builds a map of node names to their target Workspace names.
func buildTargetList(nodes []*v1.Node, target string) map[string]nodeBinding {
	results := make(map[string]nodeBinding)
	for _, n := range nodes {
		results[n.Name] = nodeBinding{workspace: target}
	}
	return results
}
