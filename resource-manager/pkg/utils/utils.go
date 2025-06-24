/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func RemoveOwnerReferences(references []metav1.OwnerReference, uid types.UID) []metav1.OwnerReference {
	newReferences := make([]metav1.OwnerReference, 0, len(references))
	for k, r := range references {
		if r.UID != uid {
			newReferences = append(newReferences, references[k])
		}
	}
	return newReferences
}

func RemoveFinalizer(ctx context.Context, cli client.Client, obj client.Object, finalizer ...string) error {
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

func IncRetryCount(ctx context.Context, cli client.Client, obj client.Object, maxCount int) (int, error) {
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

// Ignore errors that cannot be fixed
func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if commonerrors.IsBadRequest(err) || commonerrors.IsInternal(err) || commonerrors.IsNotFound(err) {
		return true
	}
	if apierrors.IsForbidden(err) || apierrors.IsNotFound(err) {
		return true
	}
	return false
}

func GetK8sClientFactory(clientManager *commonutils.ObjectManager, clusterId string) (*commonclient.ClientFactory, error) {
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
