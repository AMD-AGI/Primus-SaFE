/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	unstructuredutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/unstructured"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

// createDataplaneNamespace creates a Kubernetes namespace in dataplane if it doesn't already exist.
func createDataplaneNamespace(ctx context.Context, name string, clientSet kubernetes.Interface) error {
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
	klog.Infof("create namespace: %s", name)
	return nil
}

// deleteDataplaneNamespace deletes a Kubernetes namespace in dataplane
func deleteDataplaneNamespace(ctx context.Context, name string, clientSet kubernetes.Interface) error {
	if name == "" {
		return fmt.Errorf("the name is empty")
	}
	err := clientSet.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("delete namespace: %s", name)
	return nil
}

// createDataPlanePv creates a Kubernetes pv in dataplane if it doesn't already exist.
func createDataPlanePv(ctx context.Context, workspace *v1.Workspace, adminClient client.Client, dataplaneClient kubernetes.Interface) error {
	template, err := getPvTemplate(ctx, adminClient, workspace)
	if err != nil || template == nil {
		return err
	}
	pv := &corev1.PersistentVolume{}
	err = unstructuredutils.ConvertUnstructuredToObject(template, pv)
	if err != nil {
		return err
	}
	if err = createPV(ctx, pv, dataplaneClient); err != nil {
		return err
	}
	return nil
}

// createDataPlanePvc creates a Kubernetes pvc in dataplane if it doesn't already exist.
func createDataPlanePvc(ctx context.Context, workspace *v1.Workspace, clientSet kubernetes.Interface) error {
	// create pvc for data plane
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		if isPVCExist(ctx, workspace.Name, vol.GenFullVolumeId(), clientSet) {
			continue
		}
		pvc, err := generatePVC(&vol, workspace)
		if err != nil {
			klog.Error(err.Error())
			continue
		}
		if err = createPVC(ctx, pvc, clientSet); err != nil {
			return err
		}
	}
	return nil
}

// isPVCExist checks if a PersistentVolumeClaim exists in the specified namespace.
// Returns true if the PVC is found, false otherwise.
func isPVCExist(ctx context.Context, namespace, name string, clientSet kubernetes.Interface) bool {
	_, err := clientSet.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return true
	}
	return false
}

// generatePVC generates a PersistentVolumeClaim based on workspace volume specifications.
func generatePVC(volume *v1.WorkspaceVolume,
	workspace *v1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.SetName(volume.GenFullVolumeId())
	pvc.SetNamespace(workspace.Name)
	if len(volume.Selector) > 0 {
		pvc.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: volume.Selector,
		}
		pvc.Spec.StorageClassName = pointer.String("")
	} else {
		pvc.Spec.StorageClassName = pointer.String(volume.StorageClass)
	}

	storeQuantity, err := resource.ParseQuantity(volume.Capacity)
	if err != nil {
		return nil, err
	}
	pvc.Spec.Resources = corev1.VolumeResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: storeQuantity,
		},
	}
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{volume.AccessMode}
	volumeMode := corev1.PersistentVolumeFilesystem
	pvc.Spec.VolumeMode = &volumeMode
	return pvc, nil
}

// createPV creates a PersistentVolume.
func createPV(ctx context.Context, pvTemplate *corev1.PersistentVolume, clientSet kubernetes.Interface) error {
	pv, err := clientSet.CoreV1().PersistentVolumes().Create(ctx, pvTemplate, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create persistent volume %s", pv.Name)
	return nil
}

// deletePV deletes a PersistentVolume and removes its finalizers if present.
func deletePV(ctx context.Context, workspace *v1.Workspace, clientSet kubernetes.Interface) error {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			v1.OwnerLabel: workspace.Name,
		},
	}
	pvList, err := clientSet.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	if len(pvList.Items) == 0 {
		return nil
	}

	pv := &pvList.Items[0]
	if len(pv.Finalizers) > 0 {
		pv.Finalizers = nil
		_, err = clientSet.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
		if err != nil {
			klog.ErrorS(err, "failed to remove finalizers of pv", "name", pv.Name)
		}
	}
	err = clientSet.CoreV1().PersistentVolumes().Delete(ctx, pv.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	klog.Infof("delete persistent volume: %s", pv.Name)
	return nil
}

// createPVC creates a PersistentVolumeClaim.
func createPVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim, clientSet kubernetes.Interface) error {
	pvc, err := clientSet.CoreV1().PersistentVolumeClaims(pvc.GetNamespace()).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create persistent volume claims: %s/%s", pvc.GetNamespace(), pvc.Name)
	return nil
}

// deletePVC deletes a PersistentVolumeClaim and removes its finalizers if present.
func deletePVC(ctx context.Context, name, namespace string, clientSet kubernetes.Interface) error {
	pvc, err := clientSet.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	if len(pvc.Finalizers) > 0 {
		pvc.Finalizers = nil
		_, err = clientSet.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
		if err != nil {
			klog.ErrorS(err, "failed to remove finalizers of pvc",
				"name", name, "namespace", namespace)
		}
	}
	err = clientSet.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	klog.Infof("delete persistent volume claims: %s/%s", namespace, name)
	return nil
}

// syncDataPlanePVC reconciles actual data-plane pvc to match the desired workspace spec.
func syncDataPlanePVC(ctx context.Context, workspace *v1.Workspace, clientSet kubernetes.Interface) error {
	// Desired PVC set based on current spec
	desiredPVCs := sets.NewSet()
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		desiredPVCs.Insert(vol.GenFullVolumeId())
	}
	// Prune unexpected PVCs (those managed by workspace but not desired)
	pvcList, err := clientSet.CoreV1().PersistentVolumeClaims(workspace.Name).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		// We manage PFS PVCs named like "pfs-<id>"
		if strings.HasPrefix(pvc.Name, string(v1.PFS)+"-") && !desiredPVCs.Has(pvc.Name) {
			if err = deletePVC(ctx, pvc.Name, workspace.Name, clientSet); err != nil {
				return err
			}
		}
	}
	// Ensure desired PVCs exist (idempotent)
	if err = createDataPlanePvc(ctx, workspace, clientSet); err != nil {
		return err
	}
	return nil
}

// createCICDNoPermissionSA creates a CICD ServiceAccount for the workspace in the data plane cluster.
// It checks if the ServiceAccount already exists, and creates it if not found.
func createCICDNoPermissionSA(ctx context.Context, workspace *v1.Workspace, clientSet kubernetes.Interface) error {
	if !commonconfig.IsCICDEnable() {
		return nil
	}
	saName := commonutils.GenerateCICDNoPermissionName()
	saNamespace := workspace.Name
	// Ensure ServiceAccount exists
	_, err := clientSet.CoreV1().ServiceAccounts(saNamespace).Get(ctx, saName, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: saNamespace,
		},
	}
	if _, err = clientSet.CoreV1().ServiceAccounts(saNamespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
		return err
	}
	klog.Infof("create ServiceAccount %s/%s on cluster %s", saNamespace, saName, workspace.Spec.Cluster)
	return nil
}

// deleteCICDNoPermissionSA removes the CICD ServiceAccount for the workspace from the data plane cluster.
// If the ServiceAccount doesn't exist, it ignores the NotFound error.
func deleteCICDNoPermissionSA(ctx context.Context, workspace *v1.Workspace, clientSet kubernetes.Interface) error {
	if !commonconfig.IsCICDEnable() {
		return nil
	}
	saName := commonutils.GenerateCICDNoPermissionName()
	saNamespace := workspace.Name
	if err := clientSet.CoreV1().ServiceAccounts(saNamespace).Delete(ctx, saName, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		klog.Infof("delete ServiceAccount %s/%s on cluster %s", saNamespace, saName, workspace.Spec.Cluster)
	}
	return nil
}

// deleteWorkspaceSecrets deletes all secrets in the given workspace namespace
func deleteWorkspaceSecrets(ctx context.Context, workspace *v1.Workspace, clientSet kubernetes.Interface) error {
	namespace := workspace.Name
	secretList, err := clientSet.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range secretList.Items {
		sec := &secretList.Items[i]
		if err = clientSet.CoreV1().Secrets(namespace).Delete(ctx, sec.Name, metav1.DeleteOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		} else {
			klog.Infof("delete secret: %s/%s in data plane", namespace, sec.Name)
		}
	}
	return nil
}

func getPvTemplate(ctx context.Context, cli client.Client, workspace *v1.Workspace) (*unstructured.Unstructured, error) {
	cm := &corev1.ConfigMap{}
	if err := cli.Get(ctx, client.ObjectKey{Name: common.PrimusPvmName, Namespace: common.PrimusSafeNamespace}, cm); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	if v1.GetAnnotation(cm, "primus-safe.workspace.auto-create-pv") != v1.TrueStr {
		return nil, nil
	}
	if v1.GetDisplayName(cm) == "" {
		return nil, fmt.Errorf("failed to find the display name. name: %s", cm.Name)
	}
	templateStr, ok := cm.Data["template"]
	if !ok || templateStr == "" {
		return nil, fmt.Errorf("failed to find the template. name: %s", cm.Name)
	}
	template, err := jsonutils.ParseYamlToJson(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %v", err.Error())
	}
	if len(template.GetLabels()) == 0 {
		template.SetLabels(make(map[string]string))
	}
	pvName := v1.GetDisplayName(cm) + "-" + workspace.Name
	v1.SetLabel(template, common.PfsSelectorKey, pvName)
	v1.SetLabel(template, v1.WorkspaceIdLabel, workspace.Name)
	v1.SetLabel(template, v1.OwnerLabel, workspace.Name)
	template.SetName(pvName)
	return template, nil
}
