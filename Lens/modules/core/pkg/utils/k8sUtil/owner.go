// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package k8sUtil

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetOwnerObject(
	ctx context.Context,
	k8sClient client.Client,
	ownerRef metav1.OwnerReference,
	namespace string,
) (*unstructured.Unstructured, error) {
	return GetObjectByGvk(ctx, ownerRef.APIVersion, ownerRef.Kind, namespace, ownerRef.Name, k8sClient)
}
