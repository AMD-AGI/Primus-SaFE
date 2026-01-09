// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Event types
const (
	EventTypeLogin         = "login"
	EventTypeLogout        = "logout"
	EventTypeLoginFailed   = "login_failed"
	EventTypeSessionExpire = "session_expire"
	EventTypeSessionRevoke = "session_revoke"
	EventTypePasswordChange = "password_change"
)

// Auth types
const (
	AuthTypeLocal = "local"
	AuthTypeLDAP  = "ldap"
	AuthTypeSaFE  = "safe"
	AuthTypeOIDC  = "oidc"
)

// AuditEntry represents a login audit entry
type AuditEntry struct {
	Username      string
	UserID        string
	EventType     string
	AuthType      string
	IPAddress     string
	UserAgent     string
	Success       bool
	FailureReason string
}

// Service provides login audit functionality
type Service struct {
	retentionDays int
}

// NewService creates a new audit service
func NewService() *Service {
	return &Service{
		retentionDays: 90, // Default 90 days retention
	}
}

// NewServiceWithRetention creates a new audit service with custom retention
func NewServiceWithRetention(retentionDays int) *Service {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	return &Service{
		retentionDays: retentionDays,
	}
}

// RecordLogin records a successful login
func (s *Service) RecordLogin(ctx context.Context, username, userID, authType, ipAddress, userAgent string) error {
	return s.record(ctx, &AuditEntry{
		Username:  username,
		UserID:    userID,
		EventType: EventTypeLogin,
		AuthType:  authType,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Success:   true,
	})
}

// RecordLoginFailed records a failed login attempt
func (s *Service) RecordLoginFailed(ctx context.Context, username, authType, ipAddress, userAgent, reason string) error {
	return s.record(ctx, &AuditEntry{
		Username:      username,
		EventType:     EventTypeLoginFailed,
		AuthType:      authType,
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		Success:       false,
		FailureReason: reason,
	})
}

// RecordLogout records a logout event
func (s *Service) RecordLogout(ctx context.Context, username, userID, ipAddress, userAgent string) error {
	return s.record(ctx, &AuditEntry{
		Username:  username,
		UserID:    userID,
		EventType: EventTypeLogout,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Success:   true,
	})
}

// RecordSessionExpire records a session expiration
func (s *Service) RecordSessionExpire(ctx context.Context, username, userID string) error {
	return s.record(ctx, &AuditEntry{
		Username:  username,
		UserID:    userID,
		EventType: EventTypeSessionExpire,
		Success:   true,
	})
}

// RecordSessionRevoke records a session revocation
func (s *Service) RecordSessionRevoke(ctx context.Context, username, userID, reason string) error {
	return s.record(ctx, &AuditEntry{
		Username:      username,
		UserID:        userID,
		EventType:     EventTypeSessionRevoke,
		Success:       true,
		FailureReason: reason, // Use failure_reason to store revoke reason
	})
}

// RecordPasswordChange records a password change event
func (s *Service) RecordPasswordChange(ctx context.Context, username, userID, ipAddress, userAgent string, success bool, reason string) error {
	return s.record(ctx, &AuditEntry{
		Username:      username,
		UserID:        userID,
		EventType:     EventTypePasswordChange,
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		Success:       success,
		FailureReason: reason,
	})
}

// record creates an audit record in the database
func (s *Service) record(ctx context.Context, entry *AuditEntry) error {
	audit := &model.LensLoginAudit{
		ID:            generateAuditID(),
		Username:      entry.Username,
		UserID:        entry.UserID,
		EventType:     entry.EventType,
		AuthType:      entry.AuthType,
		IPAddress:     entry.IPAddress,
		UserAgent:     truncateUserAgent(entry.UserAgent),
		Success:       entry.Success,
		FailureReason: entry.FailureReason,
		CreatedAt:     time.Now(),
	}

	auditFacade := cpdb.GetFacade().GetLoginAudit()
	if err := auditFacade.Create(ctx, audit); err != nil {
		log.Errorf("Failed to record audit event: %v", err)
		return fmt.Errorf("failed to record audit event: %w", err)
	}

	log.Debugf("Recorded audit event: type=%s, username=%s, success=%v",
		entry.EventType, entry.Username, entry.Success)
	return nil
}

// GetRecentLogins gets recent login audit records
func (s *Service) GetRecentLogins(ctx context.Context, limit int) ([]*model.LensLoginAudit, error) {
	if limit <= 0 {
		limit = 100
	}

	auditFacade := cpdb.GetFacade().GetLoginAudit()
	return auditFacade.ListRecent(ctx, limit)
}

// GetUserLoginHistory gets login history for a specific user
func (s *Service) GetUserLoginHistory(ctx context.Context, userID string, offset, limit int) ([]*model.LensLoginAudit, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	auditFacade := cpdb.GetFacade().GetLoginAudit()
	return auditFacade.ListByUserID(ctx, userID, offset, limit)
}

// GetLoginsByUsername gets login records for a username
func (s *Service) GetLoginsByUsername(ctx context.Context, username string, offset, limit int) ([]*model.LensLoginAudit, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	auditFacade := cpdb.GetFacade().GetLoginAudit()
	return auditFacade.ListByUsername(ctx, username, offset, limit)
}

// Cleanup removes old audit records
func (s *Service) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)

	auditFacade := cpdb.GetFacade().GetLoginAudit()
	count, err := auditFacade.CleanupOld(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup audit records: %w", err)
	}

	if count > 0 {
		log.Infof("Cleaned up %d old audit records (older than %d days)", count, s.retentionDays)
	}

	return count, nil
}

// Helper functions

func generateAuditID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("audit-%s", hex.EncodeToString(bytes))
}

func truncateUserAgent(ua string) string {
	const maxLen = 500
	if len(ua) > maxLen {
		return ua[:maxLen]
	}
	return ua
}
