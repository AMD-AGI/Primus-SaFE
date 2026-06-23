/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// TestGetK8sObjectStatusNotFoundTreatedAsDeleted verifies that when the managed
// data-plane object is gone (e.g. failover deleted it for a restart), a NotFound is
// reported as K8sDeleted and the action is rewritten to ResourceDel, so the workload
// reschedules instead of being marked Failed and reaped by the TTL controller.
func TestGetK8sObjectStatusNotFoundTreatedAsDeleted(t *testing.T) {
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "autoscalingrunnersets"}, "w")
		})
	defer patches.Reset()

	r := &SyncerReconciler{}
	msg := &resourceMessage{
		name:      "w",
		namespace: "ns",
		action:    ResourceUpdate,
		gvk:       schema.GroupVersionKind{Kind: "AutoscalingRunnerSet"},
	}
	status, err := r.getK8sObjectStatus(context.Background(), msg, monkeyClientSets(), &v1.Workload{})
	assert.NilError(t, err)
	assert.Assert(t, status != nil)
	assert.Equal(t, status.Phase, string(v1.K8sDeleted))
	assert.Equal(t, msg.action, ResourceDel)
}

// TestGetK8sObjectStatusOtherErrorPropagated verifies non-NotFound errors are still
// surfaced as errors and the action is left untouched.
func TestGetK8sObjectStatusOtherErrorPropagated(t *testing.T) {
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return nil, errors.New("boom")
		})
	defer patches.Reset()

	r := &SyncerReconciler{}
	msg := &resourceMessage{
		name:      "w",
		namespace: "ns",
		action:    ResourceUpdate,
		gvk:       schema.GroupVersionKind{Kind: "AutoscalingRunnerSet"},
	}
	status, err := r.getK8sObjectStatus(context.Background(), msg, monkeyClientSets(), &v1.Workload{})
	assert.Assert(t, err != nil)
	assert.Assert(t, status == nil)
	assert.Equal(t, msg.action, ResourceUpdate)
}
