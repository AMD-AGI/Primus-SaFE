/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"reflect"

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
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	commonctrl "github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
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
	concernedK8sAnnotationKeys = []string{v1.GpuProductNameAnnotation}
)

type nodeQueueMessage struct {
	k8sNodeName   string
	adminNodeName string
	clusterName   string
	action        NodeAction
	oldConditions []corev1.NodeCondition
}

type NodeK8sReconciler struct {
	*ClusterBaseReconciler
	clientManager *commonutils.ObjectManager
	queue         NodeQueue
	*commonctrl.Controller[*nodeQueueMessage]
}

func SetupNodeK8sController(ctx context.Context, mgr manager.Manager) error {
	r := &NodeK8sReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		clientManager: commonutils.NewObjectManagerSingleton(),
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[*nodeQueueMessage](),
			workqueue.TypedRateLimitingQueueConfig[*nodeQueueMessage]{Name: "node"}),
	}
	r.Controller = commonctrl.NewControllerWithQueue[*nodeQueueMessage](r, r.queue, 1)
	if err := r.start(ctx); err != nil {
		return err
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(r.CaredPredicate())).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup K8sNode Controller successfully")
	return nil
}

func (r *NodeK8sReconciler) CaredPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, ok1 := e.ObjectOld.(*v1.Cluster)
			newCluster, ok2 := e.ObjectNew.(*v1.Cluster)
			if !ok1 || !ok2 {
				return false
			}
			if !oldCluster.IsReady() && newCluster.IsReady() ||
				!oldCluster.IsControlPlaneCertEqual(newCluster.Status.ControlPlaneStatus) ||
				!oldCluster.IsControlPlaneEndpointEqual(newCluster.Status.ControlPlaneStatus.Endpoints) {
				if err := r.addClientFactory(newCluster); err != nil {
					klog.Errorf("failed to add cluster clients, err: %v", err)
				}
			} else if oldCluster.IsReady() &&
				(!newCluster.IsReady() || !newCluster.GetDeletionTimestamp().IsZero()) {
				if err := r.clientManager.Delete(oldCluster.Name); err != nil {
					klog.Errorf("failed to delete cluster clients, err: %v", err)
				}
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			cluster, ok := e.Object.(*v1.Cluster)
			if !ok {
				return false
			}
			if err := r.clientManager.Delete(cluster.Name); err != nil {
				klog.Errorf("failed to delete cluster clients, err: %v", err)
			}
			return false
		},
	}
}

func (r *NodeK8sReconciler) addClientFactory(cluster *v1.Cluster) error {
	controlPlane := &cluster.Status.ControlPlaneStatus
	endpoint, err := commoncluster.GetEndpoint(context.Background(), r.Client, cluster.Name, controlPlane.Endpoints)
	if err != nil {
		return err
	}
	k8sClients, err := commonclient.NewClientFactory(context.Background(), cluster.Name, endpoint,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, commonclient.EnableInformer)
	if err != nil {
		return err
	}

	nodeInformer := k8sClients.SharedInformerFactory().Core().V1().Nodes().Informer()
	if _, err = nodeInformer.AddEventHandler(r.nodeEventHandler(k8sClients)); err != nil {
		klog.ErrorS(err, "failed to add event handler", "name", cluster.Name)
		return err
	}
	if err = nodeInformer.SetWatchErrorHandler(watchErrorHandler(k8sClients)); err != nil {
		klog.ErrorS(err, "failed to set error handler", "name", cluster.Name)
		return err
	}
	r.clientManager.AddOrReplace(cluster.Name, k8sClients)
	k8sClients.StartInformer()
	k8sClients.WaitForCacheSync()
	return nil
}

func (r *NodeK8sReconciler) nodeEventHandler(k8sClients *commonclient.ClientFactory) cache.ResourceEventHandler {
	check := func() {
		if !k8sClients.IsValid() {
			k8sClients.SetValid(true, "")
		}
	}
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
			check()
			node, ok := obj.(*corev1.Node)
			if !ok || !node.GetDeletionTimestamp().IsZero() || v1.GetClusterId(node) != k8sClients.Name() {
				return
			}
			klog.Infof("cluster: %s watch add-event of node: %s, workspace: %s",
				k8sClients.Name(), node.Name, v1.GetWorkspaceId(node))
			enqueue(nil, node, NodeAdd)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			check()
			oldNode, ok1 := oldObj.(*corev1.Node)
			newNode, ok2 := newObj.(*corev1.Node)
			if !ok1 || !ok2 || !newNode.GetDeletionTimestamp().IsZero() {
				return
			}
			oldClusterId := v1.GetClusterId(oldNode)
			newClusterId := v1.GetClusterId(newNode)
			switch {
			case oldClusterId == k8sClients.Name() && newClusterId != k8sClients.Name():
				klog.Infof("cluster: %s watch node unmanaged: %s, workspace: %s",
					k8sClients.Name(), oldNode.Name, v1.GetWorkspaceId(oldNode))
				enqueue(oldNode, newNode, NodeUnmanaged)
			case oldClusterId != k8sClients.Name() && newClusterId == k8sClients.Name():
				klog.Infof("cluster: %s watch node managed: %s, workspace: %s",
					k8sClients.Name(), newNode.Name, v1.GetWorkspaceId(newNode))
				enqueue(oldNode, newNode, NodeManaged)
			case newClusterId == k8sClients.Name() && r.isNodeCaredFieldChanged(oldNode, newNode):
				enqueue(oldNode, newNode, NodeUpdate)
			}
		},
		DeleteFunc: func(obj interface{}) {
			check()
			node, ok := obj.(*corev1.Node)
			if !ok || v1.GetClusterId(node) != k8sClients.Name() {
				return
			}
			klog.Infof("cluster: %s watch delete-event of node: %s, workspace: %s",
				k8sClients.Name(), node.Name, v1.GetWorkspaceId(node))
			enqueue(node, nil, NodeDelete)
		},
	}
}

func watchErrorHandler(k8sClients *commonclient.ClientFactory) cache.WatchErrorHandler {
	return func(reflector *cache.Reflector, err error) {
		cache.DefaultWatchErrorHandler(context.Background(), reflector, err)
		klog.Warningf("set clients: %s invalid", k8sClients.Name())
		k8sClients.SetValid(false, err.Error())
	}
}

func (r *NodeK8sReconciler) isNodeCaredFieldChanged(oldNode, newNode *corev1.Node) bool {
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

func (r *NodeK8sReconciler) Reconcile(_ context.Context, _ ctrlruntime.Request) (ctrlruntime.Result, error) {
	return ctrlruntime.Result{}, nil
}

func (r *NodeK8sReconciler) start(ctx context.Context) error {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
	return nil
}

func (r *NodeK8sReconciler) Do(ctx context.Context, message *nodeQueueMessage) (commonctrl.Result, error) {
	adminNode := new(v1.Node)
	err := r.Get(ctx, apitypes.NamespacedName{Name: message.adminNodeName}, adminNode)
	if err != nil {
		return commonctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !adminNode.GetDeletionTimestamp().IsZero() {
		return commonctrl.Result{}, nil
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
		return commonctrl.Result{}, ignoreError(err)
	}
	return commonctrl.Result{}, nil
}

func (r *NodeK8sReconciler) handleNodeUnmanaged(ctx context.Context, message *nodeQueueMessage, adminNode *v1.Node) error {
	clusterName := message.clusterName
	workspaceId := v1.GetWorkspaceId(adminNode)
	deleteConcernedMeta(adminNode)
	if err := r.Update(ctx, adminNode); err != nil {
		klog.ErrorS(err, "failed to update node")
		return err
	}

	patch := client.MergeFrom(adminNode.DeepCopy())
	adminNode.Status = v1.NodeStatus{
		ClusterStatus: v1.NodeClusterStatus{
			Phase: v1.NodeUnmanaged,
		},
	}
	if err := r.Status().Patch(ctx, adminNode, patch); err != nil {
		return err
	}
	klog.Infof("reset adminNode metadata and status, name: %s, cluster: %s, workspace: %s",
		adminNode.Name, clusterName, workspaceId)
	return nil
}

func (r *NodeK8sReconciler) handleNodeUpdate(ctx context.Context, message *nodeQueueMessage, adminNode *v1.Node) error {
	k8sClients, err := getK8sClientFactory(r.clientManager, message.clusterName)
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
	if err = r.syncK8sStatus(ctx, adminNode, k8sNode, message); err != nil {
		return err
	}
	if err = r.handleFault(ctx, adminNode, message); err != nil {
		return err
	}
	return nil
}

func (r *NodeK8sReconciler) syncK8sMetadata(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) error {
	isShouldUpdate := false
	for _, k := range concernedK8sLabelKeys {
		if v, ok := k8sNode.Labels[k]; ok {
			if adminNode.Labels[k] != v {
				metav1.SetMetaDataLabel(&adminNode.ObjectMeta, k, v)
				isShouldUpdate = true
			}
		} else {
			delete(adminNode.Labels, k)
			isShouldUpdate = true
		}
	}

	for _, k := range concernedK8sAnnotationKeys {
		if v, ok := k8sNode.Annotations[k]; ok {
			if adminNode.Annotations[k] != v {
				metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, k, v)
				isShouldUpdate = true
			}
		} else {
			delete(adminNode.Annotations, k)
			isShouldUpdate = true
		}
	}

	if !isShouldUpdate {
		return nil
	}
	if err := r.Update(ctx, adminNode); err != nil {
		klog.ErrorS(err, "failed to update node", "name", adminNode.Name)
		return err
	}
	return nil
}

func (r *NodeK8sReconciler) syncK8sStatus(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node, item *nodeQueueMessage) error {
	originNode := adminNode.DeepCopy()
	adminNode.Status.MachineStatus.PrivateIP = commonnodes.GetInternalIp(k8sNode)
	adminNode.Status.Unschedulable = k8sNode.Spec.Unschedulable
	adminNode.Status.Taints = k8sNode.Spec.Taints
	if isConditionsChanged(adminNode.Status.Conditions, k8sNode.Status.Conditions) {
		adminNode.Status.Conditions = k8sNode.Status.Conditions
	}
	adminNode.Status.Resources = quantity.GetConcernedResources(k8sNode.Status.Allocatable)
	if !reflect.DeepEqual(originNode.Status, adminNode.Status) {
		if err := r.Status().Update(ctx, adminNode); err != nil {
			klog.ErrorS(err, "failed to update node status", "name", adminNode.Name)
			return err
		}
	}
	return nil
}

func (r *NodeK8sReconciler) handleFault(ctx context.Context, adminNode *v1.Node, message *nodeQueueMessage) error {
	faultConfigMap, err := getFaultConfigmap(ctx, r.Client)
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
		if isShouldCreateFault(newCondition) {
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

func deleteConcernedMeta(adminNode *v1.Node) {
	for _, k := range concernedK8sLabelKeys {
		delete(adminNode.Labels, k)
	}
	for _, k := range concernedK8sAnnotationKeys {
		delete(adminNode.Annotations, k)
	}
}

func isConcernedLabelsEqual(obj1, obj2 metav1.Object) bool {
	if maps.CompareWithKeys(obj1.GetLabels(), obj2.GetLabels(), concernedK8sLabelKeys) &&
		maps.CompareWithKeys(obj1.GetAnnotations(), obj2.GetAnnotations(), concernedK8sAnnotationKeys) {
		return true
	}
	return false
}

func genFaultNodeByMessage(msg *nodeQueueMessage) *v1.FaultNode {
	return &v1.FaultNode{
		K8sName:     msg.k8sNodeName,
		AdminName:   msg.adminNodeName,
		ClusterName: msg.clusterName,
	}
}
