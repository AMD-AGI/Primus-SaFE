/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

import (
	"testing"

	"gotest.tools/assert"
)

// TestIsChannelClosed verifies channel-closed detection for nil, open and closed channels.
func TestIsChannelClosed(t *testing.T) {
	assert.Equal(t, IsChannelClosed(nil), true)

	ch := make(chan struct{})
	assert.Equal(t, IsChannelClosed(ch), false)

	close(ch)
	assert.Equal(t, IsChannelClosed(ch), true)
}

// TestIsStopped verifies the tomb reports its stopped state.
func TestIsStopped(t *testing.T) {
	tomb := NewTomb()
	assert.Equal(t, tomb.IsStopped(), false)
}
