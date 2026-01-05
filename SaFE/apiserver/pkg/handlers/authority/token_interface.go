/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TokenInput represents the input parameters for token generation
// It supports both username/password authentication and OAuth2 authorization code flow
type TokenInput struct {
	Username string
	Password string
	Code     string
}

// TokenResponse represents the response structure for token operations
type TokenResponse struct {
	// The timestamp when the user token expires, in seconds.
	Expire int64 `json:"expire"`
	// User token
	Token string `json:"token"`
}

// UserInfo represents user information extracted from ID token
type UserInfo struct {
	// User unique identifier, internally generated.
	Id string `json:"id,omitempty"`
	// User name
	Name string `json:"name,omitempty"`
	// A locally unique and never reassigned identifier within the Issuer for the End-User,
	Sub string `json:"sub,omitempty"`
	// expire time of token
	Exp int64 `json:"exp,omitempty"`
	// User time
	Email string `json:"email,omitempty"`
}

// TokenInterface defines the contract for token management operations
type TokenInterface interface {
	// Login authenticates a user based on TokenInput and returns user info and token response
	// For sso flow, use the Code field; for local auth, uses Username and Password fields
	Login(ctx context.Context, input TokenInput) (*v1.User, *TokenResponse, error)

	// Validate verifies a token string and extracts user information
	// Returns UserInfo if token is valid, error otherwise
	Validate(ctx context.Context, rawToken string) (*UserInfo, error)
}
