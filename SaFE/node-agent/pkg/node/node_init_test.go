/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	"k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestNewNodeWithClientSetNotFound returns an error when the node does not exist.
func TestNewNodeWithClientSetNotFound(t *testing.T) {
	fakeClientSet := fake.NewClientset()
	opts := &types.Options{NodeName: "missing-node"}
	_, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.Assert(t, err != nil)
}

// TestIsMatchGpuChipInvalidAmdLabel treats non-true AMD labels as mismatched.
func TestIsMatchGpuChipInvalidAmdLabel(t *testing.T) {
	testNode := genNode()
	testNode.Labels[common.AMDGpuIdentification] = "false"
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name}
	n, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	assert.Equal(t, n.IsMatchGpuChip(string(v1.AmdGpuChip)), false)
}

// TestNodeUpdateStopsOnCancel exits the sync loop when the context is cancelled.
func TestNodeUpdateStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n, _ := newNode(t)
	n.ctx = ctx
	go n.update()
	cancel()
	time.Sleep(150 * time.Millisecond)
}
