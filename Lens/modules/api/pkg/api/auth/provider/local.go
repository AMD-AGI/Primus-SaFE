// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package provider

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// LocalProvider implements AuthProvider for local database authentication
type LocalProvider struct {
	name        string
	displayName string
	enabled     bool
}

// NewLocalProvider creates a new local provider from database config
func NewLocalProvider(cfg *model.LensAuthProviders) (*LocalProvider, error) {
	displayName := cfg.Name
	if dn, ok := cfg.Config["display_name"].(string); ok && dn != "" {
		displayName = dn
	}

	return &LocalProvider{
		name:        cfg.Name,
		displayName: displayName,
		enabled:     cfg.Enabled,
	}, nil
}

func (p *LocalProvider) Name() string        { return p.name }
func (p *LocalProvider) Type() string        { return "local" }
func (p *LocalProvider) DisplayName() string { return p.displayName }
func (p *LocalProvider) IsEnabled() bool     { return p.enabled }

// NeedsExternalRedirect returns false because local auth uses built-in login form
func (p *LocalProvider) NeedsExternalRedirect() bool { return false }

// GetAuthorizeURL returns empty for local auth (uses built-in login form)
func (p *LocalProvider) GetAuthorizeURL(req *AuthorizeRequest) (string, error) {
	return "", nil
}

// HandleCallback handles local authentication with username/password
func (p *LocalProvider) HandleCallback(ctx context.Context, params map[string]string) (*UserInfo, error) {
	username := params["username"]
	password := params["password"]

	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	// Get user from database
	userFacade := cpdb.GetFacade().GetUser()
	user, err := userFacade.GetByUsername(ctx, username)
	if err != nil {
		log.Debugf("User not found: %s", username)
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if user has local auth type
	if user.AuthType != "local" && user.AuthType != "root" {
		return nil, fmt.Errorf("user does not use local authentication")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		log.Debugf("Password verification failed for user %s", username)
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if user is active
	if user.Status != "active" {
		return nil, fmt.Errorf("user account is not active")
	}

	return &UserInfo{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin || user.IsRoot,
	}, nil
}

// ValidateToken is not supported for local auth
func (p *LocalProvider) ValidateToken(ctx context.Context, token string) (*UserInfo, error) {
	return nil, fmt.Errorf("token validation not supported for local auth")
}
