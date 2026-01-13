// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"time"
)

// ProviderType represents the type of authentication provider
type ProviderType string

const (
	ProviderTypeLDAP ProviderType = "ldap"
	ProviderTypeOIDC ProviderType = "oidc"
	ProviderTypeSafe ProviderType = "safe"
)

// ProviderStatus represents the status of an authentication provider
type ProviderStatus string

const (
	ProviderStatusActive   ProviderStatus = "active"
	ProviderStatusInactive ProviderStatus = "inactive"
	ProviderStatusError    ProviderStatus = "error"
)

// AuthProviderResponse represents an auth provider in API responses
type AuthProviderResponse struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        ProviderType   `json:"type"`
	Enabled     bool           `json:"enabled"`
	Priority    int            `json:"priority"`
	Status      ProviderStatus `json:"status"`
	Description string         `json:"description,omitempty"`
	LastCheckAt *time.Time     `json:"lastCheckAt,omitempty"`
	LastError   string         `json:"lastError,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	// Config is intentionally excluded from list response for security
}

// AuthProviderDetailResponse includes config for single provider detail
type AuthProviderDetailResponse struct {
	AuthProviderResponse
	Config map[string]interface{} `json:"config"`
}

// CreateAuthProviderRequest is the request for creating an auth provider
type CreateAuthProviderRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Type        ProviderType           `json:"type" binding:"required"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config" binding:"required"`
}

// UpdateAuthProviderRequest is the request for updating an auth provider
type UpdateAuthProviderRequest struct {
	Name        *string                `json:"name"`
	Enabled     *bool                  `json:"enabled"`
	Priority    *int                   `json:"priority"`
	Description *string                `json:"description"`
	Config      map[string]interface{} `json:"config"`
}

// TestAuthProviderResponse is the response for testing an auth provider
type TestAuthProviderResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// LDAPConfig represents LDAP provider configuration
type LDAPConfig struct {
	Host            string `json:"host" binding:"required"`
	Port            int    `json:"port"`
	UseSSL          bool   `json:"useSsl"`
	UseStartTLS     bool   `json:"useStartTls"`
	SkipTLSVerify   bool   `json:"skipTlsVerify"`
	BindDN          string `json:"bindDn" binding:"required"`
	BindPassword    string `json:"bindPassword"`
	BaseDN          string `json:"baseDn" binding:"required"`
	UserSearchBase  string `json:"userSearchBase"`
	UserSearchFilter string `json:"userSearchFilter"`
	UsernameAttr    string `json:"usernameAttr"`
	EmailAttr       string `json:"emailAttr"`
	DisplayNameAttr string `json:"displayNameAttr"`
	MemberOfAttr    string `json:"memberOfAttr"`
	AdminGroupDN    string `json:"adminGroupDn"`
	PoolSize        int    `json:"poolSize"`
	ConnTimeout     int    `json:"connTimeout"`
}

// OIDCConfig represents OIDC provider configuration
type OIDCConfig struct {
	Endpoint      string   `json:"endpoint" binding:"required"`
	ClientID      string   `json:"clientId" binding:"required"`
	ClientSecret  string   `json:"clientSecret"`
	RedirectURI   string   `json:"redirectUri"`
	Scopes        []string `json:"scopes"`
	UsernameClaim string   `json:"usernameClaim"`
	EmailClaim    string   `json:"emailClaim"`
	GroupsClaim   string   `json:"groupsClaim"`
}

// ListAuthProvidersResponse is the response for listing auth providers
type ListAuthProvidersResponse struct {
	Providers []*AuthProviderResponse `json:"providers"`
}
