// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package oidc provides SaFE session validation functionality.
// This is a simplified package that only validates sessions against SaFE DB.
// It does NOT implement a full OIDC provider.
package oidc

// UserInfo represents user information from SaFE
type UserInfo struct {
	ID          string `json:"sub"`
	Username    string `json:"preferred_username,omitempty"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"name,omitempty"`
	IsAdmin     bool   `json:"is_admin,omitempty"`
}

// ValidateRequest represents the session validation request
type ValidateRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

// ValidateResponse represents the session validation response
type ValidateResponse struct {
	Valid   bool   `json:"valid"`
	UserID  string `json:"user_id,omitempty"`
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
	IsAdmin bool   `json:"is_admin,omitempty"`
	Error   string `json:"error,omitempty"`
}
