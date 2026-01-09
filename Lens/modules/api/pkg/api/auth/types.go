// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	cpauth "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth"
)

// InitStatusResponse is the response for GET /init/status
type InitStatusResponse struct {
	Initialized    bool            `json:"initialized"`
	AuthMode       cpauth.AuthMode `json:"authMode,omitempty"`
	SafeDetected   bool            `json:"safeDetected,omitempty"`
	SuggestedMode  cpauth.AuthMode `json:"suggestedMode,omitempty"`
	RootUserExists bool            `json:"rootUserExists,omitempty"`
}

// InitSetupRequest is the request for POST /init/setup
type InitSetupRequest struct {
	AuthMode     cpauth.AuthMode `json:"authMode"`
	RootPassword string          `json:"rootPassword,omitempty"`
}

// InitSetupResponse is the response for POST /init/setup
type InitSetupResponse struct {
	Initialized bool            `json:"initialized"`
	AuthMode    cpauth.AuthMode `json:"authMode"`
	RootUser    *RootUserInfo   `json:"rootUser,omitempty"`
}

// RootUserInfo contains root user information
type RootUserInfo struct {
	Username           string `json:"username"`
	MustChangePassword bool   `json:"mustChangePassword"`
	GeneratedPassword  string `json:"generatedPassword,omitempty"`
}

// AuthModeResponse is the response for GET /admin/auth/mode
type AuthModeResponse struct {
	Mode             cpauth.AuthMode     `json:"mode"`
	Initialized      bool                `json:"initialized"`
	SafeIntegration  *SafeIntegrationInfo `json:"safeIntegration,omitempty"`
	AvailableModes   []cpauth.AuthMode   `json:"availableModes"`
}

// SafeIntegrationInfo contains SaFE integration information
type SafeIntegrationInfo struct {
	Enabled      bool   `json:"enabled"`
	AutoDetected bool   `json:"autoDetected"`
	AdapterStatus string `json:"adapterStatus,omitempty"`
}

// SetAuthModeRequest is the request for PUT /admin/auth/mode
type SetAuthModeRequest struct {
	Mode cpauth.AuthMode `json:"mode" binding:"required"`
}

// SetAuthModeResponse is the response for PUT /admin/auth/mode
type SetAuthModeResponse struct {
	Mode    cpauth.AuthMode `json:"mode"`
	Message string          `json:"message"`
}

// ChangePasswordRequest is the request for POST /admin/root/change-password
type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required"`
}
