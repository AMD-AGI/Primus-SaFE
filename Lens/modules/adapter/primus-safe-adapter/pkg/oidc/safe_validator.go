// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	safedal "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
)

var (
	// ErrSessionNotFound is returned when session is not found
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExpired is returned when session has expired
	ErrSessionExpired = errors.New("session expired")
	// ErrUserNotFound is returned when user is not found
	ErrUserNotFound = errors.New("user not found")
)

// SafeValidator validates SaFE sessions by querying SaFE DB directly
type SafeValidator struct {
	safeDB *gorm.DB
}

// NewSafeValidator creates a new SafeValidator
// safeDB is the connection to SaFE database (user_tokens table)
func NewSafeValidator(safeDB *gorm.DB, _ *gorm.DB) *SafeValidator {
	return &SafeValidator{
		safeDB: safeDB,
	}
}

// ValidateSafeSession validates a session ID directly against SaFE DB
// The sessionID is the value stored in the "Token" cookie by SaFE
// This queries the user_tokens table in SaFE DB
func (v *SafeValidator) ValidateSafeSession(ctx context.Context, sessionID string) (*UserInfo, error) {
	if sessionID == "" {
		return nil, ErrSessionNotFound
	}

	if v.safeDB == nil {
		return nil, fmt.Errorf("SaFE database not configured")
	}

	// Query SaFE user_tokens table using DAL
	// This is the same table that SaFE's sso_token.Validate() queries
	dal := safedal.Use(v.safeDB)
	ut := dal.UserToken

	now := time.Now().Unix()
	// Query: WHERE session_id = ? AND expire_time > now()
	token, err := ut.WithContext(ctx).
		Where(ut.SessionID.Eq(sessionID)).
		Where(ut.ExpireTime.Gt(now)).
		First()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Debugf("Session not found or expired in SaFE DB: %s", truncateSessionID(sessionID))
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("database query failed: %w", err)
	}

	// SaFE doesn't have a separate users table - user info comes from SSO/LDAP
	// UserID in user_tokens is the username from authentication
	log.Debugf("Session validated for user: %s", token.UserID)

	return &UserInfo{
		ID:          token.UserID,
		Username:    token.UserID,
		Email:       "", // Not stored in SaFE DB
		DisplayName: token.UserID,
		IsAdmin:     false, // Will be determined by Lens based on user mapping
	}, nil
}

// truncateSessionID truncates session ID for logging (security)
func truncateSessionID(sessionID string) string {
	if len(sessionID) <= 8 {
		return "***"
	}
	return sessionID[:8] + "..."
}
