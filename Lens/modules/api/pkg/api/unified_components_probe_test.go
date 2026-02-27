// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleComponentsProbe_RequiresClusterManager(t *testing.T) {
	// handleComponentsProbe calls GetClusterManager(), which panics when the
	// cluster manager has not been initialized (e.g. in unit tests).
	// This test documents that behavior.
	ctx := context.Background()
	req := &ComponentsProbeRequest{Cluster: ""}
	var panicked interface{}
	func() {
		defer func() { panicked = recover() }()
		_, _ = handleComponentsProbe(ctx, req)
	}()
	require.NotNil(t, panicked, "handleComponentsProbe must panic when cluster manager is not initialized")
	assert.Contains(t, panicked.(string), "cluster manager not initialized",
		"panic message should mention cluster manager not initialized")
}
