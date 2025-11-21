/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

const (
	CICDClusterRoleBindingLabel = "app.kubernetes.io/role-ref"
)

// ClusterReconciler reconciles Cluster resources and manages their lifecycle.
type ClusterReconciler struct {
	*ClusterBaseReconciler
	clientManager *commonutils.ObjectManager
}

// SetupClusterController initializes and registers the ClusterReconciler with the controller manager.
func SetupClusterController(mgr manager.Manager) error {
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		clientManager: commonutils.NewObjectManagerSingleton(),
	}
	if r.clientManager == nil {
		return fmt.Errorf("failed to new clientManager")
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(predicate.Or(
			predicate.ResourceVersionChangedPredicate{}, r.relevantChangePredicate()))).
		Watches(&corev1.Pod{}, r.handlePodEvent()).
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Cluster Controller successfully")
	return nil
}

// relevantChangePredicate defines which cluster events should trigger reconciliation.
func (r *ClusterReconciler) relevantChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			cluster, ok := e.Object.(*v1.Cluster)
			if !ok || !cluster.IsReady() {
				return false
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, ok1 := e.ObjectOld.(*v1.Cluster)
			newCluster, ok2 := e.ObjectNew.(*v1.Cluster)
			if !ok1 || !ok2 {
				return false
			}
			if !oldCluster.IsReady() && newCluster.IsReady() {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

// handleNodeEvent handles node events that may affect cluster reconciliation.
func (r *ClusterReconciler) handleNodeEvent() handler.EventHandler {
	enqueue := func(node *v1.Node, q v1.RequestWorkQueue) {
		for _, owner := range node.OwnerReferences {
			if owner.APIVersion == v1.SchemeGroupVersion.String() && owner.Kind == v1.ClusterKind {
				q.Add(ctrlruntime.Request{
					NamespacedName: types.NamespacedName{
						Name: owner.Name,
					},
				})
			}
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, event event.CreateEvent, q v1.RequestWorkQueue) {
			if node, ok := event.Object.(*v1.Node); ok {
				enqueue(node, q)
			}
		},
		UpdateFunc: func(ctx context.Context, event event.UpdateEvent, q v1.RequestWorkQueue) {
			if node, ok := event.ObjectNew.(*v1.Node); ok {
				enqueue(node, q)
			}
		},
		DeleteFunc: func(ctx context.Context, event event.DeleteEvent, q v1.RequestWorkQueue) {
			if node, ok := event.Object.(*v1.Node); ok {
				enqueue(node, q)
			}
		},
		GenericFunc: nil,
	}
}

// handlePodEvent handles pod events that may affect cluster reconciliation.
func (r *ClusterReconciler) handlePodEvent() handler.EventHandler {
	enqueue := func(pod *corev1.Pod, q v1.RequestWorkQueue) {
		for _, owner := range pod.OwnerReferences {
			if owner.APIVersion == v1.SchemeGroupVersion.String() && owner.Kind == v1.ClusterKind {
				q.Add(ctrlruntime.Request{
					NamespacedName: types.NamespacedName{
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

// Reconcile processes Cluster resources to ensure they are in the desired state.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished reconcile %s cost (%v)", req.Name, time.Since(startTime))
	}()
	cluster := new(v1.Cluster)
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if cluster.Status.ControlPlaneStatus.Phase == v1.DeletedPhase {
		return ctrlruntime.Result{}, r.delete(ctx, cluster)
	}
	if err = r.guaranteeClusterControlPlane(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to guarantee cluster control plane")
		return ctrlruntime.Result{}, err
	}
	if err = r.guaranteeClientFactory(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to guarantee client factory")
		return ctrlruntime.Result{}, err
	}
	// if result, err := r.guaranteeStorage(ctx, cluster); err != nil || result.RequeueAfter > 0 {
	// 	return result, err
	// }
	if result, err := r.guaranteeDefaultAddon(ctx, cluster); err != nil || result.RequeueAfter > 0 {
		klog.ErrorS(err, "failed to guarantee default addon")
		return result, err
	}
	if result, err := r.guaranteePriorityClass(ctx, cluster); err != nil || result.RequeueAfter > 0 {
		klog.ErrorS(err, "failed to guarantee priority class")
		return result, err
	}
	if err = r.guaranteeAllImageSecrets(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to guarantee image secrets")
		return ctrlruntime.Result{}, err
	}
	// Sync CICD ClusterRole from admin plane to data plane (if present)
	if err = r.guaranteeCICDClusterRole(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to guarantee cicd cluster role")
		return ctrlruntime.Result{}, err
	}
	// Ensure ClusterRoleBinding exists and is labeled to reference this role
	if err = r.guaranteeCICDClusterRoleBinding(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to guarantee cicd cluster role binding")
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// delete handles cluster deletion by cleaning up associated resources.
func (r *ClusterReconciler) delete(ctx context.Context, cluster *v1.Cluster) error {
	if err := r.resetNodesOfCluster(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to reset nodes of cluster")
		return err
	}
	if err := r.deletePriorityClass(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to delete priority class")
		return err
	}
	if err := r.deleteAllImageSecrets(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to delete image secret")
		return err
	}
	if err := r.deleteCICDClusterRole(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to delete CICD ClusterRole")
		return err
	}
	if err := r.clientManager.Delete(cluster.Name); err != nil {
		klog.ErrorS(err, "failed to delete cluster clients", "cluster", cluster.Name)
		// During deletion, if the client is not found, the error will be ignored.
	}
	if err := utils.RemoveFinalizer(ctx, r.Client, cluster, v1.ClusterFinalizer); err != nil {
		klog.ErrorS(err, "failed to remove finalizer")
		return err
	}
	return nil
}

// resetNodesOfCluster resets all nodes associated with a cluster after deletion.
func (r *ClusterReconciler) resetNodesOfCluster(ctx context.Context, cluster *v1.Cluster) error {
	req, _ := labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{cluster.Name})
	labelSelector := labels.NewSelector().Add(*req)
	nodeList := &v1.NodeList{}
	if err := r.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		klog.ErrorS(err, "failed to list nodes")
		return err
	}
	for _, n := range nodeList.Items {
		deleteConcernedMeta(&n)
		n.Spec.Cluster = nil
		n.Spec.Workspace = nil
		if err := r.Update(ctx, &n); err != nil {
			klog.ErrorS(err, "failed to update node")
			return err
		}

		n.Status = v1.NodeStatus{}
		if err := r.Status().Update(ctx, &n); err != nil {
			klog.ErrorS(err, "failed to update node status")
			return err
		}
		klog.Infof("reset the node(%s) after the cluster(%s) is deleted.", n.Name, cluster.Name)
	}
	return nil
}

// guaranteeClientFactory ensures a Kubernetes client factory is available for the cluster.
func (r *ClusterReconciler) guaranteeClientFactory(ctx context.Context, cluster *v1.Cluster) error {
	if !cluster.IsReady() || r.clientManager.Has(cluster.Name) {
		return nil
	}
	endpoint, err := commoncluster.GetEndpoint(ctx, r.Client, cluster)
	if err != nil {
		return err
	}
	controlPlane := &cluster.Status.ControlPlaneStatus
	k8sClients, err := commonclient.NewClientFactory(ctx, cluster.Name, endpoint,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, commonclient.EnableInformer)
	if err != nil {
		return err
	}
	r.clientManager.AddOrReplace(cluster.Name, k8sClients)
	klog.Infof("add cluster %s informer, endpoint: %s", cluster.Name, endpoint)
	return nil
}

// guaranteePriorityClass ensures priority classes are created in the cluster.
func (r *ClusterReconciler) guaranteePriorityClass(ctx context.Context, cluster *v1.Cluster) (ctrlruntime.Result, error) {
	if !cluster.IsReady() {
		return ctrlruntime.Result{}, nil
	}
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	clientSet := k8sClients.ClientSet()
	allPriorityClass := genAllPriorityClass(cluster.Name)
	for _, pc := range allPriorityClass {
		_, err = clientSet.SchedulingV1().PriorityClasses().Get(ctx, pc.name, metav1.GetOptions{})
		if err == nil {
			continue
		} else if !apierrors.IsNotFound(err) {
			return ctrlruntime.Result{}, err
		}

		priorityClass := &schedulingv1.PriorityClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: pc.name,
			},
			Value:       pc.value,
			Description: "This priority class should be used for primus-safe job only.",
		}
		if _, err = clientSet.SchedulingV1().PriorityClasses().Create(
			ctx, priorityClass, metav1.CreateOptions{}); err != nil {
			return ctrlruntime.Result{}, err
		}
		klog.Infof("create PriorityClass, name: %s, value: %d", pc.name, pc.value)
	}
	return ctrlruntime.Result{}, nil
}

// deletePriorityClass deletes priority classes from the cluster.
func (r *ClusterReconciler) deletePriorityClass(ctx context.Context, cluster *v1.Cluster) error {
	//
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		// During deletion, if the client is not found, the error will be ignored.
		return nil
	}
	clientSet := k8sClients.ClientSet()
	allPriorityClass := genAllPriorityClass(cluster.Name)
	for _, pc := range allPriorityClass {
		if err = clientSet.SchedulingV1().PriorityClasses().Delete(ctx, pc.name, metav1.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		klog.Infof("delete PriorityClass, name: %s", pc.name)
	}
	return nil
}

// PriorityClass represents a Kubernetes priority class configuration
type PriorityClass struct {
	name  string
	value int32
}

// genAllPriorityClass generates all required priority classes for a cluster.
func genAllPriorityClass(clusterId string) []PriorityClass {
	return []PriorityClass{
		{name: commonutils.GenerateClusterPriorityClass(clusterId, common.HighPriority), value: 10000},
		{name: commonutils.GenerateClusterPriorityClass(clusterId, common.MedPriority), value: -1},
		{name: commonutils.GenerateClusterPriorityClass(clusterId, common.LowPriority), value: -10000},
	}
}

// guaranteeAllImageSecrets ensures image pull secrets are synchronized to the cluster.
func (r *ClusterReconciler) guaranteeAllImageSecrets(ctx context.Context, cluster *v1.Cluster) error {
	if commonconfig.GetImageSecret() == "" || !cluster.IsReady() {
		return nil
	}
	targetNamespace := corev1.NamespaceDefault
	targetName := commonutils.GenerateClusterSecret(cluster.Name, commonconfig.GetImageSecret())
	if err := r.guaranteeImageSecret(ctx, cluster, targetName, targetNamespace); err != nil {
		return err
	}

	targetNamespace = common.PrimusSafeNamespace
	targetName = commonconfig.GetImageSecret()
	if err := r.guaranteeImageSecret(ctx, cluster, targetName, targetNamespace); err != nil {
		return err
	}
	return nil
}

// guaranteeImageSecret ensures a specific image secret is synchronized to the cluster.
func (r *ClusterReconciler) guaranteeImageSecret(ctx context.Context, cluster *v1.Cluster, targetName, targetNamespace string) error {
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		return err
	}
	clientSet := k8sClients.ClientSet()
	adminPlaneSecret, err := r.getAdminImageSecret(ctx)
	if err != nil {
		return err
	}

	dataPlaneSecret, err := clientSet.CoreV1().Secrets(targetNamespace).Get(ctx, targetName, metav1.GetOptions{})
	if err == nil {
		if dataPlaneSecret.UID == adminPlaneSecret.UID {
			return nil
		}
		dataPlaneSecret.Type = adminPlaneSecret.Type
		dataPlaneSecret.Data = adminPlaneSecret.Data
		_, err = clientSet.CoreV1().Secrets(targetNamespace).Update(ctx, dataPlaneSecret, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		klog.Infof("update the %s/%s secret", targetNamespace, targetName)
	} else {
		if err = r.guaranteeNamespace(ctx, clientSet, targetNamespace); err != nil {
			return err
		}
		targetSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: targetNamespace,
			},
			Type: adminPlaneSecret.Type,
			Data: adminPlaneSecret.Data,
		}
		_, err = clientSet.CoreV1().Secrets(targetNamespace).Create(ctx, targetSecret, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		klog.Infof("copy the cluster secret to %s/%s", targetNamespace, targetName)
	}
	return nil
}

// deleteAllImageSecrets deletes image pull secrets from the cluster during cleanup.
func (r *ClusterReconciler) deleteAllImageSecrets(ctx context.Context, cluster *v1.Cluster) error {
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		// During deletion, if the client is not found, the error will be ignored.
		return nil
	}
	targetName := commonutils.GenerateClusterSecret(cluster.Name, commonconfig.GetImageSecret())
	err = k8sClients.ClientSet().CoreV1().Secrets(corev1.NamespaceDefault).Delete(ctx, targetName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	adminPlaneSecret, _ := r.getAdminImageSecret(ctx)
	targetName = commonconfig.GetImageSecret()
	dataPlaneSecret, err := k8sClients.ClientSet().CoreV1().Secrets(
		common.PrimusSafeNamespace).Get(ctx, targetName, metav1.GetOptions{})
	if err == nil && (adminPlaneSecret == nil || adminPlaneSecret.UID != dataPlaneSecret.UID) {
		err = k8sClients.ClientSet().CoreV1().Secrets(common.PrimusSafeNamespace).Delete(ctx, targetName, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

// getAdminImageSecret retrieves the image pull secret from the admin plane.
func (r *ClusterReconciler) getAdminImageSecret(ctx context.Context) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: commonconfig.GetImageSecret(),
		Namespace: common.PrimusSafeNamespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// guaranteeCICDClusterRole ensures a specific ClusterRole from admin plane is synchronized to the data plane cluster.
func (r *ClusterReconciler) guaranteeCICDClusterRole(ctx context.Context, cluster *v1.Cluster) error {
	if !commonconfig.IsCICDEnable() {
		return nil
	}
	targetName := commonconfig.GetCICDRoleName()
	if targetName == "" {
		klog.Error("failed to get cicd role name")
		return nil
	}
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		return err
	}
	clientSet := k8sClients.ClientSet()

	adminClusterRole, err := r.getAdminCICDClusterRole(ctx, targetName)
	if err != nil || adminClusterRole == nil {
		return err
	}

	// Try to get the role in the target/data-plane cluster
	dataPlaneCR, err := clientSet.RbacV1().ClusterRoles().Get(ctx, targetName, metav1.GetOptions{})
	if err == nil {
		if dataPlaneCR.UID == adminClusterRole.UID {
			return nil
		}
		// Update existing role to match admin-plane definition
		dataPlaneCR.Rules = adminClusterRole.Rules
		dataPlaneCR.AggregationRule = adminClusterRole.AggregationRule
		// Keep metadata aligned (excluding cluster-specific fields like ResourceVersion/UID)
		if dataPlaneCR.Labels == nil && len(adminClusterRole.Labels) > 0 {
			dataPlaneCR.Labels = make(map[string]string)
		}
		for k := range dataPlaneCR.Labels {
			delete(dataPlaneCR.Labels, k)
		}
		for k, v := range adminClusterRole.Labels {
			dataPlaneCR.Labels[k] = v
		}
		if dataPlaneCR.Annotations == nil && len(adminClusterRole.Annotations) > 0 {
			dataPlaneCR.Annotations = make(map[string]string)
		}
		for k := range dataPlaneCR.Annotations {
			delete(dataPlaneCR.Annotations, k)
		}
		for k, v := range adminClusterRole.Annotations {
			dataPlaneCR.Annotations[k] = v
		}

		if _, err = clientSet.RbacV1().ClusterRoles().Update(ctx, dataPlaneCR, metav1.UpdateOptions{}); err != nil {
			return err
		}
		klog.Infof("update ClusterRole %s on cluster %s", targetName, cluster.Name)
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	// Create new role in data-plane cluster based on admin-plane definition
	newCR := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:        targetName,
			Labels:      adminClusterRole.Labels,
			Annotations: adminClusterRole.Annotations,
		},
		Rules:           adminClusterRole.Rules,
		AggregationRule: adminClusterRole.AggregationRule,
	}
	if _, err = clientSet.RbacV1().ClusterRoles().Create(ctx, newCR, metav1.CreateOptions{}); err != nil {
		return err
	}
	klog.Infof("copy ClusterRole %s to cluster %s", targetName, cluster.Name)
	return nil
}

// deleteCICDClusterRole deletes the CICD ClusterRole from the data-plane cluster.
func (r *ClusterReconciler) deleteCICDClusterRole(ctx context.Context, cluster *v1.Cluster) error {
	if !commonconfig.IsCICDEnable() {
		return nil
	}
	targetName := commonconfig.GetCICDRoleName()
	if targetName == "" {
		return nil
	}
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		// During deletion, if the client is not found, the error will be ignored.
		return nil
	}

	// First delete ClusterRoleBindings that reference this role by label
	labelSelector := fmt.Sprintf("%s=%s", CICDClusterRoleBindingLabel, targetName)
	crbList, err := k8sClients.ClientSet().RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	for _, crb := range crbList.Items {
		if err = k8sClients.ClientSet().RbacV1().ClusterRoleBindings().Delete(ctx, crb.Name, metav1.DeleteOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		} else {
			klog.Infof("delete ClusterRoleBinding %s (role-ref=%s) from cluster %s", crb.Name, targetName, cluster.Name)
		}
	}
	if err = k8sClients.ClientSet().RbacV1().ClusterRoles().Delete(ctx, targetName, metav1.DeleteOptions{}); err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("delete ClusterRole %s from cluster %s", targetName, cluster.Name)
	return nil
}

// getAdminCICDClusterRole Fetch ClusterRole from admin plane
func (r *ClusterReconciler) getAdminCICDClusterRole(ctx context.Context, name string) (*rbacv1.ClusterRole, error) {
	adminClusterRole := &rbacv1.ClusterRole{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, adminClusterRole); err != nil {
		// If the admin-plane role does not exist, nothing to sync.
		return nil, client.IgnoreNotFound(err)
	}
	return adminClusterRole, nil
}

// guaranteeCICDClusterRoleBinding creates a ClusterRoleBinding for the given role if not present.
func (r *ClusterReconciler) guaranteeCICDClusterRoleBinding(ctx context.Context, cluster *v1.Cluster) error {
	if !commonconfig.IsCICDEnable() {
		return nil
	}
	roleName := commonconfig.GetCICDRoleName()
	if roleName == "" {
		klog.Error("failed to get cicd role name")
		return nil
	}

	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		return err
	}
	clientSet := k8sClients.ClientSet()
	// If exists, nothing to do
	if _, err = clientSet.RbacV1().ClusterRoleBindings().Get(ctx, roleName, metav1.GetOptions{}); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
			Labels: map[string]string{
				CICDClusterRoleBindingLabel: roleName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     common.ClusterRoleKind,
			Name:     roleName,
		},
		// Subjects intentionally omitted; label ties binding to the role for management
	}
	if _, err = clientSet.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil {
		return err
	}
	klog.Infof("create ClusterRoleBinding %s for role %s on cluster %s", roleName, roleName, cluster.Name)
	return nil
}
