// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package provider

import (
	"context"
)

// UserInfo represents authenticated user information
type UserInfo struct {
	ID          string            `json:"id"`
	Username    string            `json:"username"`
	Email       string            `json:"email"`
	DisplayName string            `json:"display_name"`
	Groups      []string          `json:"groups,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	IsAdmin     bool              `json:"is_admin"`
}

// AuthorizeRequest represents authorization request parameters
type AuthorizeRequest struct {
	State       string `json:"state"`
	Nonce       string `json:"nonce"`
	RedirectURI string `json:"redirect_uri"`
	Scope       string `json:"scope"`
}

// TokenResponse represents token endpoint response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// AuthProvider is the interface for pluggable authentication providers
type AuthProvider interface {
	// Name returns the provider name (from config)
	Name() string

	// Type returns the provider type (safe, oidc, ldap, local)
	Type() string

	// DisplayName returns the display name for login page
	DisplayName() string

	// GetAuthorizeURL returns the authorization URL for redirect
	// For OIDC: returns external IdP authorization URL
	// For SaFE: returns SaFE SSO page URL
	// For LDAP/Local: returns empty (uses built-in login form)
	GetAuthorizeURL(req *AuthorizeRequest) (string, error)

	// HandleCallback handles authentication callback
	// For OIDC: exchanges authorization code for tokens
	// For SaFE: validates cookie token
	// For LDAP: validates username/password
	HandleCallback(ctx context.Context, params map[string]string) (*UserInfo, error)

	// ValidateToken validates an existing token (for session recovery)
	ValidateToken(ctx context.Context, token string) (*UserInfo, error)

	// NeedsExternalRedirect returns whether external redirect is needed
	// true: OIDC, SaFE (needs redirect to external page)
	// false: LDAP, Local (uses built-in login form)
	NeedsExternalRedirect() bool

	// IsEnabled returns whether the provider is enabled
	IsEnabled() bool
}

// ProviderInfo represents provider information for listing
type ProviderInfo struct {
	Name                  string `json:"name"`
	Type                  string `json:"type"`
	DisplayName           string `json:"display_name"`
	Enabled               bool   `json:"enabled"`
	IsDefault             bool   `json:"is_default"`
	NeedsExternalRedirect bool   `json:"needs_external_redirect"`
}
