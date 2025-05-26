/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"strconv"

	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
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

func incFailedTimes(ctx context.Context, cli client.Client, obj client.Object) (int, error) {
	count := 1
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	} else if strCount, ok := annotations[v1.FailedCountAnnotation]; ok {
		if oldCount, err := strconv.Atoi(strCount); err == nil {
			count += oldCount
		}
	}
	patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	annotations[v1.FailedCountAnnotation] = strconv.Itoa(count)
	obj.SetAnnotations(annotations)
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
