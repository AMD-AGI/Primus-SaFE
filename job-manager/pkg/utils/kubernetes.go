/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// Retrieve the corresponding resource_template based on the workload's GVK.
// Multiple workloads may map to the same type of resource-template
func GetResourceTemplate(ctx context.Context, adminClient client.Client, gvk schema.GroupVersionKind) (*v1.ResourceTemplate, error) {
	rtl := &v1.ResourceTemplateList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		v1.WorkloadKindLabel: gvk.Kind, v1.WorkloadVersionLabel: gvk.Version})
	if err := adminClient.List(ctx, rtl, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	if len(rtl.Items) == 0 {
		return nil, commonerrors.NewBadRequest(
			fmt.Sprintf("the resource template is not found, kind: %s, version: %s", gvk.Kind, gvk.Version))
	}
	return &rtl.Items[0], nil
}

// Construct a reference object pointing to a k8s object based on the workload.
func GenObjectReference(ctx context.Context, adminClient client.Client, workload *v1.Workload) (*unstructured.Unstructured, error) {
	rt, err := GetResourceTemplate(ctx, adminClient, workload.ToSchemaGVK())
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{}
	obj.SetName(workload.Name)
	obj.SetNamespace(workload.Spec.Workspace)
	obj.SetGroupVersionKind(rt.ToSchemaGVK())
	return obj, nil
}

func CreateObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory, obj *unstructured.Unstructured) error {
	gvr, err := CvtToGVR(k8sClientFactory.Mapper(), obj.GroupVersionKind())
	if err != nil {
		return err
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

func UpdateObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory, obj *unstructured.Unstructured) error {
	gvr, err := CvtToGVR(k8sClientFactory.Mapper(), obj.GroupVersionKind())
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

func GetObject(informer informers.GenericInformer, name, namespace string) (*unstructured.Unstructured, error) {
	obj, err := informer.Lister().ByNamespace(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	objUnstructured, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, commonerrors.NewInternalError("the object is invalid")
	}
	return objUnstructured.DeepCopy(), nil
}

func DeleteObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory, obj *unstructured.Unstructured) error {
	gvr, err := CvtToGVR(k8sClientFactory.Mapper(), obj.GroupVersionKind())
	if err != nil {
		return err
	}
	gracePeriod := int64(300)
	policy := metav1.DeletePropagationBackground
	err = k8sClientFactory.DynamicClient().Resource(gvr).Namespace(obj.GetNamespace()).Delete(ctx, obj.GetName(),
		metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &policy})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("delete k8s object, name: %s, namespace: %s", obj.GetName(), obj.GetNamespace())
	return nil
}

func CvtToGVR(mapper meta.RESTMapper, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	m, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		klog.ErrorS(err, "failed to RESTMapping")
		return schema.GroupVersionResource{}, err
	}
	return m.Resource, nil
}

func CreateNamespace(ctx context.Context, name string, clientSet kubernetes.Interface) error {
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
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	_, err = clientSet.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create namespace: %s", name)
	return nil
}

func DeleteNamespace(ctx context.Context, name string, clientSet kubernetes.Interface) error {
	if name == "" {
		return fmt.Errorf("the name is empty")
	}
	newContext, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	err := clientSet.CoreV1().Namespaces().Delete(newContext, name, metav1.DeleteOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("delete namespace: %s", name)
	return nil
}

func CopySecret(ctx context.Context,
	sourceSecret *corev1.Secret, namespace string, clientSet kubernetes.Interface) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceSecret.Name,
			Namespace: namespace,
		},
		Type: sourceSecret.Type,
		Data: sourceSecret.Data,
	}
	newContext, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	_, err := clientSet.CoreV1().Secrets(namespace).Create(newContext, secret, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("copy secret: %s/%s", namespace, sourceSecret.Name)
	return nil
}

func CreatePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim, clientSet kubernetes.Interface) error {
	newContext, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	var err error
	pvc, err = clientSet.CoreV1().PersistentVolumeClaims(pvc.GetNamespace()).Create(newContext, pvc, metav1.CreateOptions{})
	if err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create persistent volume claims: %s/%s", pvc.GetNamespace(), pvc.Name)
	return nil
}

func DeletePVC(ctx context.Context, name, namespace string, clientSet kubernetes.Interface) error {
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
		return client.IgnoreNotFound(err)
	}
	klog.Infof("delete persistent volume claims: %s/%s", namespace, name)
	return nil
}
