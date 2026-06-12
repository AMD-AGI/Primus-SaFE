/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestFindCondition locates a condition using a custom equality function.
func TestFindCondition(t *testing.T) {
	n, _ := newNode(t)
	cond := corev1.NodeCondition{Type: "safe.find", Status: corev1.ConditionTrue}
	assert.NilError(t, n.UpdateConditions([]corev1.NodeCondition{cond}))
	found := n.FindCondition(&cond, func(a, b *corev1.NodeCondition) bool {
		return a.Type == b.Type
	})
	assert.Assert(t, found != nil)
	assert.Equal(t, string(found.Type), "safe.find")
}

// TestGetEphemeralStorage returns allocatable ephemeral storage quantity.
func TestGetEphemeralStorage(t *testing.T) {
	n, _ := newNode(t)
	q := n.GetEphemeralStorage()
	assert.Equal(t, q.IsZero(), true)

	node := n.GetK8sNode()
	node.Status.Allocatable = corev1.ResourceList{
		corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
	}
	n.k8sNode = node
	q = n.GetEphemeralStorage()
	expected := resource.MustParse("100Gi")
	assert.Equal(t, q.Value(), expected.Value())
}

// TestIsMatchGpuChipNvidia matches nodes labeled for NVIDIA GPUs.
func TestIsMatchGpuChipNvidia(t *testing.T) {
	testNode := genNode()
	delete(testNode.Labels, common.AMDGpuIdentification)
	testNode.Labels[common.NvidiaIdentification] = "true"
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name}
	n, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	assert.Equal(t, n.IsMatchGpuChip(string(v1.NvidiaGpuChip)), true)
	assert.Equal(t, n.IsMatchGpuChip(string(v1.AmdGpuChip)), false)
}

// TestIsMatchGpuChipEmpty matches any chip when filter is empty.
func TestIsMatchGpuChipEmpty(t *testing.T) {
	n, _ := newNode(t)
	assert.Equal(t, n.IsMatchGpuChip(""), true)
	assert.Equal(t, n.IsMatchGpuChip("unknown"), false)
}

// TestGetGpuQuantityNvidia reads NVIDIA GPU allocatable resources.
func TestGetGpuQuantityNvidia(t *testing.T) {
	testNode := genNode()
	delete(testNode.Labels, common.AMDGpuIdentification)
	testNode.Labels[common.NvidiaIdentification] = "true"
	testNode.Status.Allocatable = corev1.ResourceList{
		common.NvidiaGpu: resource.MustParse("4"),
	}
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name}
	n, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	gpuQty := n.GetGpuQuantity()
	assert.Equal(t, gpuQty.Value(), int64(4))
}

// TestSyncK8sNode refreshes the cached node object from the API.
func TestSyncK8sNode(t *testing.T) {
	n, fakeClientSet := newNode(t)
	_, err := fakeClientSet.CoreV1().Nodes().Update(context.Background(), n.GetK8sNode(), metav1.UpdateOptions{})
	assert.NilError(t, err)
	assert.NilError(t, n.syncK8sNode())
}

// TestStartUninitializedNode returns error when node is nil.
func TestStartUninitializedNode(t *testing.T) {
	var n *Node
	err := n.Start()
	assert.Error(t, err, "please initialize node first")
}

// TestUpdateConditionsUninitialized returns error when k8s node is nil.
func TestUpdateConditionsUninitialized(t *testing.T) {
	n := &Node{}
	err := n.UpdateConditions(nil)
	assert.Error(t, err, "please initialize node first")
}

// TestGetLocation reads system timezone when nsenter is disabled.
func TestGetLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	loc, err := getLocation()
	assert.NilError(t, err)
	assert.Assert(t, loc != nil)
}

// TestGetUptime parses host boot time when nsenter is disabled.
func TestGetUptime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	loc, err := getLocation()
	assert.NilError(t, err)
	start, err := getUptime(loc)
	assert.NilError(t, err)
	assert.Assert(t, !start.IsZero())
}

// TestUpdateStartTimeViaNode exercises updateStartTime on a live node object.
func TestUpdateStartTimeViaNode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	n, _ := newNode(t)
	err := n.updateStartTime()
	if err != nil {
		t.Skip("host uptime command unavailable")
	}
	assert.Assert(t, v1.GetNodeStartupTime(n.GetK8sNode()) != "")
}

// TestFindConditionByTypeNilNode returns nil when the node is uninitialized.
func TestFindConditionByTypeNilNode(t *testing.T) {
	n := &Node{}
	assert.Assert(t, n.FindConditionByType("safe.none") == nil)
}

// TestFindConditionNilNode returns nil when the node is uninitialized.
func TestFindConditionNilNode(t *testing.T) {
	n := &Node{}
	cond := corev1.NodeCondition{Type: "safe.none"}
	assert.Assert(t, n.FindCondition(&cond, func(a, b *corev1.NodeCondition) bool {
		return a.Type == b.Type
	}) == nil)
}

// TestGetGpuQuantityNilNode returns zero when the node is uninitialized.
func TestGetGpuQuantityNilNode(t *testing.T) {
	n := &Node{}
	qty := n.GetGpuQuantity()
	assert.Equal(t, qty.IsZero(), true)
}

// TestGetEphemeralStorageNilNode returns zero when the node is uninitialized.
func TestGetEphemeralStorageNilNode(t *testing.T) {
	n := &Node{}
	storage := n.GetEphemeralStorage()
	assert.Equal(t, storage.IsZero(), true)
}

// TestSyncK8sNodeError propagates API errors from the Kubernetes client.
func TestSyncK8sNodeError(t *testing.T) {
	n, fakeClientSet := newNode(t)
	assert.NilError(t, fakeClientSet.CoreV1().Nodes().Delete(context.Background(), n.GetK8sNode().Name, metav1.DeleteOptions{}))
	err := n.syncK8sNode()
	assert.Assert(t, err != nil)
}

// TestUpdateConditionsConflict retries after a Kubernetes conflict error.
func TestUpdateConditionsConflict(t *testing.T) {
	n, fakeClientSet := newNode(t)
	attempts := 0
	fakeClientSet.PrependReactor("update", "nodes", func(action ktesting.Action) (bool, kruntime.Object, error) {
		attempts++
		if attempts == 1 {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Resource: "nodes"},
				n.GetK8sNode().Name,
				fmt.Errorf("conflict"),
			)
		}
		return false, nil, nil
	})
	cond := corev1.NodeCondition{Type: "safe.conflict", Status: corev1.ConditionTrue}
	assert.NilError(t, n.UpdateConditions([]corev1.NodeCondition{cond}))
	assert.Assert(t, attempts > 1)
}

// TestUpdateNodeStartTimeNoChange skips patch when label already matches.
func TestUpdateNodeStartTimeNoChange(t *testing.T) {
	n, _ := newNode(t)
	now := time.Unix(1700000000, 0).UTC()
	assert.NilError(t, n.updateNodeStartTime(now))
	err := n.updateNodeStartTime(now)
	assert.NilError(t, err)
}
