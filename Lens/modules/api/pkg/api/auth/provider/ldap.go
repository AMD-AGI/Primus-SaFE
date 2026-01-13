// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package provider

import (
	"context"
	"fmt"

	ldappkg "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth/ldap"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// LDAPProvider implements AuthProvider for LDAP authentication
type LDAPProvider struct {
	name        string
	displayName string
	enabled     bool
	ldapClient  *ldappkg.Provider
}

// NewLDAPProvider creates a new LDAP provider from database config
func NewLDAPProvider(cfg *model.LensAuthProviders) (*LDAPProvider, error) {
	displayName := cfg.Name
	if dn, ok := cfg.Config["display_name"].(string); ok && dn != "" {
		displayName = dn
	}

	// Create LDAP client from config
	ldapClient, err := ldappkg.NewProviderFromMap(cfg.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create LDAP provider: %w", err)
	}

	return &LDAPProvider{
		name:        cfg.Name,
		displayName: displayName,
		enabled:     cfg.Enabled,
		ldapClient:  ldapClient,
	}, nil
}

func (p *LDAPProvider) Name() string        { return p.name }
func (p *LDAPProvider) Type() string        { return "ldap" }
func (p *LDAPProvider) DisplayName() string { return p.displayName }
func (p *LDAPProvider) IsEnabled() bool     { return p.enabled }

// NeedsExternalRedirect returns false because LDAP uses built-in login form
func (p *LDAPProvider) NeedsExternalRedirect() bool { return false }

// GetAuthorizeURL returns empty for LDAP (uses built-in login form)
func (p *LDAPProvider) GetAuthorizeURL(req *AuthorizeRequest) (string, error) {
	return "", nil
}

// HandleCallback handles LDAP authentication with username/password
func (p *LDAPProvider) HandleCallback(ctx context.Context, params map[string]string) (*UserInfo, error) {
	username := params["username"]
	password := params["password"]

	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	// Authenticate with LDAP using Credentials struct
	creds := &ldappkg.Credentials{
		Username: username,
		Password: password,
	}
	result, err := p.ldapClient.Authenticate(ctx, creds)
	if err != nil {
		log.Debugf("LDAP authentication failed for user %s: %v", username, err)
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("authentication failed: %s", result.FailReason)
	}

	ldapUser := result.User
	return &UserInfo{
		ID:          ldapUser.ExternalID, // Use LDAP DN as ID
		Username:    ldapUser.Username,
		Email:       ldapUser.Email,
		DisplayName: ldapUser.DisplayName,
		Groups:      ldapUser.Groups,
		IsAdmin:     ldapUser.IsAdmin,
	}, nil
}

// ValidateToken is not supported for LDAP
func (p *LDAPProvider) ValidateToken(ctx context.Context, token string) (*UserInfo, error) {
	return nil, fmt.Errorf("token validation not supported for LDAP")
}

// Close closes the LDAP connection
func (p *LDAPProvider) Close() {
	if p.ldapClient != nil {
		p.ldapClient.Close()
	}
}
