/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	harborCACertPathUbuntu = "/usr/local/share/ca-certificates/harbor-ca.crt"
	harborCACertPathCentOS = "/etc/pki/ca-trust/source/anchors/harbor-ca.crt"
)

type NodeReconciler struct {
	*ClusterBaseReconciler
	clientManager *commonutils.ObjectManager
}

// SetupNodeController: initializes and registers the NodeReconciler with the controller manager
func SetupNodeController(mgr manager.Manager) error {
	r := &NodeReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		clientManager: commonutils.NewObjectManagerSingleton(),
	}
	if r.clientManager == nil {
		return fmt.Errorf("failed to new clientManager")
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Node{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, r.relevantChangePredicate()))).
		Watches(&corev1.Pod{}, r.handlePodEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Node Controller successfully")
	return nil
}

// relevantChangePredicate: defines which Node changes should trigger reconciliation
func (r *NodeReconciler) relevantChangePredicate() predicate.Predicate {
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
			return r.isNodeRelevantFieldChanged(oldNode, newNode)
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}
}

// isNodeRelevantFieldChanged: checks if any watched fields in the Node have changed
func (r *NodeReconciler) isNodeRelevantFieldChanged(oldNode, newNode *v1.Node) bool {
	if v1.GetClusterId(oldNode) != v1.GetClusterId(newNode) ||
		v1.GetWorkspaceId(oldNode) != v1.GetWorkspaceId(newNode) ||
		oldNode.Status.MachineStatus.Phase != newNode.Status.MachineStatus.Phase ||
		oldNode.Status.ClusterStatus.Phase != newNode.Status.ClusterStatus.Phase ||
		(v1.GetNodeLabelAction(oldNode) == "" && v1.GetNodeLabelAction(newNode) != "") ||
		(v1.GetNodeAnnotationAction(oldNode) == "" && v1.GetNodeAnnotationAction(newNode) != "") ||
		oldNode.GetDeletionTimestamp().IsZero() && !newNode.GetDeletionTimestamp().IsZero() ||
		commonfaults.HasPrimusSafeTaint(oldNode.Status.Taints) && !commonfaults.HasPrimusSafeTaint(newNode.Status.Taints) {
		return true
	}
	return false
}

// handlePodEvent: creates an event handler that enqueues Node requests when related Pods change
func (r *NodeReconciler) handlePodEvent() handler.EventHandler {
	enqueue := func(pod *corev1.Pod, q v1.RequestWorkQueue) {
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
		CreateFunc: func(ctx context.Context, event event.CreateEvent, q v1.RequestWorkQueue) {
			if pod, ok := event.Object.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		UpdateFunc: func(ctx context.Context, event event.UpdateEvent, q v1.RequestWorkQueue) {
			if pod, ok := event.ObjectNew.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		DeleteFunc: func(ctx context.Context, event event.DeleteEvent, q v1.RequestWorkQueue) {
			if pod, ok := event.Object.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		GenericFunc: nil,
	}
}

// Reconcile is the main control loop for Node resources
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
		if client.IgnoreNotFound(err) != nil {
			klog.ErrorS(err, "failed to get k8s node")
		}
		return result, err
	}
	if quit, err := r.observe(ctx, adminNode, k8sNode); quit || err != nil {
		return ctrlruntime.Result{}, err
	}
	return r.processNode(ctx, adminNode, k8sNode)
}

// delete: handles Node deletion by removing the finalizer
func (r *NodeReconciler) delete(ctx context.Context, adminNode *v1.Node) error {
	return utils.RemoveFinalizer(ctx, r.Client, adminNode, v1.NodeFinalizer)
}

// getK8sNode: retrieves the Kubernetes Node object in the data plane associated with the admin Node
func (r *NodeReconciler) getK8sNode(ctx context.Context, adminNode *v1.Node) (*corev1.Node, ctrlruntime.Result, error) {
	clusterName := getClusterId(adminNode)
	k8sNodeName := adminNode.GetK8sNodeName()
	if clusterName == "" || k8sNodeName == "" {
		return nil, ctrlruntime.Result{}, nil
	}
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, clusterName)
	if err != nil || !k8sClients.IsValid() {
		return nil, ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	k8sNode, err := getNodeByInformer(ctx, k8sClients, k8sNodeName)
	return k8sNode, ctrlruntime.Result{}, err
}

// observe: checks if any observed fields have changed and need updating
func (r *NodeReconciler) observe(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (bool, error) {
	if !adminNode.IsReady() || k8sNode == nil {
		return false, nil
	}
	// Observe whether the node has changed. If no changes are detected (ultimately returning true), exit NodeReconciler directly.
	functions := []func(context.Context, *v1.Node, *corev1.Node) (bool, error){
		r.observeTaints, r.observeLabelAction, r.observeAnnotationAction, r.observeWorkspace, r.observeCluster,
	}
	for _, f := range functions {
		ok, err := f(ctx, adminNode, k8sNode)
		if !ok || err != nil {
			return false, err
		}
	}
	return true, nil
}

// observeTaints: checks if taints need to be synchronized
func (r *NodeReconciler) observeTaints(_ context.Context, adminNode *v1.Node, _ *corev1.Node) (bool, error) {
	var statusTaints []corev1.Taint
	for i, t := range adminNode.Status.Taints {
		if strings.HasPrefix(t.Key, v1.PrimusSafePrefix) {
			statusTaints = append(statusTaints, adminNode.Status.Taints[i])
		}
	}
	return commonfaults.IsTaintsEqualIgnoreOrder(adminNode.Spec.Taints, statusTaints), nil
}

// observeLabelAction: checks if label actions need to be processed
func (r *NodeReconciler) observeLabelAction(_ context.Context, adminNode *v1.Node, _ *corev1.Node) (bool, error) {
	if v1.GetNodeLabelAction(adminNode) == "" {
		return true, nil
	}
	return false, nil
}

// observeAnnotationAction: checks if annotation actions need to be processed
func (r *NodeReconciler) observeAnnotationAction(_ context.Context, adminNode *v1.Node, _ *corev1.Node) (bool, error) {
	if v1.GetNodeAnnotationAction(adminNode) == "" {
		return true, nil
	}
	return false, nil
}

// observeWorkspace: checks if workspace information needs to be synchronized
func (r *NodeReconciler) observeWorkspace(_ context.Context, adminNode *v1.Node, _ *corev1.Node) (bool, error) {
	if adminNode.GetSpecWorkspace() == v1.GetWorkspaceId(adminNode) {
		return true, nil
	}
	return false, nil
}

// observeCluster: checks if cluster information needs to be synchronized
func (r *NodeReconciler) observeCluster(_ context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (bool, error) {
	if adminNode.GetSpecCluster() != v1.GetClusterId(adminNode) {
		return false, nil
	}
	if adminNode.GetSpecCluster() != "" {
		if !adminNode.IsManaged() || k8sNode == nil || v1.GetClusterId(k8sNode) == "" {
			return false, nil
		}
	} else {
		if adminNode.IsManaged() || k8sNode != nil {
			return false, nil
		}
	}
	return true, nil
}

// processNode: handles the main processing logic for a Node
func (r *NodeReconciler) processNode(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	if !adminNode.IsReady() {
		return r.syncMachineStatus(ctx, adminNode)
	}
	if result, err := r.updateK8sNode(ctx, adminNode, k8sNode); err != nil || result.RequeueAfter > 0 {
		if err != nil {
			klog.ErrorS(err, "failed to update k8s node")
		}
		return result, err
	}
	return r.processNodeManagement(ctx, adminNode, k8sNode)
}

// syncMachineStatus: synchronizes the machine status of a Node via SSH
func (r *NodeReconciler) syncMachineStatus(ctx context.Context, node *v1.Node) (ctrlruntime.Result, error) {
	originalNode := client.MergeFrom(node.DeepCopy())
	sshClient, err := utils.GetSSHClient(ctx, r.Client, node)
	if err != nil {
		klog.ErrorS(err, "failed to get client for ssh", "node", node.Name)
		node.Status.MachineStatus.Phase = v1.NodeSSHFailed
		if err = r.Status().Patch(ctx, node, originalNode); err != nil {
			klog.ErrorS(err, "failed to patch status", "node", node.Name)
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{RequeueAfter: time.Second * 30}, nil
	}
	defer sshClient.Close()

	hostname, err := r.syncHostname(node, sshClient)
	if err != nil {
		klog.ErrorS(err, "failed to sync hostname", "node", node.Name)
		node.Status.MachineStatus.Phase = v1.NodeHostnameFailed
		if err = r.Status().Patch(ctx, node, originalNode); err != nil {
			klog.ErrorS(err, "failed to patch status", "node", node.Name)
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{RequeueAfter: time.Second * 30}, nil
	}
	node.Status.MachineStatus.HostName = hostname
	node.Status.MachineStatus.Phase = v1.NodeReady
	if err = r.Status().Patch(ctx, node, originalNode); err != nil {
		klog.ErrorS(err, "failed to patch status", "node", node.Name)
		return ctrlruntime.Result{}, err
	}
	klog.Infof("the node %s is ready", hostname)
	return ctrlruntime.Result{}, nil
}

// syncHostname: synchronizes the hostname of a Node via SSH
func (r *NodeReconciler) syncHostname(node *v1.Node, client *ssh.Client) (string, error) {
	if node.Status.MachineStatus.HostName != "" && node.Status.MachineStatus.HostName == node.GetSpecHostName() {
		return node.Status.MachineStatus.HostName, nil
	}
	hostname, err := getHostname(client)
	if err != nil {
		return "", err
	}
	if node.Spec.Hostname != nil && *node.Spec.Hostname != hostname {
		if err = setHostname(client, *node.Spec.Hostname); err != nil {
			return "", err
		}
		hostname = *node.Spec.Hostname
	}
	if hostname == "" {
		return "", fmt.Errorf("hostname not found for node %s", node.Name)
	}
	return hostname, nil
}

// updateK8sNode: updates the Kubernetes Node object in the data plane with changes from the admin Node
func (r *NodeReconciler) updateK8sNode(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	clusterName := getClusterId(adminNode)
	if k8sNode == nil || clusterName == "" {
		return ctrlruntime.Result{}, nil
	}
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, clusterName)
	if err != nil || !k8sClients.IsValid() {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	functions := []func(adminNode *v1.Node, k8sNode *corev1.Node) bool{
		r.updateK8sNodeTaints, r.updateK8sNodeLabels,
		r.updateK8sNodeAnnotations, r.updateK8sNodeWorkspace,
	}
	shouldUpdate := false
	for _, f := range functions {
		if f(adminNode, k8sNode) {
			shouldUpdate = true
		}
	}
	if shouldUpdate {
		if k8sNode, err = k8sClients.ClientSet().CoreV1().Nodes().Update(ctx, k8sNode, metav1.UpdateOptions{}); err != nil {
			klog.ErrorS(err, "failed to update k8s node")
			return ctrlruntime.Result{}, err
		}
	}
	if err = clearConditions(ctx, k8sClients.ClientSet(), k8sNode); err != nil {
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

// updateK8sNodeTaints: updates taints on the Kubernetes Node in the data plane
func (r *NodeReconciler) updateK8sNodeTaints(adminNode *v1.Node, k8sNode *corev1.Node) bool {
	var reservedTaints []corev1.Taint
	for i, t := range k8sNode.Spec.Taints {
		if !strings.HasPrefix(t.Key, v1.PrimusSafePrefix) {
			reservedTaints = append(reservedTaints, k8sNode.Spec.Taints[i])
		}
	}
	reservedTaints = append(reservedTaints, adminNode.Spec.Taints...)

	if commonfaults.IsTaintsEqualIgnoreOrder(reservedTaints, k8sNode.Spec.Taints) {
		return false
	}
	k8sNode.Spec.Taints = reservedTaints
	klog.Infof("update node taint, name: %s, taints: %v", adminNode.Name, reservedTaints)
	return true
}

// clearConditions: removes node conditions that correspond to removed taints
func clearConditions(ctx context.Context, k8sClient kubernetes.Interface, k8sNode *corev1.Node) error {
	specTaintsSet := sets.NewSet()
	for _, t := range k8sNode.Spec.Taints {
		specTaintsSet.Insert(t.Key)
	}

	shouldUpdate := false
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
		shouldUpdate = true
		klog.Infof("remove node condition, name: %s, type: %s", k8sNode.Name, cond.Type)
	}
	if !shouldUpdate {
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

// updateK8sNodeLabels: updates labels on the Kubernetes Node based on label actions of admin node
func (r *NodeReconciler) updateK8sNodeLabels(adminNode *v1.Node, k8sNode *corev1.Node) bool {
	actionData := v1.GetNodeLabelAction(adminNode)
	getLabels := func(obj metav1.Object) map[string]string {
		return obj.GetLabels()
	}
	if len(k8sNode.Labels) == 0 {
		k8sNode.SetLabels(make(map[string]string))
	}
	return r.updateK8sNodeByAction(adminNode, k8sNode, actionData, getLabels)
}

// updateK8sNodeAnnotations: updates annotations on the Kubernetes Node based on annotation actions of admin node
func (r *NodeReconciler) updateK8sNodeAnnotations(adminNode *v1.Node, k8sNode *corev1.Node) bool {
	actionData := v1.GetNodeAnnotationAction(adminNode)
	getAnnotations := func(obj metav1.Object) map[string]string {
		return obj.GetAnnotations()
	}
	if len(k8sNode.Annotations) == 0 {
		k8sNode.SetAnnotations(make(map[string]string))
	}
	return r.updateK8sNodeByAction(adminNode, k8sNode, actionData, getAnnotations)
}

// updateK8sNodeByAction: updates Node labels or annotations based on action specifications
func (r *NodeReconciler) updateK8sNodeByAction(adminNode *v1.Node, k8sNode *corev1.Node,
	actionData string, getLabels func(obj metav1.Object) map[string]string) bool {
	actionMap := make(map[string]string)
	if err := json.Unmarshal([]byte(actionData), &actionMap); err != nil {
		return false
	}
	k8sNodeLabels := getLabels(k8sNode)
	adminNodeLabels := getLabels(adminNode)
	shouldUpdate := false
	for key, action := range actionMap {
		val, ok := k8sNodeLabels[key]
		if action == v1.NodeActionRemove {
			if ok {
				delete(k8sNodeLabels, key)
				shouldUpdate = true
			}
			delete(adminNodeLabels, key)
		} else if !ok || val != adminNodeLabels[key] {
			k8sNodeLabels[key] = adminNodeLabels[key]
			shouldUpdate = true
		}
	}
	return shouldUpdate
}

// updateK8sNodeWorkspace: updates the workspace label on the Kubernetes Node in the data plane
func (r *NodeReconciler) updateK8sNodeWorkspace(adminNode *v1.Node, k8sNode *corev1.Node) bool {
	workspace := adminNode.GetSpecWorkspace()
	if workspace == v1.GetLabel(k8sNode, v1.WorkspaceIdLabel) {
		return false
	}

	if workspace == "" {
		v1.RemoveLabel(k8sNode, v1.WorkspaceIdLabel)
	} else {
		v1.SetLabel(k8sNode, v1.WorkspaceIdLabel, workspace)
	}
	klog.Infof("update node workspace, node-name: %s, workspace-name: %s", k8sNode.Name, workspace)
	return true
}

func (r *NodeReconciler) processNodeManagement(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	var err error
	var result ctrlruntime.Result
	originalNode := client.MergeFrom(adminNode.DeepCopy())
	if adminNode.GetSpecCluster() != "" {
		if adminNode.IsManaged() && (k8sNode != nil && v1.GetClusterId(k8sNode) != "") {
			return ctrlruntime.Result{}, nil
		}
		if err = r.syncClusterStatus(ctx, adminNode); err != nil {
			return ctrlruntime.Result{RequeueAfter: time.Second * 30}, nil
		}
		result, err = r.manage(ctx, adminNode, k8sNode)
	} else if adminNode.Status.ClusterStatus.Cluster != nil || (k8sNode != nil && v1.GetClusterId(k8sNode) != "") {
		result, err = r.unmanage(ctx, adminNode, k8sNode)
	} else {
		return ctrlruntime.Result{}, nil
	}
	if err != nil {
		klog.ErrorS(err, "failed to handle node", "node", adminNode.Name)
		return ctrlruntime.Result{}, err
	}
	if err = r.Status().Patch(ctx, adminNode, originalNode); err != nil {
		klog.ErrorS(err, "failed to update node status", "node", adminNode.Name)
		return ctrlruntime.Result{}, err
	}
	return result, nil
}

// syncClusterStatus: synchronizes the cluster status of a Node by handling authorization and certificate installation
func (r *NodeReconciler) syncClusterStatus(ctx context.Context, node *v1.Node) error {
	if node.IsManaged() {
		return nil
	}

	// Handle cluster authorization
	if err := r.handleClusterAuthorization(ctx, node); err != nil {
		return err
	}

	// Handle Harbor certificate installation
	if err := r.handleHarborCertificate(ctx, node); err != nil {
		return err
	}

	node.Status.ClusterStatus.Cluster = node.Spec.Cluster
	if node.IsReady() {
		node.Status.ClusterStatus.Phase = v1.NodeReady
	} else {
		node.Status.ClusterStatus.Phase = v1.NodeNotReady
	}
	return nil
}

// handleClusterAuthorization: handles SSH authorization for cluster access
func (r *NodeReconciler) handleClusterAuthorization(ctx context.Context, node *v1.Node) error {
	if isCommandSuccessful(node.Status.ClusterStatus.CommandStatus, utils.Authorize) {
		return nil
	}

	sshClient, err := utils.GetSSHClient(ctx, r.Client, node)
	if err != nil {
		klog.ErrorS(err, "failed to get client for ssh")
		return err
	}
	defer sshClient.Close()

	if err = r.authorizeClusterAccess(ctx, node, sshClient); err != nil {
		klog.ErrorS(err, "failed to authorize node", "node", node.Name)
		node.Status.ClusterStatus.CommandStatus =
			setCommandStatus(node.Status.ClusterStatus.CommandStatus, utils.Authorize, v1.CommandFailed)
		return err
	}

	klog.Infof("node %s is Authorized", node.Name)
	node.Status.ClusterStatus.CommandStatus =
		setCommandStatus(node.Status.ClusterStatus.CommandStatus, utils.Authorize, v1.CommandSucceeded)
	return nil
}

// handleHarborCertificate: handles Harbor CA certificate installation on the node
func (r *NodeReconciler) handleHarborCertificate(ctx context.Context, node *v1.Node) error {
	if isCommandSuccessful(node.Status.ClusterStatus.CommandStatus, HarborCA) {
		return nil
	}

	sshClient, err := utils.GetSSHClient(ctx, r.Client, node)
	if err != nil {
		klog.ErrorS(err, "failed to get client for ssh")
		return err
	}
	defer sshClient.Close()

	ok, err := r.installHarborCert(ctx, sshClient)
	if err != nil {
		klog.ErrorS(err, "failed to harbor ca ", "node", node.Name)
		node.Status.ClusterStatus.CommandStatus =
			setCommandStatus(node.Status.ClusterStatus.CommandStatus, HarborCA, v1.CommandFailed)
		return err
	}
	if ok {
		node.Status.ClusterStatus.CommandStatus =
			setCommandStatus(node.Status.ClusterStatus.CommandStatus, HarborCA, v1.CommandSucceeded)
		return nil
	}
	return nil
}

// authorizeClusterAccess: sets up SSH authorization for cluster access
func (r *NodeReconciler) authorizeClusterAccess(ctx context.Context, node *v1.Node, sshClient *ssh.Client) error {
	if node.GetSpecCluster() == "" {
		return nil
	}
	cluster, err := r.getCluster(ctx, node.GetSpecCluster())
	if err != nil {
		return err
	}

	shouldAuthorize, secret, err := isNeedAuthorization(ctx, r.Client, node, cluster)
	if err != nil || !shouldAuthorize {
		return err
	}

	username, err := r.getUsername(ctx, node, cluster)
	if err != nil {
		username = string(secret.Data[utils.Username])
	}
	hasAuthorized, err := isAlreadyAuthorized(username, secret, sshClient)
	if err != nil || hasAuthorized {
		return err
	}

	pub := string(secret.Data[utils.AuthorizePub])
	var cmd string
	if username == "" || username == "root" {
		cmd = fmt.Sprintf("echo '\n %s' >> /root/.ssh/authorized_keys", pub)
	} else {
		cmd = fmt.Sprintf("mkdir -p /home/%s/.ssh && sudo chmod -R 700 /home/%s/.ssh && sudo echo '\n %s' >> /home/%s/.ssh/authorized_keys && sudo chmod -R 600 /home/%s/.ssh/authorized_keys",
			username, username, pub, username, username)
	}
	if err = r.executeSSHCommand(sshClient, cmd); err != nil {
		return err
	}
	return nil
}

// manage: handles the process of managing a Node in a Kubernetes cluster
func (r *NodeReconciler) manage(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	// if the Kubernetes node is already present, it means the node has been successfully managed.
	if k8sNode != nil {
		k8sClients, err := utils.GetK8sClientFactory(r.clientManager, adminNode.GetSpecCluster())
		if err != nil || !k8sClients.IsValid() {
			return ctrlruntime.Result{RequeueAfter: time.Second}, nil
		}
		if err = r.syncLabelsToK8sNode(ctx, k8sClients.ClientSet(), adminNode, k8sNode); err != nil {
			return ctrlruntime.Result{}, err
		}
		if err = r.installAddons(ctx, adminNode); err != nil {
			return ctrlruntime.Result{}, err
		}
		if err = r.deleteScaleUpPods(ctx, adminNode.GetSpecCluster(), adminNode.Name); err != nil {
			return ctrlruntime.Result{}, err
		}
		adminNode.Status.ClusterStatus.Phase = v1.NodeManaged
		klog.Infof("managed node %s", k8sNode.Name)
		if stringutil.StrCaseEqual(v1.GetLabel(adminNode, v1.NodeManageRebootLabel), v1.TrueStr) {
			r.rebootNode(ctx, adminNode)
			return ctrlruntime.Result{RequeueAfter: time.Second * 10}, nil
		}
		return ctrlruntime.Result{}, nil
	}
	if isControlPlaneNode(adminNode) {
		return ctrlruntime.Result{}, r.syncControlPlaneNodeStatus(ctx, adminNode)
	}
	return r.syncOrCreateScaleUpPod(ctx, adminNode)
}

// syncControlPlaneNodeStatus: synchronizes the status of control plane nodes
func (r *NodeReconciler) syncControlPlaneNodeStatus(ctx context.Context, adminNode *v1.Node) error {
	pods, err := r.listPod(ctx, adminNode.GetSpecCluster(), "", string(v1.ClusterCreateAction))
	if err != nil {
		return err
	}
	if len(pods) > 0 && pods[0].Status.Phase == corev1.PodFailed {
		adminNode.Status.ClusterStatus.Phase = v1.NodeManagedFailed
	} else {
		adminNode.Status.ClusterStatus.Phase = v1.NodeManaging
	}
	return nil
}

// syncLabelsToK8sNode: synchronizes labels from the admin Node to the Kubernetes Node
func (r *NodeReconciler) syncLabelsToK8sNode(ctx context.Context,
	clientSet kubernetes.Interface, adminNode *v1.Node, k8sNode *corev1.Node) error {
	labels := map[string]string{}
	for k, v := range adminNode.Labels {
		if k == v1.DisplayNameLabel {
			continue
		}
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

// syncOrCreateScaleUpPod: synchronizes or creates a scale-up Pod for the Node when managing
func (r *NodeReconciler) syncOrCreateScaleUpPod(ctx context.Context, adminNode *v1.Node) (ctrlruntime.Result, error) {
	pods, err := r.listPod(ctx, adminNode.GetSpecCluster(), adminNode.Name, string(v1.ClusterScaleUpAction))
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if len(pods) == 0 {
		cluster, err := r.getCluster(ctx, adminNode.GetSpecCluster())
		if err != nil || cluster == nil {
			return ctrlruntime.Result{RequeueAfter: time.Second}, err
		}
		username, err := r.getUsername(ctx, adminNode, cluster)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		hostsContent, err := r.generateHosts(ctx, cluster, adminNode)
		if err != nil || hostsContent == nil {
			return ctrlruntime.Result{}, err
		}
		if _, err = r.guaranteeHostsConfigMapCreated(ctx, adminNode.Name,
			genNodeOwnerReference(adminNode), hostsContent); err != nil {
			return ctrlruntime.Result{}, err
		}
		cmd := getKubeSprayScaleUpCMD(username, adminNode.Name, getKubeSprayEnv(cluster))
		pod := generateScaleWorkerPod(v1.ClusterScaleUpAction, cluster, adminNode, username,
			cmd, getKubesprayImage(cluster), adminNode.Name, hostsContent)

		if err = r.Create(ctx, pod); err != nil {
			return ctrlruntime.Result{}, err
		}
		klog.Infof("kubernetes cluster %s begins to manage %s, pod: %s",
			cluster.Name, adminNode.Name, pod.Name)
		adminNode.Status.ClusterStatus.Phase = v1.NodeManaging
	} else {
		pod := pods[0]
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			klog.Infof("pod(%s) is succeeded", pod.Name)
			return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
		case corev1.PodFailed:
			adminNode.Status.ClusterStatus.Phase = v1.NodeManagedFailed
		default:
			adminNode.Status.ClusterStatus.Phase = v1.NodeManaging
		}
	}
	return ctrlruntime.Result{}, nil
}

// unmanage: handles the process of unmanaging a Node from a Kubernetes cluster
func (r *NodeReconciler) unmanage(ctx context.Context, adminNode *v1.Node, k8sNode *corev1.Node) (ctrlruntime.Result, error) {
	if isControlPlaneNode(adminNode) {
		return ctrlruntime.Result{}, nil
	}
	// Waiting for taint to disappear and workspace to be successfully unbound
	if commonfaults.HasPrimusSafeTaint(adminNode.Status.Taints) || v1.GetWorkspaceId(adminNode) != "" {
		return ctrlruntime.Result{}, nil
	}

	clusterId := ""
	if adminNode.Status.ClusterStatus.Cluster != nil {
		clusterId = *adminNode.Status.ClusterStatus.Cluster
	} else {
		clusterId = v1.GetClusterId(k8sNode)
	}

	if k8sNode == nil {
		if pods, err := r.listPod(ctx, clusterId, adminNode.Name, string(v1.ClusterScaleDownAction)); err != nil {
			return ctrlruntime.Result{}, err
		} else if len(pods) > 0 {
			if err = r.Delete(ctx, &pods[0]); err != nil {
				return ctrlruntime.Result{}, err
			}
			klog.Infof("delete pod(%s) for scaleDown", pods[0].Name)
		}
		adminNode.Status = v1.NodeStatus{
			ClusterStatus: v1.NodeClusterStatus{
				Phase: v1.NodeUnmanaged,
			},
		}
		klog.Infof("node %s is unmanaged", adminNode.Name)
		if stringutil.StrCaseEqual(v1.GetLabel(adminNode, v1.NodeUnmanageNoRebootLabel), v1.TrueStr) {
			return ctrlruntime.Result{}, nil
		}
		r.rebootNode(ctx, adminNode)
		// The node will be rebooted. Need to retry getting the node status later
		return ctrlruntime.Result{RequeueAfter: time.Second * 10}, nil
	}

	// delete all scaleup pod when doing scaledown
	if err := r.deleteScaleUpPods(ctx, clusterId, adminNode.Name); err != nil {
		return ctrlruntime.Result{}, err
	}

	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, clusterId)
	if err != nil || !k8sClients.IsValid() {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	return r.syncOrCreateScaleDownPod(ctx, k8sClients.ClientSet(), adminNode, k8sNode, clusterId)
}

// rebootNode: reboots the Node via SSH
func (r *NodeReconciler) rebootNode(ctx context.Context, node *v1.Node) {
	sshClient, err := utils.GetSSHClient(ctx, r.Client, node)
	if err != nil {
		klog.ErrorS(err, "failed to get ssh client", "node", node.Name)
		return
	}
	defer sshClient.Close()

	cmd := "sudo reboot"
	if err = r.executeSSHCommand(sshClient, cmd); err != nil {
		return
	}
	klog.Infof("machine node %s reboot", node.Name)
}

// syncOrCreateScaleDownPod: synchronizes or creates a scale-down Pod for the Node when unmanaging
func (r *NodeReconciler) syncOrCreateScaleDownPod(ctx context.Context,
	clientSet kubernetes.Interface, adminNode *v1.Node, k8sNode *corev1.Node, clusterId string) (ctrlruntime.Result, error) {
	cluster, err := r.getCluster(ctx, clusterId)
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	hostname := adminNode.Status.MachineStatus.HostName
	pods, err := r.listPod(ctx, cluster.Name, hostname, string(v1.ClusterScaleDownAction))
	if err != nil {
		return ctrlruntime.Result{}, err
	}

	adminNode.Status.ClusterStatus.Phase = v1.NodeUnmanaging
	if len(pods) == 0 {
		username, err := r.getUsername(ctx, adminNode, cluster)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		hostsContent, err := r.generateHosts(ctx, cluster, adminNode)
		if err != nil || hostsContent == nil {
			return ctrlruntime.Result{}, err
		}
		if _, err = r.guaranteeHostsConfigMapCreated(ctx, adminNode.Name,
			genNodeOwnerReference(adminNode), hostsContent); err != nil {
			return ctrlruntime.Result{}, err
		}
		pod := generateScaleWorkerPod(v1.ClusterScaleDownAction, cluster, adminNode, username,
			getKubeSprayScaleDownCMD(username, hostname, getKubeSprayEnv(cluster)),
			getKubesprayImage(cluster), adminNode.Name, hostsContent)
		if err = r.Create(ctx, pod); err != nil {
			return ctrlruntime.Result{}, err
		}
		klog.Infof("kubernetes cluster %s begins to unmanage %s, pod: %s",
			cluster.Name, adminNode.Name, pod.Name)
	} else {
		pod := pods[0]
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			klog.Infof("the pod(%s) is succeeded", pod.Name)
			return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
		case corev1.PodFailed:
			if !isK8sNodeReady(k8sNode) {
				if err = clientSet.CoreV1().Nodes().Delete(ctx, k8sNode.Name, metav1.DeleteOptions{}); err != nil {
					return ctrlruntime.Result{}, err
				}
				return ctrlruntime.Result{RequeueAfter: time.Second * 3}, nil
			} else {
				adminNode.Status.ClusterStatus.Phase = v1.NodeUnmanagedFailed
			}
		default:
		}
	}
	return ctrlruntime.Result{}, nil
}

// getClusterId: retrieves the cluster ID for a Node
func getClusterId(adminNode *v1.Node) string {
	clusterId := adminNode.GetSpecCluster()
	if clusterId == "" {
		clusterId = v1.GetClusterId(adminNode)
	}
	return clusterId
}

// installHarborCert: installs the Harbor CA certificate on the Node
func (r *NodeReconciler) installHarborCert(ctx context.Context, sshClient *ssh.Client) (bool, error) {
	secret := new(corev1.Secret)
	err := r.Get(ctx, types.NamespacedName{
		Namespace: "harbor",
		Name:      "harbor-tls",
	}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	ca, ok := secret.Data["ca.crt"]
	if !ok {
		return false, nil
	}
	ubuntu := fmt.Sprintf("sudo touch %s && sudo chmod -R 666 %s && sudo echo \"%s\" > %s && sudo update-ca-certificates",
		harborCACertPathUbuntu, harborCACertPathUbuntu, string(ca), harborCACertPathUbuntu)
	centos := fmt.Sprintf("sudo touch %s && sudo chmod -R 666 %s && sudo echo \"%s\" > %s && sudo update-ca-trust",
		harborCACertPathCentOS, harborCACertPathCentOS, string(ca), harborCACertPathCentOS)
	cmd := fmt.Sprintf("command -v update-ca-certificates >/dev/null 2>&1 && (%s) || (%s)", ubuntu, centos)
	if err = r.executeSSHCommand(sshClient, cmd); err != nil {
		return false, err
	}
	return true, nil
}

// installAddons: installs addons on the Node by creating an OpsJob
func (r *NodeReconciler) installAddons(ctx context.Context, adminNode *v1.Node) error {
	if adminNode.Spec.NodeTemplate == nil || v1.IsNodeTemplateInstalled(adminNode) {
		return nil
	}
	name := string(v1.OpsJobAddonType) + "-" + adminNode.Name
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				v1.ClusterManageActionLabel: string(v1.ClusterScaleUpAction),
				v1.ClusterIdLabel:           adminNode.GetSpecCluster(),
				v1.NodeFlavorIdLabel:        adminNode.GetSpecNodeFlavor(),
				v1.DisplayNameLabel:         name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: common.UserSystem,
			},
			Name: v1.OpsJobKind + "-" + name,
		},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobAddonType,
			Inputs: []v1.Parameter{{
				Name:  v1.ParameterNode,
				Value: adminNode.Name,
			}, {
				Name:  v1.ParameterNodeTemplate,
				Value: adminNode.Spec.NodeTemplate.Name,
			}},
		},
	}
	if err := r.Create(ctx, job); err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create addon job(%s), node.name: %s", job.Name, adminNode.Name)
	return nil
}

// listPod: lists Pods matching the specified criteria
func (r *NodeReconciler) listPod(ctx context.Context, clusterName, nodeName, action string) ([]corev1.Pod, error) {
	labelSelector := client.MatchingLabels{
		v1.ClusterManageClusterLabel: clusterName,
		v1.ClusterManageActionLabel:  action,
	}
	if nodeName != "" {
		labelSelector[v1.ClusterManageNodeLabel] = nodeName
	}
	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// deleteScaleUpPods: deletes all scale-up Pods for the specified node
func (r *NodeReconciler) deleteScaleUpPods(ctx context.Context, clusterId, nodeId string) error {
	pods, err := r.listPod(ctx, clusterId, nodeId, string(v1.ClusterScaleUpAction))
	if err != nil {
		return err
	}
	for _, pod := range pods {
		if err = r.Delete(ctx, &pod); client.IgnoreNotFound(err) != nil {
			return err
		}
		klog.Infof("delete pod(%s) for scaleUp", pod.Name)
	}
	return nil
}

// executeSSHCommand executes a command via SSH on the specified node
func (r *NodeReconciler) executeSSHCommand(sshClient *ssh.Client, command string) error {
	session, err := sshClient.NewSession()
	if err != nil {
		klog.ErrorS(err, "failed to new session")
		return err
	}
	var b bytes.Buffer
	session.Stdout = &b
	defer session.Close()

	if err = session.Run(command); err != nil {
		return fmt.Errorf("failed to execute command '%s': %v", command, err)
	}
	klog.Infof("execute command, result %s", b.String())
	return nil
}
