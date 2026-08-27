/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"errors"
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
	// apiReader reads straight from the apiserver, bypassing the manager's cache. Used where
	// a cached answer would be either unsafe or final: in updateSingleNodeBinding, where the
	// read is both the state the ownership rule is judged against and the resourceVersion the
	// write is locked to; and when deciding that the workspace a migration is addressed to is
	// really gone, which cannot be taken back.
	apiReader client.Reader
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
	expectations map[string]*nodeExpectations
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
		apiReader:             mgr.GetAPIReader(),
		recorder:              mgr.GetEventRecorderFor("workspace-controller"),
		clientManager:         commonutils.NewObjectManagerSingleton(),
		expectations:          make(map[string]*nodeExpectations),
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
	// Waking only, never settling: an expectation is settled by the label arriving, and a
	// migration says nothing about whether it has.
	enqueueMigration := func(q v1.RequestWorkQueue, node *v1.Node) {
		info := v1.GetNodeMigrateInfo(node)
		if info == nil {
			return
		}
		enqueue(q, info.From)
		enqueue(q, info.Target)
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			node, ok := evt.Object.(*v1.Node)
			if !ok {
				return
			}
			observe(node.Name, v1.GetWorkspaceId(node))
			enqueue(q, v1.GetWorkspaceId(node))
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

	// Uncached, and scoped to this workspace's cluster.
	//
	// Uncached because of what is done with the answer: every node this list misses keeps its
	// claim on a Workspace that is about to lose its finalizer and disappear, and there is no
	// second pass to catch it -- the claim is then held by a name that resolves to nothing, and
	// only a manual edit gets the node back. A cached List is stale exactly when it matters
	// most, right after this controller's own writes; the rest of the single-owner path already
	// reads through r.apiReader for the same reason, and dropping the finalizer is the one
	// decision here that cannot be taken back.
	//
	// Scoped because the filter below is a claim on a node in this workspace's own cluster, so
	// no node outside it can qualify, and without the selector every deletion pulls the whole
	// NodeList across the wire -- a cost the cache used to hide and an uncached read does not.
	nodeList := &v1.NodeList{}
	listOpts := &client.ListOptions{}
	if workspace.Spec.Cluster != "" {
		listOpts.LabelSelector = labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: workspace.Spec.Cluster})
	}
	if err = r.apiReader.List(ctx, nodeList, listOpts); err != nil {
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
	// No wait for the label round trip here, unlike processWorkspace. An expectation exists to
	// keep a workspace from acting on a node count that has not caught up with what it just
	// bound or released; a workspace on its way out has no next decision to get wrong. What
	// does have to be true before the finalizer comes off is that every claim is released, and
	// that is what the call above returns an error for -- the label follows from the claim on
	// the node's own reconcile, whether or not this Workspace still exists.
	//
	// Waiting was also wrong here in a way it is not there: the node list is rebuilt from the
	// remaining claims on every pass, so it shrinks as unbinds land, and setExpectations
	// replaces rather than merges. A node whose claim was cleared on an earlier pass dropped
	// out of the set anyway.
	//
	// The entry still goes, so a workspace that had expectations outstanding when it was
	// deleted does not leave them in the map for good. concurrent.Exec has joined its
	// goroutines by now, so nothing is left to write the entry back; a late Node event finds
	// no entry and settles nothing.
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

// expectationTimeout is how long a workspace waits to be told its bindings landed before it
// carries on without being told.
//
// The wait is normally seconds -- it ends when the node controller writes the workspace onto
// the node's labels and the event comes back. What it protects against is a scaling decision
// taken on counts that have not caught up, and a stale count is a far smaller problem than
// the alternative: nothing settles an expectation except that one event, and a workspace
// still holding one runs nothing at all -- no status, no scaling, and no deletion, because
// removing the finalizer waits behind the same gate. A node deleted before its label is
// written, or an event missed, would otherwise leave the workspace that way for good.
const expectationTimeout = 5 * time.Minute

// nodeExpectations is the nodes a Workspace is waiting to see settled, each with the point
// past which it stops being waited on.
//
// Per node, not per workspace. A workspace that keeps binding nodes keeps having entries
// added, and one shared deadline pushed forward by each of them never arrives -- which is
// exactly the workspace where a stale entry hides, since node actions run before the gate and
// go on adding to it.
type nodeExpectations struct {
	deadlines map[string]time.Time
}

// unsettled counts the nodes still worth waiting on at the given time.
func (e *nodeExpectations) unsettled(now time.Time) int {
	count := 0
	for _, deadline := range e.deadlines {
		if now.Before(deadline) {
			count++
		}
	}
	return count
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
	if !ok || left == nil {
		left = &nodeExpectations{deadlines: make(map[string]time.Time)}
		r.expectations[workspaceId] = left
	}
	now := time.Now()
	deadline := now.Add(expectationTimeout)
	for nodeName := range nodeNames {
		// Each node keeps the deadline it came in with; a later binding is not a reason to go
		// on waiting for an earlier one.
		//
		// Unless that deadline has already passed. An entry is only removed when the node
		// settles or when a prune reaches it, and neither is guaranteed to have happened by
		// now -- so a lapsed entry can still be sitting here when the same node is bound
		// again. Left as it was, the new binding inherits a deadline that expired before it
		// started, the gate opens on it at once, and it opens on that node for every binding
		// after this one too.
		if existing, waiting := left.deadlines[nodeName]; !waiting || !now.Before(existing) {
			left.deadlines[nodeName] = deadline
		}
	}
}

// meetExpectations checks if all expected node operations for a Workspace have been completed.
// It only reads: an expired entry is not waited on, and pruneExpectations is what removes it.
// A check that quietly consumed the way out would leave whoever calls it next -- a metric, a
// probe -- to find the workspace waiting all over again.
func (r *WorkspaceReconciler) meetExpectations(workspaceId string) bool {
	r.RLock()
	defer r.RUnlock()
	left, ok := r.expectations[workspaceId]
	return !ok || left == nil || left.unsettled(time.Now()) == 0
}

// pruneExpectations drops the nodes a Workspace has stopped waiting on.
func (r *WorkspaceReconciler) pruneExpectations(workspaceId string) {
	r.Lock()
	defer r.Unlock()
	left, ok := r.expectations[workspaceId]
	if !ok || left == nil {
		return
	}
	now := time.Now()
	for nodeName, deadline := range left.deadlines {
		if now.Before(deadline) {
			continue
		}
		klog.Infof("workspace(%s) stopped waiting on node(%s) after %s",
			workspaceId, nodeName, expectationTimeout.String())
		delete(left.deadlines, nodeName)
	}
	if len(left.deadlines) == 0 {
		delete(r.expectations, workspaceId)
	}
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
// An emptied entry is deleted rather than left behind: observeNodeForAll walks this map on
// every admin Node event, and NodeK8sReconciler writes those nodes every few seconds.
func (r *WorkspaceReconciler) settle(workspaceId, nodeName string) bool {
	left, ok := r.expectations[workspaceId]
	if !ok || left == nil {
		return false
	}
	if _, waiting := left.deadlines[nodeName]; !waiting {
		return false
	}
	delete(left.deadlines, nodeName)
	if len(left.deadlines) == 0 {
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
	// Ahead of the node action, not after it: that call returns early on the passes where it
	// applied something, which are exactly the passes that have just added expectations, so a
	// prune behind it never runs on the rounds that need it most.
	r.pruneExpectations(workspace.Name)
	var actionResult ctrlruntime.Result
	if v1.GetWorkspaceNodesAction(workspace) != "" {
		var isUpdated bool
		if actionResult, isUpdated, err = r.processNodesAction(ctx, workspace); err != nil || isUpdated {
			return actionResult, err
		}
	}
	if !r.meetExpectations(workspace.Name) {
		// Same reason: the event that settles this may never come, and the deadline that ends
		// the wait is only read when something asks.
		if actionResult.RequeueAfter == 0 {
			actionResult.RequeueAfter = r.option.nodeWait
		}
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
	candidates := make([]int, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		if !isNodeEligibleForScalingUp(&nodeList.Items[i], workspace) {
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

// isNodeEligibleForScalingUp reports whether an unbound node in the workspace's cluster may
// be picked up by it. Callers have already narrowed the list to that cluster.
//
// The workspace argument is what keeps a migration from being undone by the scaling loop.
// A node released by its source workspace is unbound and of a matching flavor, which is
// every quality scale-up looks for, so between the two halves of a migration any workspace
// in the cluster short of a replica would take it -- including the source workspace itself,
// were its replica not decremented first. The node carries the target it was released for,
// and only that target may claim it.
func isNodeEligibleForScalingUp(node *v1.Node, workspace *v1.Workspace) bool {
	if !node.IsMachineReady() || !node.IsManaged() {
		return false
	}
	if node.GetSpecWorkspace() != "" || v1.GetWorkspaceId(node) != "" {
		return false
	}
	if v1.GetNodeFlavorId(node) != workspace.Spec.NodeFlavor {
		return false
	}
	// No exception for the target of the migration. Letting it pick the node up here was a
	// second way to finish a crossing whose handover had been lost, but it finishes it
	// without the accounting: scaling up happens because the workspace is already short of a
	// replica, so the node arrives against a replica the workspace wanted anyway, and the one
	// the migration was supposed to add is never added. The source has already given one up,
	// so a migration completed this way quietly costs the pair a replica. The handover is
	// retried until the migration times out, which is the one path that keeps the count.
	//
	// Judged against the shared timeout rather than this controller's own. The node webhook
	// stops refusing bindings at the shared one, and a node this loop thinks is free while
	// the webhook still refuses to bind it is picked and rejected, over and over.
	//
	// A stale reservation is ignored, because this is the only place a reservation can do
	// harm on its own and the things that clear one can all go away: the source workspace
	// can be deleted mid-migration, and a node can be taken out of the cluster and brought
	// back still carrying the annotation. Without the age check any of those parks a node
	// for good, and nothing left in the system would explain why.
	if info := v1.GetNodeMigrateInfo(node); info != nil &&
		!v1.IsNodeMigrationExpired(info, v1.DefaultNodeMigrateTimeout) {
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

	// The claim, not the label. Status.AvailableReplica and AbnormalReplica are what
	// CurrentReplica adds up, and CurrentReplica is what the scaling switch below turns into a
	// count of nodes to bind or release -- while the candidates for that count come from
	// GetIdleNodesOfWorkspace, which answers on spec.workspace. Counting here on the label made
	// the two disagree for the length of a binding's round trip, and the disagreement is not
	// symmetric: an over-count asks scale-down for one node more than the workspace holds, and
	// the extra one comes out of the machines it does hold.
	nodes, err := commonnodes.GetNodesOfWorkspaces(ctx, r.Client, []string{workspace.Name},
		commonnodes.FilterUnclaimedNode(workspace.Name))
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
// Silent on a bad value by design, which is the only thing it adds to commonnodes.ParseAction.
// Most callers read other workspaces' annotations on a hot path and would report the same
// broken value every round without being able to do anything about it; the workspace whose
// request it is logs it once, in processNodesAction, and clears it in the same round.
func parseNodesAction(workspace *v1.Workspace) map[string]string {
	actions, _ := commonnodes.ParseAction(workspace)
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
//
// A migration is the exception, and the only entry that does not finish in a single pass. It
// is carried out from the source workspace, whose annotation stays put for the whole crossing
// and is the record that one is under way: release the node here, hand it to the target, and
// clear the annotation once it has landed. Every pass works out which of those three it is
// looking at, so an interrupted migration resumes rather than restarts -- and one that cannot
// finish is refused like any other entry, which withdraws it where the user can see it.
func (r *WorkspaceReconciler) processNodesAction(ctx context.Context,
	workspace *v1.Workspace) (ctrlruntime.Result, bool, error) {
	actions := parseNodesAction(workspace)
	if len(actions) == 0 {
		if raw := v1.GetWorkspaceNodesAction(workspace); raw != "" {
			klog.Errorf("workspace %s: unusable nodes action %q. dropping it", workspace.Name, raw)
		}
		return ctrlruntime.Result{}, false, r.removeNodesAction(ctx, workspace)
	}
	klog.Infof("process node action: %v", actions)

	newActions := make(map[string]nodeBinding)
	adminNodes := make([]*v1.Node, 0, len(actions))
	refusals := make(map[string]string, len(actions))
	// Nodes already released and waiting for the target to take them, by target, and nodes
	// whose reservation has to come off because their migration is being given up on.
	pendingHandover := make(map[string][]*v1.Node)
	var abandoned []*v1.Node
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
			return ctrlruntime.Result{}, false, err
		}
		if migrateTarget, ok := v1.ParseMigrateAction(val); ok {
			state, migration := r.classifyMigration(node, workspace.Name, migrateTarget)
			switch state {
			case migrationRelease:
				// Judged as the unbind it is, by the same rule as any other release: only the
				// workspace holding a node may let it go.
				if verdict, reason := judgeNodeBinding(node, "", workspace.Name); verdict != bindProceed {
					refusals[key] = reason
					continue
				}
				newActions[node.Name] = nodeBinding{migration: migration}
				adminNodes = append(adminNodes, node)
			case migrationPending:
				pendingHandover[migrateTarget] = append(pendingHandover[migrateTarget], node)
			case migrationAbandoned:
				abandoned = append(abandoned, node)
				refusals[key] = fmt.Sprintf("its migration to workspace(%s) cannot be completed", migrateTarget)
			case migrationDone:
			}
			continue
		}
		target := workspace.Name
		if val == v1.NodeActionRemove {
			target = ""
		} else if val != v1.NodeActionAdd {
			// Anything else is not an action this understands, and the one thing not to do
			// with it is guess. Reading it as an add -- which taking the default above
			// amounts to -- binds the node with none of the replica accounting a real add is
			// admitted with, so a migrate that lost its target on the way in would quietly
			// claim the node instead of being turned down.
			refusals[key] = fmt.Sprintf("%q is not an action this understands", val)
			continue
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
		newActions[node.Name] = nodeBinding{workspace: target}
		adminNodes = append(adminNodes, node)
	}
	if len(abandoned) > 0 {
		// Before the withdrawal below takes the entries away: the annotation is what brings
		// this code back, so a node left carrying a reservation after its entry has gone is
		// reserved for a workspace that is never coming for it.
		if err := r.abandonMigrations(ctx, workspace, abandoned); err != nil {
			return ctrlruntime.Result{}, false, err
		}
	}
	if len(refusals) > 0 {
		if err := r.dropRefusedActions(ctx, workspace, actions, refusals); err != nil {
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
		waiting := 0
		for _, nodes := range pendingHandover {
			waiting += len(nodes)
		}
		if lost := r.handOverMigrations(ctx, workspace, pendingHandover); len(lost) > 0 {
			if err := r.abandonMigrations(ctx, workspace, lost); err != nil {
				return ctrlruntime.Result{}, false, err
			}
			waiting -= len(lost)
		}
		if waiting > 0 {
			return ctrlruntime.Result{RequeueAfter: r.option.nodeWait}, false, nil
		}
	}
	// Nothing is pending any more, whether the entries were applied already or withdrawn.
	// Saying otherwise makes processWorkspace stop for a requeue that may never be asked
	// for: a withdrawal need not change any spec field, and neither predicate on this
	// controller fires for an annotation that only goes away.
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
	// A node on its way out is not going anywhere else. Releasing it would still be allowed --
	// only binding a deleting node is refused, so that a workspace being deleted can let go of
	// what it holds -- and the crossing would then run to its timeout being turned down by the
	// far end, once a pass, for a node that is about to stop existing.
	if !node.GetDeletionTimestamp().IsZero() {
		klog.Infof("node(%s) is being deleted, giving up its migration to workspace(%s)",
			node.Name, target)
		return migrationAbandoned, nil
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
	if !v1.IsNodeReleasedFor(node, source, target) {
		// The node is neither here nor there and is not reserved for this migration -- not
		// reserved at all, reserved for somewhere else, or released by another workspace
		// whose crossing this one has no business finishing. The admission side refuses to
		// take one of those on; this is the same line held where the work is done.
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
//
// A target that is gone is the exception, and the nodes released for it are returned so the
// caller can give up on them now. Retrying that one is waiting for a workspace to come back:
// the nodes stay reserved and the source holds its action slot for the whole of the migration
// timeout to reach a conclusion that is already certain. Deleted and being deleted count the
// same -- a workspace on its way out is not going to take the nodes on.
func (r *WorkspaceReconciler) handOverMigrations(ctx context.Context,
	workspace *v1.Workspace, byTarget map[string][]*v1.Node) []*v1.Node {
	var lost []*v1.Node
	for target, nodes := range byTarget {
		nodeNames := make([]string, 0, len(nodes))
		for _, node := range nodes {
			nodeNames = append(nodeNames, node.Name)
		}
		if err := r.handOverToTarget(ctx, target, nodeNames); err != nil {
			if errors.Is(err, errMigrationTargetGone) {
				klog.Infof("the target workspace(%s) is gone, giving up the migration of nodes %v from workspace(%s)",
					target, nodeNames, workspace.Name)
				lost = append(lost, nodes...)
				continue
			}
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
	return lost
}

// getWorkspace reads a workspace, or reports why it could not be read.
func (r *WorkspaceReconciler) getWorkspace(ctx context.Context, name string) (*v1.Workspace, error) {
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}

// errMigrationTargetGone marks a handover that will not succeed however often it is retried,
// because the workspace it is addressed to has gone or is going.
var errMigrationTargetGone = errors.New("the migration target is gone")

// handOverToTarget writes the add action for the migrated nodes on the target workspace.
func (r *WorkspaceReconciler) handOverToTarget(ctx context.Context, target string, nodeNames []string) error {
	action := commonnodes.BuildAction(v1.NodeActionAdd, nodeNames...)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		targetWorkspace := &v1.Workspace{}
		if err := r.Get(ctx, client.ObjectKey{Name: target}, targetWorkspace); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			// Confirmed against the apiserver before it counts. This read comes from a cache,
			// and giving up is final -- the reservation comes off and the source's replica is
			// not given back -- so a cache that has not caught up must not be what decides
			// it. Without a reader to confirm with, the cached answer stands.
			if r.apiReader != nil {
				if err = r.apiReader.Get(ctx, client.ObjectKey{Name: target}, targetWorkspace); err == nil {
					return fmt.Errorf("the target workspace(%s) is not in the cache yet", target)
				} else if !apierrors.IsNotFound(err) {
					return err
				}
			}
			return fmt.Errorf("%w: workspace(%s) does not exist", errMigrationTargetGone, target)
		}
		if !targetWorkspace.GetDeletionTimestamp().IsZero() {
			return fmt.Errorf("%w: workspace(%s) is being deleted", errMigrationTargetGone, target)
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
		target := targets[n.Name]
		ok, err := r.updateSingleNodeBinding(ctx, workspace.Name, n, target)
		// An expectation waits for the workspace label to make the round trip through the
		// data plane, and handleNodeEvent only credits it on a *change* of that label. A node
		// whose label already reads the target has nothing left to wait for, so waiting anyway
		// wedges the workspace: no admin Node write is coming to carry a transition that has
		// already happened. n is the object updateSingleNodeBinding read through, so its
		// labels are fresh.
		if !ok || err != nil || v1.GetWorkspaceId(n) == target.workspace {
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

// clearsReservation reports whether this binding ends a crossing the node is carrying.
//
// Binding to any workspace ends it: the one the node was released for, or -- once the
// reservation has expired and stopped being honoured -- whoever picks it up next. Releasing
// does not, or a release would erase the record it is supposed to be writing.
func clearsReservation(node *v1.Node, target nodeBinding) bool {
	return target.workspace != "" && target.migration == nil &&
		v1.HasAnnotation(node, v1.NodeMigrateAnnotation)
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
	requester string, node *v1.Node, target nodeBinding) (bool, error) {
	action := "bind"
	if target.workspace == "" {
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
		verdict, reason := judgeNodeBinding(node, target.workspace, requester)
		switch verdict {
		case bindSettled:
			// Already where it should be. Note this covers an unbind against a nil
			// Spec.Workspace, which reads as "". Writing "" over the nil to make the release
			// explicit would return updated=true and stop the caller settling the
			// expectation, while changing nothing any reader can see.
			//
			// A reservation left on a node that has already arrived is the exception: the
			// bind landed and the write that was to clear it did not, and nothing else would
			// ever take it off.
			if !clearsReservation(node, target) {
				return nil
			}
		case bindRefused:
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "refused").Inc()
			return fmt.Errorf("cannot %s node %s for %s: %s", action, node.Name, requester, reason)
		}
		patch := client.MergeFromWithOptions(node.DeepCopy(), client.MergeFromWithOptimisticLock{})
		node.Spec.Workspace = pointer.String(target.workspace)
		// The crossing is recorded and erased by the same write that moves the node, so there
		// is no moment where the node is released without the record, or bound with it still
		// on. The first would leave it unassigned and unclaimed for anyone short of a replica
		// to take; the second would go on naming workspaces with no part in it.
		if target.migration != nil {
			v1.SetNodeMigrateInfo(node, target.migration)
		} else if clearsReservation(node, target) {
			v1.RemoveAnnotation(node, v1.NodeMigrateAnnotation)
		}
		patchErr := r.Patch(ctx, node, patch)
		if patchErr == nil {
			updated = true
			klog.Infof("updateSingleNodeBinding, node: %s, target: %s", node.Name, target.workspace)
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "success").Inc()
			return nil
		}
		if !apierrors.IsConflict(patchErr) {
			klog.ErrorS(patchErr, "failed to update node", "target", target)
			rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, "failed").Inc()
			return patchErr
		}
		klog.Warningf("conflict while trying to %s node %s, target: %s. it will be retried",
			action, node.Name, target.workspace)
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
