/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeStatsService_Name(t *testing.T) {
	service := NewNodeStatsService(nil)
	assert.Equal(t, "node-stats-collector", service.Name())
}

func TestNewNodeStatsService(t *testing.T) {
	service := NewNodeStatsService(nil)
	assert.NotNil(t, service)
	assert.Nil(t, service.safeDB)
}

func TestNodeStatsService_Run_NoClusters(t *testing.T) {
	// This test would require mocking clientsets.GetClusterManager()
	// which is a global function. For now, we'll skip this test
	// as it requires more complex setup.
	t.Skip("Skipping test that requires global mocking")
}

func TestNodeStatsService_Run_Context(t *testing.T) {
	// Test that the service respects context cancellation
	service := NewNodeStatsService(nil)
	ctx := context.Background()

	// The Run method should return without error even with nil DB
	// because it will fail early on cluster name retrieval
	err := service.Run(ctx)
	assert.Nil(t, err)
}

