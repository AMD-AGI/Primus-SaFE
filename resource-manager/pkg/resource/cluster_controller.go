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
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
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
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

type ClusterReconciler struct {
	*ClusterBaseReconciler
	clientManager *commonutils.ObjectManager
}

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
			predicate.ResourceVersionChangedPredicate{}, r.caredChangePredicate()))).
		Watches(&corev1.Pod{}, r.handlePodEvent()).
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Cluster Controller successfully")
	return nil
}

func (r *ClusterReconciler) caredChangePredicate() predicate.Predicate {
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
			cluster, ok := e.Object.(*v1.Cluster)
			if !ok {
				return false
			}
			if err := r.clientManager.Delete(cluster.Name); err != nil {
				klog.ErrorS(err, "failed to delete cluster clients", "cluster", cluster.Name)
			} else {
				klog.Infof("delete cluster clients successfully. cluster: %s", cluster.Name)
			}
			return false
		},
	}
}

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
	if !cluster.DeletionTimestamp.IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, cluster)
	}
	if err = r.guaranteeClusterControlPlane(ctx, cluster); err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = r.guaranteeClientFactory(ctx, cluster); err != nil {
		return ctrlruntime.Result{}, err
	}
	if result, err := r.guaranteeStorage(ctx, cluster); err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	if result, err := r.guaranteeDefaultAddon(ctx, cluster); err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	if result, err := r.guaranteePriorityClass(ctx, cluster); err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	if err = r.guaranteeAllImageSecrets(ctx, cluster); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

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
	if err := utils.RemoveFinalizer(ctx, r.Client, cluster, v1.ClusterFinalizer); err != nil {
		klog.ErrorS(err, "failed to remove finalizer")
		return err
	}
	return nil
}

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
	return nil
}

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
		desc := "This priority class should be used for primus-safe job only."
		if err = createPriorityClass(ctx, clientSet, pc.name, desc, pc.value); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	return ctrlruntime.Result{}, nil
}

func (r *ClusterReconciler) deletePriorityClass(ctx context.Context, cluster *v1.Cluster) error {
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		if !cluster.IsReady() {
			return nil
		}
		return err
	}
	clientSet := k8sClients.ClientSet()
	allPriorityClass := genAllPriorityClass(cluster.Name)
	for _, pc := range allPriorityClass {
		if err = deletePriorityClass(ctx, clientSet, pc.name); err != nil {
			return err
		}
	}
	return nil
}

func createPriorityClass(ctx context.Context, clientSet kubernetes.Interface, name, description string, value int32) error {
	priorityClass := &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Value:       value,
		Description: description,
	}
	if _, err := clientSet.SchedulingV1().PriorityClasses().Create(
		ctx, priorityClass, metav1.CreateOptions{}); err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create PriorityClass, name: %s, value: %d", name, value)
	return nil
}

func deletePriorityClass(ctx context.Context, clientSet kubernetes.Interface, name string) error {
	if err := clientSet.SchedulingV1().PriorityClasses().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("delete PriorityClass, name: %s", name)
	return nil
}

type PriorityClass struct {
	name  string
	value int32
}

func genAllPriorityClass(clusterId string) []PriorityClass {
	return []PriorityClass{
		{name: commonutils.GenerateClusterPriorityClass(clusterId, common.HighPriority), value: 10000},
		{name: commonutils.GenerateClusterPriorityClass(clusterId, common.MedPriority), value: -1},
		{name: commonutils.GenerateClusterPriorityClass(clusterId, common.LowPriority), value: -10000},
	}
}

func (r *ClusterReconciler) guaranteeAllImageSecrets(ctx context.Context, cluster *v1.Cluster) error {
	if commonconfig.GetImageSecret() == "" {
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
		if err = r.createNamespace(ctx, targetNamespace, clientSet); err != nil {
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

func (r *ClusterReconciler) deleteAllImageSecrets(ctx context.Context, cluster *v1.Cluster) error {
	k8sClients, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil {
		if !cluster.IsReady() {
			return nil
		}
		return err
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

func (r *ClusterReconciler) createNamespace(ctx context.Context, name string, clientSet kubernetes.Interface) error {
	if name == "" {
		return fmt.Errorf("the name is empty")
	}
	_, err := clientSet.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err = clientSet.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create namespace %s of dataplane", name)
	return nil
}

func (r *ClusterReconciler) getAdminImageSecret(ctx context.Context) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: commonconfig.GetImageSecret(),
		Namespace: common.PrimusSafeNamespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}
