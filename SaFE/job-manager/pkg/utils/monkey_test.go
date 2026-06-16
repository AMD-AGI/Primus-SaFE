/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

func TestListObjectsByWorkload(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetWorkloadGVK, func(*v1.Workload) []schema.GroupVersionKind {
		return []schema.GroupVersionKind{{Group: "batch", Version: "v1", Kind: "Job"}}
	})
	patches.ApplyFunc(commonworkload.GetResourceTemplateByGVK,
		func(context.Context, ctrlClient.Client, schema.GroupVersionKind) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(ListObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) ([]unstructured.Unstructured, error) {
			return []unstructured.Unstructured{{}}, nil
		})

	w := &v1.Workload{}
	objs, err := ListObjectsByWorkload(context.Background(), nil, nil, w)
	assert.NilError(t, err)
	assert.Equal(t, len(objs), 1)
}

func TestDeleteObjectsByWorkload(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetWorkloadGVK, func(*v1.Workload) []schema.GroupVersionKind {
		return []schema.GroupVersionKind{{Group: "batch", Version: "v1", Kind: "Job"}}
	})
	patches.ApplyFunc(commonworkload.GetResourceTemplateByGVK,
		func(context.Context, ctrlClient.Client, schema.GroupVersionKind) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(ListObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) ([]unstructured.Unstructured, error) {
			return []unstructured.Unstructured{{}}, nil
		})
	patches.ApplyFunc(DeleteObject,
		func(context.Context, *commonclient.ClientFactory, *unstructured.Unstructured) error { return nil })

	w := &v1.Workload{}
	found, err := DeleteObjectsByWorkload(context.Background(), nil, nil, w)
	assert.NilError(t, err)
	assert.Equal(t, found, true)
}

func TestDeleteObjectsByWorkloadEmpty(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetWorkloadGVK, func(*v1.Workload) []schema.GroupVersionKind {
		return []schema.GroupVersionKind{{Group: "batch", Version: "v1", Kind: "Job"}}
	})
	patches.ApplyFunc(commonworkload.GetResourceTemplateByGVK,
		func(context.Context, ctrlClient.Client, schema.GroupVersionKind) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(ListObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) ([]unstructured.Unstructured, error) {
			return nil, nil
		})

	w := &v1.Workload{}
	found, err := DeleteObjectsByWorkload(context.Background(), nil, nil, w)
	assert.NilError(t, err)
	// No objects -> nothing deleted.
	assert.Equal(t, found, false)
}
