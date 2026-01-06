/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

const (
	WorkloadGracePeriod = 180
)

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
	obj *unstructured.Unstructured, p []byte) error {
	gvr, err := ConvertGVKToGVR(k8sClientFactory.Mapper(), obj.GroupVersionKind())
	if err != nil {
		return err
	}
	if _, patchErr := k8sClientFactory.DynamicClient().
		Resource(gvr).
		Namespace(obj.GetNamespace()).
		Patch(ctx, obj.GetName(), apitypes.MergePatchType, p, metav1.PatchOptions{}); patchErr != nil {
		return patchErr
	}
	return nil
}

// GetObject retrieves an object via the dynamic client.
func GetObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory,
	name, namespace string, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
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

// ListObject list objects via the dynamic client.
func ListObject(ctx context.Context, k8sClientFactory *commonclient.ClientFactory,
	labelSelector, namespace string, gvk schema.GroupVersionKind) ([]unstructured.Unstructured, error) {
	gvr, err := ConvertGVKToGVR(k8sClientFactory.Mapper(), gvk)
	if err != nil {
		return nil, err
	}
	list, getErr := k8sClientFactory.DynamicClient().
		Resource(gvr).
		Namespace(namespace).
		List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if getErr != nil {
		return nil, getErr
	}
	return list.Items, nil
}

// DeleteObjectsByWorkload deletes all Kubernetes objects associated with a specific workload
// It retrieves all related objects in the data plane, and deletes each object one by one
// Returns true if objects were found and deleted, false if no objects were found.
func DeleteObjectsByWorkload(ctx context.Context, adminClient client.Client,
	k8sClientFactory *commonclient.ClientFactory, adminWorkload *v1.Workload) (bool, error) {

	var gvks []schema.GroupVersionKind
	if commonworkload.IsTorchFT(adminWorkload) {
		gvks = append(gvks, schema.GroupVersionKind{
			Group: "kubeflow.org", Version: common.DefaultVersion, Kind: common.PytorchJobKind,
		})
		gvks = append(gvks, schema.GroupVersionKind{
			Group: "apps", Version: common.DefaultVersion, Kind: common.DeploymentKind,
		})
	} else {
		rt, err := commonworkload.GetResourceTemplate(ctx, adminClient, adminWorkload)
		if err != nil {
			return false, err
		}
		gvks = append(gvks, rt.ToSchemaGVK())
	}

	labelSelector := v1.WorkloadIdLabel + "=" + adminWorkload.Name
	hasFound := false
	for _, gvk := range gvks {
		unstructuredObjs, err := ListObject(ctx, k8sClientFactory, labelSelector, adminWorkload.Spec.Workspace, gvk)
		if err != nil {
			return false, err
		}
		if len(unstructuredObjs) == 0 {
			continue
		}
		hasFound = true
		for _, obj := range unstructuredObjs {
			// delete the related resource in data plane
			if err = DeleteObject(ctx, k8sClientFactory, &obj); err != nil && !apierrors.IsNotFound(err) {
				klog.ErrorS(err, "failed to delete k8s object")
				return false, err
			}
		}
	}
	return hasFound, nil
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

// FindFailedCondition checks if a workload has a failed condition
// It looks for a K8sFailed type condition that matches the workload's dispatch count
// Returns true if a matching failed condition is found, otherwise returns false
func FindFailedCondition(workload *v1.Workload) bool {
	cond := &metav1.Condition{
		Type:   string(v1.K8sFailed),
		Reason: commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload)),
	}
	if FindCondition(workload, cond) != nil {
		return true
	}
	return false
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

// isWorkloadOrPod checks if the given GroupVersionKind represents a workload or pod resource.
func isWorkloadOrPod(gvk schema.GroupVersionKind) bool {
	switch gvk.Kind {
	case "Pod",
		"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet",
		"Job", "CronJob", "EphemeralRunner":
		return true
	default:
		return false
	}
}
