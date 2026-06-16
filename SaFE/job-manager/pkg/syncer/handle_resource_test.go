/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestHandleResourceWrongType(t *testing.T) {
	called := false
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { called = true }),
	}
	// Not an *unstructured.Unstructured -> ignored.
	c.handleResource(context.Background(), nil, &corev1.Pod{}, ResourceAdd)
	assert.Equal(t, called, false)
}

func TestHandleResourceNoWorkloadId(t *testing.T) {
	called := false
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { called = true }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Pod")
	u.SetName("p1")
	// No workload-id label and no mesh label -> not a managed object, ignored.
	c.handleResource(context.Background(), nil, u, ResourceAdd)
	assert.Equal(t, called, false)
}

func TestHandleResourceManaged(t *testing.T) {
	var captured *resourceMessage
	c := &ClusterClientSets{
		name:    "cl",
		handler: ResourceHandler(func(m *resourceMessage) { captured = m }),
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Job")
	u.SetName("obj")
	u.SetNamespace("ns")
	u.SetLabels(map[string]string{
		v1.WorkloadIdLabel:          "w",
		v1.WorkloadDispatchCntLabel: "3",
	})
	c.handleResource(context.Background(), nil, u, ResourceAdd)
	assert.Assert(t, captured != nil)
	assert.Equal(t, captured.workloadId, "w")
	assert.Equal(t, captured.dispatchCount, 3)
	assert.Equal(t, captured.cluster, "cl")
}
