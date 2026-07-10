/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newConfigReadRBACHandler seeds a private remote_api model in ws-2 (owned by
// stranger-1) so the SFT/RL config read paths can be exercised against read
// visibility. The DB client is nil, so lookups take the K8s path.
func newConfigReadRBACHandler(t *testing.T) *Handler {
	t.Helper()
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(
		newReadModel("m-ws2", "stranger-1", "ws-2"),
	).Build()
	return &Handler{k8sClient: k8s, accessController: newReadRBACAC(t)}
}

// TestGetSftConfigReadRBAC verifies getSftConfig enforces model read visibility
// before returning the model's SFT configuration.
func TestGetSftConfigReadRBAC(t *testing.T) {
	h := newConfigReadRBACHandler(t)
	id := gin.Params{{Key: "id", Value: "m-ws2"}}

	// member-1 (member of ws-1, not ws-2, not owner) is denied.
	_, err := h.getSftConfig(readRBACCtx("member-1", "workspace=ws-2", id))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")

	// The owner and a system admin pass the visibility gate.
	_, err = h.getSftConfig(readRBACCtx("stranger-1", "workspace=ws-2", id))
	assert.NoError(t, err)
	_, err = h.getSftConfig(readRBACCtx("admin-1", "workspace=ws-2", id))
	assert.NoError(t, err)
}

// TestGetRlConfigReadRBAC verifies getRlConfig enforces model read visibility
// before returning the model's RL configuration.
func TestGetRlConfigReadRBAC(t *testing.T) {
	h := newConfigReadRBACHandler(t)
	id := gin.Params{{Key: "id", Value: "m-ws2"}}

	// member-1 is denied.
	_, err := h.getRlConfig(readRBACCtx("member-1", "workspace=ws-2", id))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")

	// The owner and a system admin pass the visibility gate.
	_, err = h.getRlConfig(readRBACCtx("stranger-1", "workspace=ws-2", id))
	assert.NoError(t, err)
	_, err = h.getRlConfig(readRBACCtx("admin-1", "workspace=ws-2", id))
	assert.NoError(t, err)
}
