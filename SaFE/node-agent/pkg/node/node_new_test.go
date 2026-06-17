/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"context"
	"testing"

	"gotest.tools/assert"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestNewNodeOutsideCluster returns an error when no in-cluster config is available.
func TestNewNodeOutsideCluster(t *testing.T) {
	ctx := context.Background()
	opts := &types.Options{NodeName: "test-node"}
	_, err := NewNode(ctx, opts)
	assert.Assert(t, err != nil)
}
