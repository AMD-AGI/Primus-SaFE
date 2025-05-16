/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

type NodeReconciler struct {
	*ClusterBaseReconciler
	cm *ClusterManager
}

func SetupNodeController(mgr manager.Manager) error {
	r := &NodeReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		cm: newClusterManager(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Node{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, r.CaredPredicate()))).
		Watches(&corev1.Pod{}, r.enqueueRequestByWorkerPod()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Node Controller successfully")
	return nil
}

func (r *NodeReconciler) CaredPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, ok := e.Object.(*v1.Node)
			return ok
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, ok1 := e.ObjectOld.(*v1.Node)
			newNode, ok2 := e.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 {
				return false
			}
			return r.isNodeCaredFieldChanged(oldNode, newNode)
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}
}

func (r *NodeReconciler) isNodeCaredFieldChanged(oldNode, newNode *v1.Node) bool {
	if v1.GetClusterId(oldNode) != v1.GetClusterId(newNode) ||
		v1.GetWorkspaceId(oldNode) != v1.GetWorkspaceId(newNode) ||
		oldNode.Status.MachineStatus.Phase != newNode.Status.MachineStatus.Phase ||
		oldNode.Status.ClusterStatus.Phase != newNode.Status.ClusterStatus.Phase ||
		(v1.GetNodeLabelAction(oldNode) == "" && v1.GetNodeLabelAction(newNode) != "") ||
		(v1.GetNodeAnnotationAction(oldNode) == "" && v1.GetNodeAnnotationAction(newNode) != "") ||
		(len(oldNode.Status.Taints) != 0 && len(newNode.Status.Taints) == 0) {
		return true
	}
	return false
}

func (r *NodeReconciler) enqueueRequestByWorkerPod() handler.EventHandler {
	enqueue := func(pod *corev1.Pod, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		for _, owner := range pod.OwnerReferences {
			if owner.APIVersion == v1.SchemeGroupVersion.String() && owner.Kind == v1.NodeKind {
				q.Add(ctrlruntime.Request{
					NamespacedName: apitypes.NamespacedName{
						Name: owner.Name,
					},
				})
			}
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, event event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if pod, ok := event.Object.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		UpdateFunc: func(ctx context.Context, event event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if pod, ok := event.ObjectNew.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		DeleteFunc: func(ctx context.Context, event event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if pod, ok := event.Object.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		GenericFunc: nil,
	}
}

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished %s reconcile %s, cost (%v)", v1.NodeKind, req.Name, time.Since(startTime))
	}()

	adminNode := new(v1.Node)
	if err := r.Get(ctx, req.NamespacedName, adminNode); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !adminNode.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, adminNode)
	}
	k8sNode, result, err := r.getK8sNode(ctx, adminNode)
	if client.IgnoreNotFound(err) != nil || result.RequeueAfter > 0 {
		return result, err
	}
	if quit, err := r.observe(adminNode, k8sNode); quit || err != nil {
		return ctrlruntime.Result{}, err
	}
	return r.handle(ctx, adminNode, k8sNode)
}

func (r *NodeReconciler) delete(ctx context.Context, adminNode *v1.Node) error {
	return removeFinalizer(ctx, r.Client, adminNode, v1.NodeFinalizer)
}

func (r *NodeReconciler) getK8sNode(ctx context.Context, adminNode *v1.Node) (*corev1.Node, ctrlruntime.Result, error) {
	clusterName := getClusterId(adminNode)
	k8sNodeName := adminNode.GetK8sNodeName()
	if clusterName == "" || k8sNodeName == "" {
		return nil, ctrlruntime.Result{}, nil
	}
	informer := r.cm.getInformer(clusterName)
	if informer == nil || !informer.IsValid() {
		return nil, ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	k8sNode, err := getNodeByInformer(ctx, informer, k8sNodeName)
	return k8sNode, ctrlruntime.Result{}, err
}

func (r *NodeReconciler) observe(adminNode *v1.Node, k8sNode *corev1.Node) (bool, error) {
	if !adminNode.IsReady() || k8sNode == nil {
		return false, nil
	}
	// Observe whether the node has changed. If no changes are detected (ultimately returning true), exit NodeReconciler directly.
	functions := []func(*v1.Node) (bool, error){
		r.observeTaints, r.observeLabelAction, r.observeAnnotationAction, r.observeWorkspace, r.observeCluster,
	}
	for _, f := range functions {
		ok, err := f(adminNode)
		if !ok || err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *NodeReconciler) observeTaints(adminNode *v1.Node) (bool, error) {
	var taints []corev1.Taint
	for i, t := range adminNode.Status.Taints {
		if strings.HasPrefix(t.Key, v1.PrimusSafePrefix) {
			taints = append(taints, adminNode.Status.Taints[i])
		}
	}
	return faults.IsTaintsEqualIgnoreOrder(adminNode.Spec.Taints, taints), nil
}

func (r *NodeReconciler) observeLabelAction(adminNode *v1.Node) (bool, error) {
	if v1.GetNodeLabelAction(adminNode) == "" {
		return true, nil
	}
	return false, nil
}

func (r *NodeReconciler) observeAnnotationAction(adminNode *v1.Node) (bool, error) {
	if v1.GetNodeAnnotationAction(adminNode) == "" {
		return true, nil
	}
	return false, nil
}

func (r *NodeReconciler) observeWorkspace(adminNode *v1.Node) (bool, error) {
	if adminNode.GetSpecWorkspace() == v1.GetWorkspaceId(adminNode) {
		return true, nil
	}
	return false, nil
}

func (r *NodeReconciler) observeCluster(adminNode *v1.Node) (bool, error) {
	if adminNode.GetSpecCluster() == v1.GetClusterId(adminNode) {
		return true, nil
	}
	return false, nil
}

func (r *NodeReconciler) handle(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	if !adminNode.IsReady() {
		return r.syncMachineStatus(ctx, adminNode)
	}
	if result, err := r.updateK8sNode(ctx, adminNode, k8sNode); err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	return r.updateAdminNode(ctx, adminNode, k8sNode)
}

func (r *NodeReconciler) syncMachineStatus(ctx context.Context, node *v1.Node) (ctrlruntime.Result, error) {
	n := client.MergeFrom(node.DeepCopy())
	sshClient, err := getSSHClient(ctx, r.Client, node)
	if err != nil {
		klog.ErrorS(err, "failed to get client for ssh")
		node.Status.MachineStatus.Phase = v1.NodeSSHFailed
		if err = r.Status().Patch(ctx, node, n); err != nil {
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{RequeueAfter: time.Second * 30}, nil
	}
	hostname, err := r.syncHostname(node, sshClient)
	if err != nil {
		klog.ErrorS(err, "failed to sync hostname", "node", node.Name)
		node.Status.MachineStatus.Phase = v1.NodeHostnameFailed
		if err = r.Status().Patch(ctx, node, n); err != nil {
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{RequeueAfter: time.Second * 30}, nil
	}
	node.Status.MachineStatus.HostName = hostname
	node.Status.MachineStatus.Phase = v1.NodeReady
	if err = r.Status().Patch(ctx, node, n); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

func (r *NodeReconciler) syncHostname(node *v1.Node, client *ssh.Client) (string, error) {
	if node.Status.MachineStatus.HostName != "" {
		return node.Status.MachineStatus.HostName, nil
	}
	hostname, err := getHostname(client)
	if err != nil {
		return "", err
	}
	if node.Spec.Hostname != nil && *node.Spec.Hostname != hostname {
		hostname, err = setHostname(client, *node.Spec.Hostname)
		if err != nil {
			return "", err
		}
	}
	if hostname == "" {
		return "", fmt.Errorf("hostname not found for node %s", node.Name)
	}
	return hostname, nil
}

func (r *NodeReconciler) updateK8sNode(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	clusterName := getClusterId(adminNode)
	if k8sNode == nil || clusterName == "" {
		return ctrlruntime.Result{}, nil
	}

	informer := r.cm.getInformer(clusterName)
	if informer == nil || !informer.IsValid() {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	functions := []func(adminNode *v1.Node, k8sNode *corev1.Node) bool{
		r.updateK8sNodeTaints, r.updateK8sNodeLabels,
		r.updateK8sNodeAnnotations, r.updateK8sNodeWorkspace,
	}
	isShouldUpdate := false
	for _, f := range functions {
		if f(adminNode, k8sNode) {
			isShouldUpdate = true
		}
	}
	var err error
	if isShouldUpdate {
		if k8sNode, err = informer.GetClient().CoreV1().Nodes().Update(ctx, k8sNode, metav1.UpdateOptions{}); err != nil {
			klog.ErrorS(err, "failed to update k8s node")
			return ctrlruntime.Result{}, err
		}
	}
	if err = removeTaintConditions(ctx, informer.GetClient(), k8sNode); err != nil {
		klog.ErrorS(err, "failed to remove taint conditions")
		return ctrlruntime.Result{}, err
	}
	delete(adminNode.Annotations, v1.NodeLabelAction)
	delete(adminNode.Annotations, v1.NodeAnnotationAction)
	if err = r.Update(ctx, adminNode); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

func (r *NodeReconciler) updateK8sNodeTaints(adminNode *v1.Node, k8sNode *corev1.Node) bool {
	var reservedTaints []corev1.Taint
	for i, t := range k8sNode.Spec.Taints {
		if !strings.HasPrefix(t.Key, v1.PrimusSafePrefix) {
			reservedTaints = append(reservedTaints, k8sNode.Spec.Taints[i])
		}
	}
	reservedTaints = append(reservedTaints, adminNode.Spec.Taints...)

	if faults.IsTaintsEqualIgnoreOrder(reservedTaints, k8sNode.Spec.Taints) {
		return false
	}
	k8sNode.Spec.Taints = reservedTaints
	klog.Infof("update node taint, name: %s, taints: %v", adminNode.Name, reservedTaints)
	return true
}

func removeTaintConditions(ctx context.Context,
	k8sClient kubernetes.Interface, k8sNode *corev1.Node) error {
	specTaintsSet := sets.NewSet()
	for _, t := range k8sNode.Spec.Taints {
		specTaintsSet.Insert(t.Key)
	}

	isShouldUpdate := false
	var reservedConditions []corev1.NodeCondition
	for i, cond := range k8sNode.Status.Conditions {
		if !isPrimusCondition(cond.Type) {
			reservedConditions = append(reservedConditions, k8sNode.Status.Conditions[i])
			continue
		}
		key := string(cond.Type)
		if specTaintsSet.Has(key) {
			reservedConditions = append(reservedConditions, k8sNode.Status.Conditions[i])
			continue
		}
		klog.Infof("remove node condition, name: %s, type: %s", k8sNode.Name, cond.Type)
		isShouldUpdate = true
	}
	if !isShouldUpdate {
		return nil
	}

	var err error
	k8sNode.Status.Conditions = reservedConditions
	k8sNode, err = k8sClient.CoreV1().Nodes().UpdateStatus(ctx, k8sNode, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (r *NodeReconciler) updateK8sNodeLabels(adminNode *v1.Node, k8sNode *corev1.Node) bool {
	strAction := v1.GetNodeLabelAction(adminNode)
	getLabels := func(obj metav1.Object) map[string]string {
		return obj.GetLabels()
	}
	if len(k8sNode.Labels) == 0 {
		k8sNode.SetLabels(make(map[string]string))
	}
	return r.updateK8sNodeByAction(adminNode, k8sNode, strAction, getLabels)
}

func (r *NodeReconciler) updateK8sNodeAnnotations(adminNode *v1.Node, k8sNode *corev1.Node) bool {
	strAction := v1.GetNodeAnnotationAction(adminNode)
	getAnnotations := func(obj metav1.Object) map[string]string {
		return obj.GetAnnotations()
	}
	if len(k8sNode.Annotations) == 0 {
		k8sNode.SetAnnotations(make(map[string]string))
	}
	return r.updateK8sNodeByAction(adminNode, k8sNode, strAction, getAnnotations)
}

func (r *NodeReconciler) updateK8sNodeByAction(adminNode *v1.Node, k8sNode *corev1.Node,
	strAction string, getLabels func(obj metav1.Object) map[string]string) bool {
	actionMap := make(map[string]string)
	if err := json.Unmarshal([]byte(strAction), &actionMap); err != nil {
		klog.ErrorS(err, "failed to Unmarshal", "data", strAction)
		return false
	}
	k8sNodeLabels := getLabels(k8sNode)
	adminNodeLabels := getLabels(adminNode)
	isShouldUpdate := false
	for key, action := range actionMap {
		val, ok := k8sNodeLabels[key]
		if action == v1.NodeActionRemove {
			if ok {
				delete(k8sNodeLabels, key)
				isShouldUpdate = true
			}
			delete(adminNodeLabels, key)
		} else if !ok || val != adminNodeLabels[key] {
			k8sNodeLabels[key] = adminNodeLabels[key]
			isShouldUpdate = true
		}
	}
	return isShouldUpdate
}

func (r *NodeReconciler) updateK8sNodeWorkspace(adminNode *v1.Node, k8sNode *corev1.Node) bool {
	workspace := adminNode.GetSpecWorkspace()
	if workspace == v1.GetLabel(k8sNode, v1.WorkspaceIdLabel) {
		return false
	}

	if workspace == "" {
		delete(k8sNode.GetLabels(), v1.WorkspaceIdLabel)
	} else {
		metav1.SetMetaDataLabel(&k8sNode.ObjectMeta, v1.WorkspaceIdLabel, workspace)
	}
	klog.Infof("update node workspace, node-name: %s, workspace-name: %s", k8sNode.Name, workspace)
	return true
}

func (r *NodeReconciler) updateAdminNode(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	var err error
	var result ctrlruntime.Result
	n := client.MergeFrom(adminNode.DeepCopy())
	if adminNode.GetSpecCluster() != "" {
		if adminNode.IsManaged() {
			return ctrlruntime.Result{}, nil
		}
		if err = r.syncClusterStatus(ctx, adminNode); err != nil {
			return ctrlruntime.Result{RequeueAfter: time.Second * 30}, nil
		}
		result, err = r.manage(ctx, adminNode, k8sNode)
	} else if adminNode.Status.ClusterStatus.Cluster != nil {
		result, err = r.unmanage(ctx, adminNode, k8sNode)
	} else {
		return ctrlruntime.Result{}, nil
	}

	if err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	return ctrlruntime.Result{}, r.Status().Patch(ctx, adminNode, n)
}

func (r *NodeReconciler) syncClusterStatus(ctx context.Context, node *v1.Node) error {
	if !isCommandSuccessful(node.Status.ClusterStatus.CommandStatus, Authorize) {
		sshClient, err := getSSHClient(ctx, r.Client, node)
		if err != nil {
			klog.ErrorS(err, "failed to get client for ssh")
			return err
		}
		if err = r.authorizeClusterAccess(ctx, node, sshClient); err != nil {
			klog.ErrorS(err, "failed to authorize node", "node", node.Name)
			node.Status.ClusterStatus.CommandStatus =
				setCommandStatus(node.Status.ClusterStatus.CommandStatus, Authorize, v1.CommandFailed)
			return err
		}
		node.Status.ClusterStatus.CommandStatus =
			setCommandStatus(node.Status.ClusterStatus.CommandStatus, Authorize, v1.CommandSucceeded)
	}
	node.Status.ClusterStatus.Cluster = node.Spec.Cluster
	if node.IsReady() {
		node.Status.ClusterStatus.Phase = v1.NodeReady
	} else {
		node.Status.ClusterStatus.Phase = v1.NodeNotReady
	}
	return nil
}

func (r *NodeReconciler) authorizeClusterAccess(ctx context.Context, node *v1.Node, sshClient *ssh.Client) error {
	if node.GetSpecCluster() == "" {
		return nil
	}
	cluster, err := r.getCluster(ctx, node.GetSpecCluster())
	if err != nil {
		return err
	}

	isShouldAuthorize, secret, err := isNeedAuthorization(ctx, r.Client, node, cluster)
	if err != nil || !isShouldAuthorize {
		return err
	}

	username, err := r.getUsername(ctx, node, cluster)
	if err != nil {
		username = string(secret.Data[Username])
	}
	hasAuthorized, err := isAlreadyAuthorized(username, secret, sshClient)
	if err != nil || hasAuthorized {
		return err
	}

	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	var b bytes.Buffer
	session.Stdout = &b
	pub := string(secret.Data[AuthorizePub])
	var cmd string
	if username == "" || username == "root" {
		cmd = fmt.Sprintf("echo '\n %s' >> /root/.ssh/authorized_keys", pub)
	} else {
		cmd = fmt.Sprintf("mkdir -p /home/%s/.ssh && sudo chmod -R 700 /home/%s/.ssh && sudo echo '\n %s' >> /home/%s/.ssh/authorized_keys && sudo chmod -R 600 /home/%s/.ssh/authorized_keys", username, username, pub, username, username)
	}
	if err = session.Run(cmd); err != nil {
		return fmt.Errorf("failed %s: %v", cmd, err)
	}
	klog.Infof("authorize successfully. node: %s, cmd: %s", node.Name, cmd)
	return nil
}

func (r *NodeReconciler) manage(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	if isControlPlaneNode(adminNode) {
		return ctrlruntime.Result{}, r.syncControlPlaneNodeStatus(ctx, adminNode, k8sNode)
	}
	// if the Kubernetes node is already present, it means the node has been successfully managed.
	if k8sNode != nil {
		informer := r.cm.getInformer(adminNode.GetSpecCluster())
		if informer == nil || !informer.IsValid() {
			return ctrlruntime.Result{RequeueAfter: time.Second}, nil
		}
		if err := r.syncLabelsToK8sNode(ctx, informer.clientSet, adminNode, k8sNode); err != nil {
			return ctrlruntime.Result{}, err
		}
		adminNode.Status.ClusterStatus.Phase = v1.NodeManaged
		return ctrlruntime.Result{}, nil
	}
	return ctrlruntime.Result{}, r.syncOrCreateScaleUpPod(ctx, adminNode)
}

// Synchronize the status of control plane nodes in Kubernetes
func (r *NodeReconciler) syncControlPlaneNodeStatus(ctx context.Context,
	adminNode *v1.Node, k8sNode *corev1.Node) error {
	if k8sNode != nil {
		adminNode.Status.ClusterStatus.Phase = v1.NodeManaged
		return nil
	}
	labelSelector := client.MatchingLabels{
		v1.ClusterManageActionLabel:  string(v1.ClusterCreateAction),
		v1.ClusterManageClusterLabel: adminNode.GetSpecCluster()}
	pod, err := r.getPod(ctx, labelSelector)
	if err != nil {
		return err
	}
	if pod != nil && pod.Status.Phase == corev1.PodFailed {
		adminNode.Status.ClusterStatus.Phase = v1.NodeManagedFailed
	} else {
		adminNode.Status.ClusterStatus.Phase = v1.NodeManaging
	}
	return nil
}

// Synchronize the labels of admin node to k8s node
func (r *NodeReconciler) syncLabelsToK8sNode(ctx context.Context,
	clientSet kubernetes.Interface, adminNode *v1.Node, k8sNode *corev1.Node) error {
	labels := map[string]string{}
	for k, v := range adminNode.Labels {
		if v != k8sNode.Labels[k] {
			labels[k] = v
		}
	}

	v, _ := k8sNode.Labels[v1.ClusterIdLabel]
	if v != adminNode.GetSpecCluster() {
		labels[v1.ClusterIdLabel] = adminNode.GetSpecCluster()
	}
	v, _ = k8sNode.Labels[v1.NodeIdLabel]
	if v != adminNode.Name {
		labels[v1.NodeIdLabel] = adminNode.Name
	}

	if len(labels) == 0 {
		return nil
	}
	data, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": labels,
		},
	})
	if err != nil {
		return err
	}
	_, err = clientSet.CoreV1().Nodes().Patch(ctx,
		k8sNode.Name, apitypes.StrategicMergePatchType, data, metav1.PatchOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (r *NodeReconciler) syncOrCreateScaleUpPod(ctx context.Context, adminNode *v1.Node) error {
	labelSelector := client.MatchingLabels{
		v1.ClusterManageActionLabel:  string(v1.ClusterScaleUpAction),
		v1.ClusterManageClusterLabel: adminNode.GetSpecCluster(),
		v1.ClusterManageNodeLabel:    adminNode.Name,
	}
	pod, err := r.getPod(ctx, labelSelector)
	if err != nil {
		return err
	}
	if pod == nil {
		cluster, err := r.getCluster(ctx, adminNode.GetSpecCluster())
		if err != nil || cluster == nil {
			return err
		}
		username, err := r.getUsername(ctx, adminNode, cluster)
		if err != nil {
			return err
		}
		hostsContent, err := r.generateHosts(ctx, cluster, adminNode)
		if err != nil || hostsContent == nil {
			return err
		}
		if _, err = r.guaranteeHostsConfigMapCreated(ctx, adminNode.Name,
			genNodeOwnerReference(adminNode), hostsContent); err != nil {
			return err
		}
		cmd := getKubeSprayScaleUpCMD(username, adminNode.Name, getKubeSprayEnv(cluster))
		pod = generateScaleWorkerPod(v1.ClusterScaleUpAction, cluster, adminNode, username,
			cmd, getKubesprayImage(cluster), adminNode.Name, hostsContent)

		if err = r.Create(ctx, pod); err != nil {
			return err
		}
	} else {
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			adminNode.Status.ClusterStatus.Phase = v1.NodeManaged
			return r.Delete(ctx, pod)
		case corev1.PodFailed:
			adminNode.Status.ClusterStatus.Phase = v1.NodeManagedFailed
		default:
			adminNode.Status.ClusterStatus.Phase = v1.NodeManaging
		}
	}
	return nil
}

func (r *NodeReconciler) unmanage(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	if isControlPlaneNode(adminNode) {
		return ctrlruntime.Result{}, nil
	}
	// Waiting for taint to disappear and workspace to be successfully unbound
	if len(adminNode.Status.Taints) != 0 || v1.GetWorkspaceId(adminNode) != "" {
		return ctrlruntime.Result{}, nil
	}

	if k8sNode == nil {
		adminNode.Status.ClusterStatus.Cluster = nil
		adminNode.Status.ClusterStatus.CommandStatus = nil
		adminNode.Status.ClusterStatus.Phase = v1.NodeUnmanaged
		r.rebootNode(ctx, adminNode)
		return ctrlruntime.Result{}, nil
	}

	// delete all scaleup pod when do scaledown
	clusterName := *adminNode.Status.ClusterStatus.Cluster
	labelSelector := client.MatchingLabels{v1.ClusterManageActionLabel: string(v1.ClusterScaleUpAction),
		v1.ClusterManageClusterLabel: clusterName, v1.ClusterManageNodeLabel: adminNode.Name}
	pods, err := r.getPodList(ctx, labelSelector)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	for _, pod := range pods {
		if err = r.Delete(ctx, &pod); client.IgnoreNotFound(err) != nil {
			return ctrlruntime.Result{}, err
		}
	}

	informer := r.cm.getInformer(clusterName)
	if informer == nil || !informer.IsValid() {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	return ctrlruntime.Result{}, r.syncOrCreateScaleDownPod(ctx, informer.clientSet, adminNode, k8sNode)
}

func (r *NodeReconciler) rebootNode(ctx context.Context, node *v1.Node) {
	sshClient, err := getSSHClient(ctx, r.Client, node)
	if err != nil {
		klog.Errorf("machine node %s %+v", node.Name, err)
		return
	}
	session, err := sshClient.NewSession()
	if err != nil {
		klog.Errorf("machine node %s new session failed %+v", node.Name, err)
		return
	}
	if err = session.Run("sudo reboot"); err != nil {
		klog.Errorf("machine node %s unmanaged exec reboot failed %+v", node.Name, err)
	}
	node.Status.MachineStatus = v1.MachineStatus{}
}

func (r *NodeReconciler) syncOrCreateScaleDownPod(ctx context.Context,
	clientSet kubernetes.Interface, adminNode *v1.Node, k8sNode *corev1.Node) error {
	cluster, err := r.getCluster(ctx, *adminNode.Status.ClusterStatus.Cluster)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	hostname := adminNode.Status.MachineStatus.HostName
	labelSelector := client.MatchingLabels{v1.ClusterManageActionLabel: string(v1.ClusterScaleDownAction),
		v1.ClusterManageClusterLabel: cluster.Name, v1.ClusterManageNodeLabel: hostname}
	pod, err := r.getPod(ctx, labelSelector)
	if err != nil {
		return err
	}

	adminNode.Status.ClusterStatus.Phase = v1.NodeUnmanaging
	if pod == nil {
		username, err := r.getUsername(ctx, adminNode, cluster)
		if err != nil {
			return err
		}
		hostsContent, err := r.generateHosts(ctx, cluster, adminNode)
		if err != nil || hostsContent == nil {
			return err
		}
		if _, err = r.guaranteeHostsConfigMapCreated(ctx, adminNode.Name,
			genNodeOwnerReference(adminNode), hostsContent); err != nil {
			return err
		}
		pod = generateScaleWorkerPod(v1.ClusterScaleDownAction, cluster, adminNode, username,
			getKubeSprayScaleDownCMD(username, hostname, getKubeSprayEnv(cluster)),
			getKubesprayImage(cluster), adminNode.Name, hostsContent)
		if err = r.Create(ctx, pod); err != nil {
			return err
		}
		klog.Infof("kubernetes cluster %s scale down %s, pod: %s",
			cluster.Name, adminNode.Name, pod.Name)
	} else {
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			adminNode.Status.ClusterStatus.Phase = v1.NodeUnmanaged
			if err = r.Delete(ctx, pod); err != nil {
				return err
			}
		case corev1.PodFailed:
			if !isK8sNodeReady(k8sNode) {
				if err = clientSet.CoreV1().Nodes().Delete(ctx, k8sNode.Name, metav1.DeleteOptions{}); err != nil {
					return err
				}
				if err = r.Delete(ctx, pod); err != nil {
					return err
				}
				adminNode.Status.ClusterStatus.Phase = v1.NodeUnmanaged
			} else {
				adminNode.Status.ClusterStatus.Phase = v1.NodeUnmanagedFailed
			}
		default:
		}
	}
	return nil
}

func getClusterId(adminNode *v1.Node) string {
	clusterId := adminNode.GetSpecCluster()
	if clusterId == "" {
		clusterId = v1.GetClusterId(adminNode)
	}
	return clusterId
}
