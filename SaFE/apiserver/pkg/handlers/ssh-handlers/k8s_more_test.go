/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestResolveExecContainer covers the WebShell/SSH container fallback: an explicit container is
// used as-is, while an empty or whitespace-only value falls back to the pod's main container
// (from the workload's main-container annotation).
func TestResolveExecContainer(t *testing.T) {
	wl := &v1.Workload{}
	wl.SetAnnotations(map[string]string{v1.MainContainerAnnotation: "pytorch"})

	// Explicit container is honored, no fallback.
	assert.Equal(t, "custom", resolveExecContainer(wl, "pod-0", "custom"))

	// Empty / whitespace-only falls back to the pod's main container.
	assert.Equal(t, "pytorch", resolveExecContainer(wl, "pod-0", ""))
	assert.Equal(t, "pytorch", resolveExecContainer(wl, "pod-0", "   "))
}
