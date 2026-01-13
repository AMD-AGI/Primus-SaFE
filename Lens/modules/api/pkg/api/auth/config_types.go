// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"time"
)

// SystemConfigResponse represents a system config in API responses
type SystemConfigResponse struct {
	Key         string      `json:"key"`
	Value       interface{} `json:"value"`
	Category    string      `json:"category"`
	Description string      `json:"description,omitempty"`
	IsSecret    bool        `json:"isSecret"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

// ListSystemConfigsResponse is the response for listing system configs
type ListSystemConfigsResponse struct {
	Configs []*SystemConfigResponse `json:"configs"`
}

// UpdateSystemConfigRequest is the request for updating a system config
type UpdateSystemConfigRequest struct {
	Value       interface{} `json:"value" binding:"required"`
	Description string      `json:"description,omitempty"`
}
