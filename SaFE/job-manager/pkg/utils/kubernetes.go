/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

const (
	DefaultTimeout      = 10 * time.Second
	WorkloadGracePeriod = 180
)

// GenObjectReference constructs a reference object pointing to a k8s object based on the workload.
func GenObjectReference(ctx context.Context, adminClient client.Client, workload *v1.Workload) (*unstructured.Unstructured, error) {
	rt, err := commonworkload.GetResourceTemplate(ctx, adminClient, workload.ToSchemaGVK())
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{}
	obj.SetName(workload.Name)
	obj.SetNamespace(workload.Spec.Workspace)
	obj.SetGroupVersionKind(rt.ToSchemaGVK())
	return obj, nil
}

// CreateObject creates a Kubernetes object using the dynamic client.
func CreateObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory, obj *unstructured.Unstructured) error {
	gvr, err := ConvertGVKToGVR(k8sClientFactory.Mapper(), obj.GroupVersionKind())
	if err != nil {
		return commonerrors.NewInternalError(err.Error())
	}
	obj, err = k8sClientFactory.DynamicClient().Resource(gvr).Namespace(obj.GetNamespace()).Create(
		ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create k8s object, name: %s, namespace: %s, uid: %s, generation: %d",
		obj.GetName(), obj.GetNamespace(), obj.GetUID(), obj.GetGeneration())
	return nil
}

// UpdateObject updates a Kubernetes object using the dynamic client.
func UpdateObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory, obj *unstructured.Unstructured) error {
	gvr, err := ConvertGVKToGVR(k8sClientFactory.Mapper(), obj.GroupVersionKind())
	if err != nil {
		return err
	}
	obj, err = k8sClientFactory.DynamicClient().Resource(gvr).Namespace(obj.GetNamespace()).Update(
		ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("update k8s object, name: %s, namespace: %s, generation: %d",
		obj.GetName(), obj.GetNamespace(), obj.GetGeneration())
	return nil
}

// PatchObject patch a Kubernetes object using the dynamic client.
func PatchObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory,
	name, namespace string, gvk schema.GroupVersionKind, p []byte) error {
	gvr, err := ConvertGVKToGVR(k8sClientFactory.Mapper(), gvk)
	if err != nil {
		return err
	}
	if _, patchErr := k8sClientFactory.DynamicClient().
		Resource(gvr).
		Namespace(namespace).
		Patch(ctx, name, apitypes.MergePatchType, p, metav1.PatchOptions{}); patchErr != nil {
		return patchErr
	}
	return nil
}

// GetObject retrieves an object via the dynamic client.
func GetObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory, name, namespace string,
	gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
	gvr, err := ConvertGVKToGVR(k8sClientFactory.Mapper(), gvk)
	if err != nil {
		return nil, err
	}
	obj, getErr := k8sClientFactory.DynamicClient().
		Resource(gvr).
		Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if getErr != nil {
		return nil, getErr
	}
	return obj.DeepCopy(), nil
}

// GetObjectByInformer retrieves an object from the informer cache.
func GetObjectByInformer(informer informers.GenericInformer, name, namespace string) (*unstructured.Unstructured, error) {
	obj, err := informer.Lister().ByNamespace(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("the object is not of type *unstructured.Unstructured, got %T", obj))
	}
	return unstructuredObj.DeepCopy(), nil
}

// GetObjectByClientFactory retrieves an object from Kubernetes using the dynamic client factory.
// It converts the GroupVersionKind to GroupVersionResource and fetches the object by name and namespace.
func GetObjectByClientFactory(ctx context.Context, k8sClientFactory *commonclient.ClientFactory, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	gvr, err := ConvertGVKToGVR(k8sClientFactory.Mapper(), obj.GroupVersionKind())
	if err != nil {
		return nil, err
	}
	unstructuredObj, err := k8sClientFactory.DynamicClient().
		Resource(gvr).Namespace(obj.GetNamespace()).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return unstructuredObj.DeepCopy(), nil
}

// DeleteObject deletes a Kubernetes object with appropriate grace period and propagation policy.
func DeleteObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory, obj *unstructured.Unstructured) error {
	gvr, err := ConvertGVKToGVR(k8sClientFactory.Mapper(), obj.GroupVersionKind())
	if err != nil {
		return err
	}
	gracePeriod := int64(0)
	if isWorkloadOrPod(obj.GroupVersionKind()) {
		gracePeriod = WorkloadGracePeriod
	}
	policy := metav1.DeletePropagationForeground
	err = k8sClientFactory.DynamicClient().
		Resource(gvr).
		Namespace(obj.GetNamespace()).
		Delete(ctx, obj.GetName(), metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
			PropagationPolicy:  &policy,
		})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("deleting k8s object %s/%s, kind: %s", obj.GetNamespace(), obj.GetName(), obj.GetKind())
	return nil
}

// ConvertGVKToGVR converts a GroupVersionKind to GroupVersionResource using the REST mapper.
func ConvertGVKToGVR(mapper meta.RESTMapper, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	m, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		klog.ErrorS(err, "failed to RESTMapping")
		return schema.GroupVersionResource{}, err
	}
	return m.Resource, nil
}

// CopySecret copies a secret from admin plane to target namespace in the data plane.
func CopySecret(ctx context.Context, clientSet kubernetes.Interface,
	adminPlaneSecret *corev1.Secret, targetNamespace string) error {
	dataPlaneSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      adminPlaneSecret.Name,
			Namespace: targetNamespace,
		},
		Type: adminPlaneSecret.Type,
		Data: adminPlaneSecret.Data,
	}
	newContext, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	_, err := clientSet.CoreV1().Secrets(targetNamespace).Create(newContext, dataPlaneSecret, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("copy secret: %s/%s", targetNamespace, adminPlaneSecret.Name)
	return nil
}

// UpdateSecret updates a secret in the target namespace in the data plane with admin plane secret data.
func UpdateSecret(ctx context.Context, clientSet kubernetes.Interface,
	adminPlaneSecret *corev1.Secret, targetNamespace string) error {
	dataPlaneSecret, err := clientSet.CoreV1().Secrets(targetNamespace).Get(
		ctx, adminPlaneSecret.Name, metav1.GetOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	dataPlaneSecret.Type = adminPlaneSecret.Type
	dataPlaneSecret.Data = adminPlaneSecret.Data
	newContext, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	_, err = clientSet.CoreV1().Secrets(targetNamespace).Update(newContext, dataPlaneSecret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	klog.Infof("update secret: %s/%s", targetNamespace, adminPlaneSecret.Name)
	return nil
}

// DeleteSecret deletes a secret from the target namespace in the data plane.
func DeleteSecret(ctx context.Context, clientSet kubernetes.Interface, targetName, targetNamespace string) error {
	newContext, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	err := clientSet.CoreV1().Secrets(targetNamespace).Delete(newContext, targetName, metav1.DeleteOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("delete secret: %s/%s", targetNamespace, targetName)
	return nil
}

// isWorkloadOrPod checks if the given GroupVersionKind represents a workload or pod resource.
func isWorkloadOrPod(gvk schema.GroupVersionKind) bool {
	switch gvk.Kind {
	case "Pod",
		"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet",
		"Job", "CronJob":
		return true
	default:
		return false
	}
}
