/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package service

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
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
	// Initialize cluster manager for testing
	// We disable both K8S and Storage client loading since we're just testing
	// that the service can handle no clusters scenario
	ctx := context.Background()
	err := clientsets.InitClusterManager(ctx, false, false, false)
	if err != nil {
		t.Fatalf("Failed to initialize cluster manager: %v", err)
	}

	// Test that the service handles no clusters gracefully
	service := NewNodeStatsService(nil)

	// The Run method should return without error when no clusters are found
	err = service.Run(ctx)
	assert.Nil(t, err)
}
