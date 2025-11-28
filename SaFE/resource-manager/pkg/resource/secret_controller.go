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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

const (
	// managedSecretLabelKey marks secrets created/maintained by resource-manager in the data plane
	managedSecretLabelKey = v1.PrimusSafeDomain + "secret-mirror"
	managedSecretLabelVal = "true"
)

type SecretReconciler struct {
	*ClusterBaseReconciler
	clientManager *commonutils.ObjectManager
}

func SetupSecretController(mgr manager.Manager) error {
	r := &SecretReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		clientManager: commonutils.NewObjectManagerSingleton(),
	}
	if r.clientManager == nil {
		return fmt.Errorf("failed to new clientManager")
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(relevantChangePredicate{})).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Secret Controller successfully")
	return nil
}

type relevantChangePredicate struct {
	predicate.Funcs
}

func (relevantChangePredicate) Create(e event.CreateEvent) bool {
	secret, ok := e.Object.(*corev1.Secret)
	if !ok {
		return false
	}
	if secret.Namespace != common.PrimusSafeNamespace ||
		v1.GetAnnotation(secret, v1.WorkspaceIdsAnnotation) == "" {
		return false
	}
	return true
}

func (relevantChangePredicate) Update(e event.UpdateEvent) bool {
	oldSecret, ok1 := e.ObjectOld.(*corev1.Secret)
	newSecret, ok2 := e.ObjectNew.(*corev1.Secret)
	if !ok1 || !ok2 || newSecret.Namespace != common.PrimusSafeNamespace {
		return false
	}
	if v1.GetAnnotation(newSecret, v1.WorkspaceIdsAnnotation) != "" && !newSecret.GetDeletionTimestamp().IsZero() {
		return true
	}
	if v1.GetAnnotation(oldSecret, v1.WorkspaceIdsAnnotation) != v1.GetAnnotation(newSecret, v1.WorkspaceIdsAnnotation) {
		return true
	}
	if !compareSecretData(oldSecret.Data, newSecret.Data) {
		return true
	}
	if oldSecret.Type != newSecret.Type {
		return true
	}
	return false
}

func compareSecretData(d1, d2 map[string][]byte) bool {
	if len(d1) != len(d2) {
		return false
	}
	for key, value1 := range d1 {
		value2, ok := d2[key]
		if !ok {
			return false
		}
		if string(value1) != string(value2) {
			return false
		}
	}
	return true
}
func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	secret := new(corev1.Secret)
	if err := r.Get(ctx, req.NamespacedName, secret); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !secret.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, secret)
	}
	return r.processSecrets(ctx, secret)
}

// delete handles secret deletion by removing the finalizer.
func (r *SecretReconciler) delete(ctx context.Context, secret *corev1.Secret) error {
	if err := r.removeSecretFromCluster(ctx, secret); err != nil {
		return err
	}
	if err := r.removeSecretFromWorkspaces(ctx, secret); err != nil {
		return err
	}
	// If deleting, try to cleanup mirrored copies
	if err := r.cleanupMirroredSecrets(ctx, secret.Name, nil); err != nil {
		klog.ErrorS(err, "failed to cleanup mirrored secrets on delete", "name", secret.Name)
		return err
	}
	return utils.RemoveFinalizer(ctx, r.Client, secret, v1.SecretFinalizer)
}

func (r *SecretReconciler) processSecrets(ctx context.Context, secret *corev1.Secret) (ctrlruntime.Result, error) {
	workspaceIds := commonsecret.GetSecretWorkspaces(secret)
	// If this secret is not bound to any workspace, remove mirrored copies (by label) in data planes
	if len(workspaceIds) == 0 {
		return ctrlruntime.Result{}, r.cleanupMirroredSecrets(ctx, secret.Name, nil)
	}

	for _, id := range workspaceIds {
		// Get the workspace to determine target cluster/namespace
		ws := &v1.Workspace{}
		if err := r.Get(ctx, client.ObjectKey{Name: id}, ws); err != nil {
			// workspace may not yet exist; no-op if not found
			if apierrors.IsNotFound(err) {
				continue
			}
			return ctrlruntime.Result{}, err
		}
		// Ensure the mirrored secret exists/updated in target namespace on data plane
		clientSet, err := r.getClientSetOfDataplane(ctx, ws.Spec.Cluster)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return ctrlruntime.Result{RequeueAfter: time.Second}, nil
		}
		if clientSet == nil {
			continue
		}
		if err = r.syncSecretToWorkspace(ctx, clientSet, secret, ws); err != nil {
			return ctrlruntime.Result{}, err
		}
		if err = r.updateWorkspaceRefSecret(ctx, secret, ws); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	if err := r.updateClusterRefSecret(ctx, secret); err != nil {
		return ctrlruntime.Result{}, err
	}
	// Cleanup any mirrored copies in other workspaces
	if err := r.cleanupMirroredSecrets(ctx, secret.Name, workspaceIds); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// getClientSetOfDataplane returns a client-go clientSet of the data-plane cluster for a given clusterId
func (r *SecretReconciler) getClientSetOfDataplane(ctx context.Context, clusterId string) (kubernetes.Interface, error) {
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

// syncSecretToWorkspace ensures a mirrored secret exists/updated in the target namespace on data plane.
func (r *SecretReconciler) syncSecretToWorkspace(ctx context.Context, clientSet kubernetes.Interface,
	adminPlaneSecret *corev1.Secret, workspace *v1.Workspace) error {
	targetSecrets := clientSet.CoreV1().Secrets(workspace.Name)
	existing, err := targetSecrets.Get(ctx, adminPlaneSecret.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if apierrors.IsNotFound(err) {
		return copySecret(ctx, clientSet, adminPlaneSecret, workspace.Name)
	}

	// Update path
	needUpdate := false
	// ensure marker label
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	if existing.Labels[managedSecretLabelKey] != managedSecretLabelVal {
		existing.Labels[managedSecretLabelKey] = managedSecretLabelVal
		needUpdate = true
	}
	// sync type
	if existing.Type != adminPlaneSecret.Type {
		existing.Type = adminPlaneSecret.Type
		needUpdate = true
	}
	// sync data
	if !compareSecretData(existing.Data, adminPlaneSecret.Data) {
		existing.Data = adminPlaneSecret.Data
		needUpdate = true
	}
	if !needUpdate {
		return nil
	}
	if _, err = targetSecrets.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return err
	}
	klog.Infof("update secret: %s/%s for data plane", workspace.Name, adminPlaneSecret.Name)
	return nil
}

// cleanupMirroredSecrets deletes mirrored secrets (identified by label) across all workspaces,
// except the optional keepNamespace (when provided).
func (r *SecretReconciler) cleanupMirroredSecrets(ctx context.Context, secretName string, keepNamespaces []string) error {
	wsList := &v1.WorkspaceList{}
	if err := r.List(ctx, wsList); err != nil {
		return err
	}
	keepNamespaceSet := sets.NewSetByKeys(keepNamespaces...)
	for _, item := range wsList.Items {
		if keepNamespaceSet.Has(item.Name) {
			continue
		}
		clientSet, err := r.getClientSetOfDataplane(ctx, item.Spec.Cluster)
		if err != nil {
			return err
		}
		if clientSet == nil {
			continue
		}
		// remove the secret reference from the corresponding workspace if it exists
		if err = r.removeSecretFromWorkspace(ctx, secretName, &item); err != nil {
			return err
		}

		sec, err := clientSet.CoreV1().Secrets(item.Name).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if sec.Labels == nil || sec.Labels[managedSecretLabelKey] != managedSecretLabelVal {
			continue
		}
		if err = clientSet.CoreV1().Secrets(item.Name).Delete(ctx, secretName, metav1.DeleteOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		} else {
			klog.Infof("delete secret: %s/%s for data plane", item.Name, secretName)
		}
	}
	return nil
}

// removeSecretFromCluster removes secret references from cluster.
func (r *SecretReconciler) removeSecretFromCluster(ctx context.Context, secret *corev1.Secret) error {
	clusterList := &v1.ClusterList{}
	if err := r.List(ctx, clusterList, &client.ListOptions{}); err != nil {
		return err
	}
	for _, cluster := range clusterList.Items {
		imageSecret := cluster.Spec.ControlPlane.ImageSecret
		if imageSecret == nil || imageSecret.Name != secret.Name {
			continue
		}
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Spec.ControlPlane.ImageSecret = nil
		if err := r.Patch(ctx, &cluster, patch); err != nil {
			return err
		}
	}
	return nil
}

// removeSecretFromWorkspaces removes secret references from all workspaces.
func (r *SecretReconciler) removeSecretFromWorkspaces(ctx context.Context, secret *corev1.Secret) error {
	workspaceIds := commonsecret.GetSecretWorkspaces(secret)
	for _, id := range workspaceIds {
		workspace := &v1.Workspace{}
		err := r.Get(ctx, client.ObjectKey{Name: id}, workspace)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if err = r.removeSecretFromWorkspace(ctx, secret.Name, workspace); err != nil {
			return err
		}
	}
	return nil
}

// removeSecretFromWorkspaces removes secret references from the specified workspace.
func (r *SecretReconciler) removeSecretFromWorkspace(ctx context.Context, secretName string, workspace *v1.Workspace) error {
	newSecrets := make([]corev1.ObjectReference, 0, len(workspace.Spec.ImageSecrets))
	for i, currentSecret := range workspace.Spec.ImageSecrets {
		if currentSecret.Name == secretName {
			continue
		}
		newSecrets = append(newSecrets, workspace.Spec.ImageSecrets[i])
	}
	if len(newSecrets) != len(workspace.Spec.ImageSecrets) {
		patch := client.MergeFrom(workspace.DeepCopy())
		workspace.Spec.ImageSecrets = newSecrets
		if err := r.Patch(ctx, workspace, patch); err != nil {
			return err
		}
		klog.Infof("remove secret reference from workspace: %s/%s", workspace.Name, secretName)
	}
	return nil
}

// updateWorkspaceRefSecret updates the workspace's reference to a secret when the secret is updated.
// It checks if the workspace is using this secret and updates the reference with the latest ResourceVersion if changed.
func (r *SecretReconciler) updateWorkspaceRefSecret(ctx context.Context, secret *corev1.Secret, workspace *v1.Workspace) error {
	secretRef := commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	for i, currentSecret := range workspace.Spec.ImageSecrets {
		if currentSecret.Name != secret.Name {
			continue
		}
		if currentSecret.ResourceVersion != secretRef.ResourceVersion {
			patch := client.MergeFrom(workspace.DeepCopy())
			workspace.Spec.ImageSecrets[i] = *secretRef
			if err := r.Patch(ctx, workspace, patch); err != nil {
				return err
			}
		}
		break
	}
	return nil
}

// updateClusterRefSecret updates the cluster's reference to a secret when the secret is updated.
func (r *SecretReconciler) updateClusterRefSecret(ctx context.Context, secret *corev1.Secret) error {
	clusterList := &v1.ClusterList{}
	if err := r.List(ctx, clusterList, &client.ListOptions{}); err != nil {
		return err
	}
	for _, cluster := range clusterList.Items {
		imageSecret := cluster.Spec.ControlPlane.ImageSecret
		if imageSecret == nil || imageSecret.Name != secret.Name {
			continue
		}
		if imageSecret.ResourceVersion != secret.ResourceVersion {
			patch := client.MergeFrom(cluster.DeepCopy())
			cluster.Spec.ControlPlane.ImageSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
			if err := r.Patch(ctx, &cluster, patch); err != nil {
				return err
			}
		}
	}
	return nil
}

// copySecret copies a secret from admin plane to target namespace in the data plane.
func copySecret(ctx context.Context, clientSet kubernetes.Interface,
	adminPlaneSecret *corev1.Secret, targetNamespace string) error {
	dataPlaneSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      adminPlaneSecret.Name,
			Namespace: targetNamespace,
			Labels: map[string]string{
				managedSecretLabelKey: managedSecretLabelVal,
			},
		},
		Type: adminPlaneSecret.Type,
		Data: adminPlaneSecret.Data,
	}
	_, err := clientSet.CoreV1().Secrets(targetNamespace).Create(ctx, dataPlaneSecret, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create secret: %s/%s for data plane", targetNamespace, adminPlaneSecret.Name)
	return nil
}
