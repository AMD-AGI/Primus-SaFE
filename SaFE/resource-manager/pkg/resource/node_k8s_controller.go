/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonctrl "github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

type NodeAction string
type NodeQueue workqueue.TypedRateLimitingInterface[*nodeQueueMessage]

const (
	NodeAdd       NodeAction = "add"
	NodeDelete    NodeAction = "delete"
	NodeUpdate    NodeAction = "update"
	NodeManaged   NodeAction = "managed"
	NodeUnmanaged NodeAction = "unmanaged"
)

var (
	concernedK8sLabelKeys = []string{v1.WorkspaceIdLabel, v1.ClusterIdLabel,
		v1.NodeStartupTimeLabel, v1.KubernetesControlPlane}
	concernedK8sAnnotationKeys = []string{}
)

type nodeQueueMessage struct {
	k8sNodeName   string
	adminNodeName string
	clusterName   string
	action        NodeAction
	oldConditions []corev1.NodeCondition
}

type NodeK8sReconciler struct {
	ctx context.Context
	*ClusterBaseReconciler
	clientManager        *commonutils.ObjectManager
	queue                NodeQueue
	*commonctrl.Controller[*nodeQueueMessage]
	nodeInformerMu       sync.Mutex
	startedNodeInformers map[string]*commonclient.ClientFactory
	nodeInformerGroup    singleflight.Group
}

// SetupNodeK8sController initializes and registers the NodeK8sReconciler with the controller manager.
func SetupNodeK8sController(ctx context.Context, mgr manager.Manager) error {
	baseReconciler, err := newClusterBaseReconciler(mgr)
	if err != nil {
		return err
	}
	r := &NodeK8sReconciler{
		ctx:                   ctx,
		ClusterBaseReconciler: baseReconciler,
		clientManager:         commonutils.NewObjectManagerSingleton(),
		startedNodeInformers:  make(map[string]*commonclient.ClientFactory),
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[*nodeQueueMessage](),
			workqueue.TypedRateLimitingQueueConfig[*nodeQueueMessage]{Name: "node"}),
	}
	if r.clientManager == nil {
		return fmt.Errorf("failed to new clientManager")
	}
	r.Controller = commonctrl.NewControllerWithQueue[*nodeQueueMessage](r, r.queue, 1)
	if err = r.start(ctx); err != nil {
		return err
	}
	RegisterNodeInformerRestarter(func(ctx context.Context, cluster *v1.Cluster) error {
		return r.restartNodeInformerAfterClientRebuild(ctx, cluster)
	})
	RegisterNodeInformerClearer(r.clearNodeInformerRegistration)
	err = ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(r.relevantChangePredicate())).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup K8sNode Controller successfully")
	return nil
}

// relevantChangePredicate defines which Cluster changes should trigger node informer initialization.
func (r *NodeK8sReconciler) relevantChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			cluster, ok := e.Object.(*v1.Cluster)
			if !ok || !cluster.IsReady() {
				return false
			}
			go func() {
				if err := r.startNodeInformer(cluster); err != nil {
					klog.Errorf("failed to start node informer, err: %v", err)
				}
			}()
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, ok1 := e.ObjectOld.(*v1.Cluster)
			newCluster, ok2 := e.ObjectNew.(*v1.Cluster)
			if !ok1 || !ok2 {
				return false
			}
			go func() {
				if !oldCluster.IsReady() && newCluster.IsReady() {
					if err := r.startNodeInformer(newCluster); err != nil {
						klog.Errorf("failed to start node informer, err: %v", err)
					}
				}
			}()
			return false
		},
	}
}

// startNodeInformer initializes and starts a node informer for the given cluster with retry logic.
func (r *NodeK8sReconciler) startNodeInformer(cluster *v1.Cluster) error {
	if cluster == nil {
		return nil
	}
	_, err, _ := r.nodeInformerGroup.Do(cluster.Name, func() (interface{}, error) {
		return nil, r.startNodeInformerOnce(cluster)
	})
	return err
}

func (r *NodeK8sReconciler) clearNodeInformerRegistration(clusterName string) {
	r.nodeInformerMu.Lock()
	delete(r.startedNodeInformers, clusterName)
	r.nodeInformerMu.Unlock()
}

// restartNodeInformerAfterClientRebuild re-attaches the node informer without blocking on cache sync.
func (r *NodeK8sReconciler) restartNodeInformerAfterClientRebuild(ctx context.Context, cluster *v1.Cluster) error {
	if cluster == nil {
		return nil
	}
	_, err, _ := r.nodeInformerGroup.Do(cluster.Name, func() (interface{}, error) {
		return nil, r.attachNodeInformer(ctx, cluster, false)
	})
	return err
}

func (r *NodeK8sReconciler) startNodeInformerOnce(cluster *v1.Cluster) error {
	const maxRetry = 100
	waitTime := time.Millisecond * 200
	maxWaitTime := waitTime * maxRetry

	return backoff.Retry(func() error {
		return r.attachNodeInformer(r.ctx, cluster, true)
	}, maxWaitTime, waitTime)
}

// attachNodeInformer wires handlers and starts the node informer for the current client factory.
func (r *NodeK8sReconciler) attachNodeInformer(ctx context.Context, cluster *v1.Cluster, waitSync bool) error {
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		klog.ErrorS(err, "failed to get k8s client for data plane", "name", cluster.Name)
		return err
	}
	r.nodeInformerMu.Lock()
	if prev, ok := r.startedNodeInformers[cluster.Name]; ok && prev == k8sClients {
		r.nodeInformerMu.Unlock()
		return nil
	}
	r.nodeInformerMu.Unlock()

	nodeInformer := k8sClients.SharedInformerFactory().Core().V1().Nodes().Informer()
	if _, err = nodeInformer.AddEventHandler(r.nodeEventHandler(k8sClients)); err != nil {
		klog.ErrorS(err, "failed to add event handler", "name", cluster.Name)
		return err
	}
	if err = nodeInformer.SetWatchErrorHandler(commonclient.WatchErrorHandler(r.ctx, k8sClients)); err != nil {
		klog.ErrorS(err, "failed to set error handler", "name", cluster.Name)
		return err
	}
	k8sClients.StartInformer()
	r.nodeInformerMu.Lock()
	r.startedNodeInformers[cluster.Name] = k8sClients
	r.nodeInformerMu.Unlock()

	if waitSync {
		if k8sClients.WaitForCacheSync(time.Minute * 10) {
			klog.Infof("add k8s node informer successfully. cluster: %s", cluster.Name)
		} else {
			klog.Errorf("failed to sync cache for k8s node informer. cluster: %s", cluster.Name)
		}
		return nil
	}

	go func() {
		if ctx.Err() != nil {
			return
		}
		if k8sClients.WaitForCacheSync(time.Minute * 10) {
			klog.Infof("node informer cache synced after client rebuild, cluster: %s", cluster.Name)
		} else {
			klog.Errorf("failed to sync node informer cache after client rebuild, cluster: %s", cluster.Name)
		}
	}()
	return nil
}

// nodeEventHandler creates event handlers for Kubernetes node events (add, update, delete).
func (r *NodeK8sReconciler) nodeEventHandler(k8sClients *commonclient.ClientFactory) cache.ResourceEventHandler {
	enqueue := func(oldNode, newNode *corev1.Node, action NodeAction) {
		node := newNode
		if action == NodeDelete || action == NodeUnmanaged {
			node = oldNode
		}
		item := &nodeQueueMessage{
			k8sNodeName:   node.Name,
			adminNodeName: v1.GetNodeId(node),
			clusterName:   k8sClients.Name(),
			action:        action,
		}
		if oldNode != nil {
			for _, cond := range oldNode.Status.Conditions {
				item.oldConditions = append(item.oldConditions, cond)
			}
		}
		r.queue.Add(item)
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node, ok := obj.(*corev1.Node)
			if !ok || !node.GetDeletionTimestamp().IsZero() || v1.GetClusterId(node) != k8sClients.Name() {
				return
			}
			klog.Infof("cluster %s watch add-event of node %s, workspace %s",
				k8sClients.Name(), node.Name, v1.GetWorkspaceId(node))
			enqueue(nil, node, NodeAdd)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNode, ok1 := oldObj.(*corev1.Node)
			newNode, ok2 := newObj.(*corev1.Node)
			if !ok1 || !ok2 || !newNode.GetDeletionTimestamp().IsZero() {
				return
			}
			oldClusterId := v1.GetClusterId(oldNode)
			newClusterId := v1.GetClusterId(newNode)
			switch {
			case oldClusterId == k8sClients.Name() && newClusterId != k8sClients.Name():
				klog.Infof("cluster %s watch node %s unmanaged, workspace %s",
					k8sClients.Name(), oldNode.Name, v1.GetWorkspaceId(oldNode))
				enqueue(oldNode, newNode, NodeUnmanaged)
			case oldClusterId != k8sClients.Name() && newClusterId == k8sClients.Name():
				klog.Infof("cluster %s watch node %s managed, workspace %s",
					k8sClients.Name(), newNode.Name, v1.GetWorkspaceId(newNode))
				enqueue(oldNode, newNode, NodeManaged)
			case newClusterId == k8sClients.Name() && r.isRelevantFieldChanged(oldNode, newNode):
				klog.Infof("cluster %s watch node %s update, workspace %s",
					k8sClients.Name(), newNode.Name, v1.GetWorkspaceId(newNode))
				enqueue(oldNode, newNode, NodeUpdate)
			}
		},
		DeleteFunc: func(obj interface{}) {
			node, ok := obj.(*corev1.Node)
			if !ok || v1.GetClusterId(node) != k8sClients.Name() {
				return
			}
			klog.Infof("cluster %s watch delete-event of node %s, workspace %s",
				k8sClients.Name(), node.Name, v1.GetWorkspaceId(node))
			enqueue(node, nil, NodeDelete)
		},
	}
}

// isRelevantFieldChanged checks if any watched fields in the Kubernetes Node have changed.
func (r *NodeK8sReconciler) isRelevantFieldChanged(oldNode, newNode *corev1.Node) bool {
	if isConditionsChanged(oldNode.Status.Conditions, newNode.Status.Conditions) ||
		!quantity.Equal(oldNode.Status.Allocatable, newNode.Status.Allocatable) ||
		v1.GetNodeFlavorId(oldNode) != v1.GetNodeFlavorId(newNode) ||
		!commonfaults.IsTaintsEqualIgnoreOrder(oldNode.Spec.Taints, newNode.Spec.Taints) ||
		oldNode.Spec.Unschedulable != newNode.Spec.Unschedulable ||
		!isConcernedLabelsEqual(oldNode, newNode) {
		return true
	}
	return false
}

// Reconcile is the main control loop.
func (r *NodeK8sReconciler) Reconcile(_ context.Context, _ ctrlruntime.Request) (ctrlruntime.Result, error) {
	return ctrlruntime.Result{}, nil
}

// start initializes and runs the worker routines for processing node queue messages.
func (r *NodeK8sReconciler) start(ctx context.Context) error {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
	return nil
}

// Do processes node queue messages and synchronizes node status between Kubernetes and admin nodes.
func (r *NodeK8sReconciler) Do(ctx context.Context, message *nodeQueueMessage) (ctrlruntime.Result, error) {
	adminNode := new(v1.Node)
	err := r.Get(ctx, apitypes.NamespacedName{Name: message.adminNodeName}, adminNode)
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !adminNode.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}

	switch message.action {
	case NodeDelete, NodeUnmanaged:
		err = r.handleNodeUnmanaged(ctx, message, adminNode)
	default:
		err = r.handleNodeUpdate(ctx, message, adminNode)
	}
	if err != nil {
		klog.ErrorS(err, "failed to handle message", "clusterName", message.clusterName,
			"k8sNodeName", message.k8sNodeName, "action", message.action)
		if commonerrors.IsNonRetryableError(err) {
			err = nil
		}
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// handleNodeUnmanaged handles node unmanaged or deletion events by resetting admin node metadata and status.
func (r *NodeK8sReconciler) handleNodeUnmanaged(ctx context.Context, message *nodeQueueMessage, adminNode *v1.Node) error {
	clusterName := message.clusterName
	workspaceId := v1.GetWorkspaceId(adminNode)
	if deleteConcernedMeta(adminNode) {
		if err := r.Update(ctx, adminNode); err != nil {
			klog.ErrorS(err, "failed to update node", "node", adminNode.Name)
			return err
		}
	}

	adminNode.Status.Taints = nil
	adminNode.Status.Resources = nil
	adminNode.Status.Conditions = nil
	adminNode.Status.Unschedulable = true
	if err := r.Status().Update(ctx, adminNode); err != nil {
		klog.ErrorS(err, "failed to update node status", "node", adminNode.Name)
		return err
	}
	klog.Infof("reset adminNode metadata and status, name: %s, cluster: %s, workspace: %s",
		adminNode.Name, clusterName, workspaceId)
	return nil
}

// handleNodeUpdate handles node update events by synchronizing metadata, status, and processing faults.
func (r *NodeK8sReconciler) handleNodeUpdate(ctx context.Context, message *nodeQueueMessage, adminNode *v1.Node) error {
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, message.clusterName)
	if err != nil || !k8sClients.IsValid() {
		return fmt.Errorf("the cluster(%s) clients is not ready", message.clusterName)
	}
	k8sNode, err := getNodeByInformer(ctx, k8sClients, message.k8sNodeName)
	if err != nil {
		return err
	}
	if err = r.syncK8sMetadata(ctx, adminNode, k8sNode); err != nil {
		return err
	}
	if err = r.syncK8sStatus(ctx, adminNode, k8sNode); err != nil {
		return err
	}
	if err = r.processFault(ctx, adminNode, message); err != nil {
		return err
	}
	return nil
}

// syncK8sMetadata synchronizes labels and annotations from the Kubernetes node to the admin node.
func (r *NodeK8sReconciler) syncK8sMetadata(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) error {
	shouldUpdate := false
	for _, k := range concernedK8sLabelKeys {
		v, ok := k8sNode.Labels[k]
		// Which workspace owns a node is decided in the admin plane; the data plane label is
		// only the confirmation that the binding landed. A value that disagrees with
		// spec.workspace is stale or was edited by hand, and copying it back would make the
		// admin plane report an ownership nobody asked for. NodeReconciler pushes the right
		// value back down; the label is still removed through the branch below, once the data
		// plane no longer carries it at all.
		//
		// V(4), not a warning: every bind and unbind opens this window between the admin
		// plane write and the data plane catching up, and this function runs against every
		// managed node every few seconds. A scale-down of a large workspace would otherwise
		// fill the log with a line per node per pass while it is doing exactly the right
		// thing.
		if ok && k == v1.WorkspaceIdLabel && v != adminNode.GetSpecWorkspace() {
			klog.V(4).Infof("ignore workspace label %s on k8s node %s, the node is bound to %q",
				v, k8sNode.Name, adminNode.GetSpecWorkspace())
			continue
		}
		if ok {
			if v1.SetLabel(adminNode, k, v) {
				shouldUpdate = true
			}
		} else if v1.RemoveLabel(adminNode, k) {
			shouldUpdate = true
		}
	}

	for _, k := range concernedK8sAnnotationKeys {
		if v, ok := k8sNode.Annotations[k]; ok {
			if v1.SetAnnotation(adminNode, k, v) {
				shouldUpdate = true
			}
		} else if v1.RemoveAnnotation(adminNode, k) {
			shouldUpdate = true
		}
	}

	if !shouldUpdate {
		return nil
	}
	if err := r.Update(ctx, adminNode); err != nil {
		return err
	}
	return nil
}

// syncK8sStatus synchronizes status information from the Kubernetes node to the admin node.
func (r *NodeK8sReconciler) syncK8sStatus(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) error {
	originalNode := adminNode.DeepCopy()
	adminNode.Status.MachineStatus.PrivateIP = commonnodes.GetInternalIp(k8sNode)
	adminNode.Status.Unschedulable = k8sNode.Spec.Unschedulable
	adminNode.Status.Taints = k8sNode.Spec.Taints
	adminNode.Status.Conditions = k8sNode.Status.Conditions
	adminNode.Status.Resources = quantity.GetConcernedResources(k8sNode.Status.Allocatable)
	if !reflect.DeepEqual(originalNode.Status, adminNode.Status) {
		if err := r.Status().Update(ctx, adminNode); err != nil {
			klog.ErrorS(err, "failed to update node status", "name", adminNode.Name)
			return err
		}
	}
	return nil
}

// processFault processes node condition changes and creates or deletes faults accordingly.
func (r *NodeK8sReconciler) processFault(ctx context.Context, adminNode *v1.Node, message *nodeQueueMessage) error {
	faultConfigMap, err := GetFaultConfigmap(ctx, r.Client)
	if err != nil || len(faultConfigMap) == 0 {
		return err
	}

	oldConditionMap := make(map[corev1.NodeConditionType]corev1.ConditionStatus)
	for i := range message.oldConditions {
		oldConditionMap[message.oldConditions[i].Type] = message.oldConditions[i].Status
	}
	newConditionSet := sets.NewSet()
	faultNode := genFaultNodeByMessage(message)
	for _, newCondition := range adminNode.Status.Conditions {
		newConditionSet.Insert(string(newCondition.Type))
		oldCondStatus, ok := oldConditionMap[newCondition.Type]
		// Skip if no changes are detected
		if ok && oldCondStatus == newCondition.Status {
			continue
		}
		if shouldCreateFault(newCondition) {
			if adminNode.GetSpecCluster() == "" {
				continue
			}
			if f := generateFaultOnCreation(faultNode, newCondition, faultConfigMap); f != nil {
				if err = createFault(ctx, r.Client, f); err != nil {
					return err
				}
			}
		} else if f := generateFaultOnDeletion(faultNode, newCondition, faultConfigMap); f != nil {
			if err = deleteFault(ctx, r.Client, f); err != nil {
				return err
			}
		}
	}
	for _, oldCondition := range message.oldConditions {
		if newConditionSet.Has(string(oldCondition.Type)) {
			continue
		}
		if f := generateFaultOnDeletion(faultNode, oldCondition, faultConfigMap); f != nil {
			if err = deleteFault(ctx, r.Client, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteConcernedMeta removes all concerned labels and annotations from the admin node.
// Returns true if any metadata was removed, false otherwise.
func deleteConcernedMeta(adminNode *v1.Node) bool {
	isChanged := false
	for _, k := range concernedK8sLabelKeys {
		if v1.RemoveLabel(adminNode, k) {
			isChanged = true
		}
	}
	for _, k := range concernedK8sAnnotationKeys {
		if v1.RemoveAnnotation(adminNode, k) {
			isChanged = true
		}
	}
	if v1.RemoveAnnotation(adminNode, v1.NodeTemplateInstalledAnnotation) {
		isChanged = true
	}
	return isChanged
}

// isConcernedLabelsEqual checks if the concerned labels and annotations are equal between two objects.
func isConcernedLabelsEqual(obj1, obj2 metav1.Object) bool {
	if maps.CompareWithKeys(obj1.GetLabels(), obj2.GetLabels(), concernedK8sLabelKeys) &&
		maps.CompareWithKeys(obj1.GetAnnotations(), obj2.GetAnnotations(), concernedK8sAnnotationKeys) {
		return true
	}
	return false
}

// genFaultNodeByMessage creates a FaultNode object from a node queue message.
func genFaultNodeByMessage(message *nodeQueueMessage) *v1.FaultNode {
	return &v1.FaultNode{
		AdminName:   message.adminNodeName,
		ClusterName: message.clusterName,
	}
}
