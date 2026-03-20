/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2ahandlers

import (
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// Handler holds dependencies for A2A API handlers.
type Handler struct {
	dbClient dbclient.Interface
}

// NewHandler creates a new A2A handler.
func NewHandler(dbClient dbclient.Interface) *Handler {
	return &Handler{dbClient: dbClient}
}
