/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"strconv"

	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func removeOwnerReferences(references []metav1.OwnerReference, uid types.UID) []metav1.OwnerReference {
	newReferences := make([]metav1.OwnerReference, 0, len(references))
	for k, r := range references {
		if r.UID != uid {
			newReferences = append(newReferences, references[k])
		}
	}
	return newReferences
}

func removeFinalizer(ctx context.Context, cli client.Client, obj client.Object, finalizer ...string) error {
	var found bool
	for _, val := range finalizer {
		if found = controllerutil.ContainsFinalizer(obj, val); found {
			break
		}
	}
	if !found {
		return nil
	}

	for _, val := range finalizer {
		controllerutil.RemoveFinalizer(obj, val)
	}
	if err := cli.Update(ctx, obj); err != nil {
		klog.ErrorS(err, "failed to remove finalizer")
		return err
	}
	return nil
}

func incRetryCount(ctx context.Context, cli client.Client, obj client.Object, maxCount int) (int, error) {
	count := v1.GetRetryCount(obj) + 1
	if count > maxCount {
		return count, nil
	}
	patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	v1.SetAnnotation(obj, v1.RetryCountAnnotation, strconv.Itoa(count))
	if err := cli.Patch(ctx, obj, patch); err != nil {
		return 0, client.IgnoreNotFound(err)
	}
	return count, nil
}

// Ignore errors that cannot be fixed or are tolerable.
func ignoreError(err error) error {
	if err == nil {
		return nil
	}
	if commonerrors.IsPrimus(err) {
		return nil
	}
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func getK8sClientFactory(clientManager *commonutils.ObjectManager, clusterId string) (*commonclient.ClientFactory, error) {
	if clientManager == nil {
		return nil, commonerrors.NewInternalError("client manager is emtpy")
	}
	obj, _ := clientManager.Get(clusterId)
	if obj == nil {
		err := fmt.Errorf("the client of cluster %s is not found. pls retry later", clusterId)
		return nil, commonerrors.NewInternalError(err.Error())
	}
	k8sClients, ok := obj.(*commonclient.ClientFactory)
	if !ok {
		return nil, commonerrors.NewInternalError("failed to correctly build the k8s client")
	}
	return k8sClients, nil
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
