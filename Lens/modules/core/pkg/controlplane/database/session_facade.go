// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
)

// SessionFacadeInterface defines Session database operations
type SessionFacadeInterface interface {
	Create(ctx context.Context, session *model.LensSessions) error
	GetByID(ctx context.Context, id string) (*model.LensSessions, error)
	GetValidByTokenHash(ctx context.Context, tokenHash string) (*model.LensSessions, error)
	Revoke(ctx context.Context, sessionID string, reason string) error
	RevokeAllForUser(ctx context.Context, userID string, reason string) error
	GetUserSessions(ctx context.Context, userID string) ([]*model.LensSessions, error)
	CleanupExpired(ctx context.Context, before time.Time) (int64, error)
	GetBySafeSessionID(ctx context.Context, safeSessionID string) (*model.LensSessions, error)
	UpdateLastAccessed(ctx context.Context, sessionID string) error
	UpdateExpiresAt(ctx context.Context, sessionID string, expiresAt time.Time) error
	// ListBySyncSource lists sessions by sync source (e.g., "safe", "local")
	ListBySyncSource(ctx context.Context, source string) ([]*model.LensSessions, error)
}

// SessionFacade implements SessionFacadeInterface
type SessionFacade struct {
	BaseFacade
}

// NewSessionFacade creates a new SessionFacade
func NewSessionFacade() *SessionFacade {
	return &SessionFacade{}
}

// Create creates a new session
func (f *SessionFacade) Create(ctx context.Context, session *model.LensSessions) error {
	return f.getDB().WithContext(ctx).Create(session).Error
}

// GetByID gets a session by ID
func (f *SessionFacade) GetByID(ctx context.Context, id string) (*model.LensSessions, error) {
	var session model.LensSessions
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetValidByTokenHash gets a valid session by token hash
func (f *SessionFacade) GetValidByTokenHash(ctx context.Context, tokenHash string) (*model.LensSessions, error) {
	var session model.LensSessions
	err := f.getDB().WithContext(ctx).
		Where("token_hash = ? AND is_revoked = false AND expires_at > ?",
			tokenHash, time.Now()).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// Revoke revokes a session
func (f *SessionFacade) Revoke(ctx context.Context, sessionID string, reason string) error {
	now := time.Now()
	return f.getDB().WithContext(ctx).
		Model(&model.LensSessions{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"is_revoked":    true,
			"revoked_at":    now,
			"revoke_reason": reason,
		}).Error
}

// RevokeAllForUser revokes all sessions for a user
func (f *SessionFacade) RevokeAllForUser(ctx context.Context, userID string, reason string) error {
	now := time.Now()
	return f.getDB().WithContext(ctx).
		Model(&model.LensSessions{}).
		Where("user_id = ? AND is_revoked = false", userID).
		Updates(map[string]interface{}{
			"is_revoked":    true,
			"revoked_at":    now,
			"revoke_reason": reason,
		}).Error
}

// GetUserSessions gets all active sessions for a user
func (f *SessionFacade) GetUserSessions(ctx context.Context, userID string) ([]*model.LensSessions, error) {
	var sessions []*model.LensSessions
	err := f.getDB().WithContext(ctx).
		Where("user_id = ? AND is_revoked = false AND expires_at > ?", userID, time.Now()).
		Order("created_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// CleanupExpired removes expired or revoked sessions
func (f *SessionFacade) CleanupExpired(ctx context.Context, before time.Time) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Where("expires_at < ? OR (is_revoked = true AND revoked_at < ?)", before, before).
		Delete(&model.LensSessions{})
	return result.RowsAffected, result.Error
}

// GetBySafeSessionID gets a session by SaFE session ID
func (f *SessionFacade) GetBySafeSessionID(ctx context.Context, safeSessionID string) (*model.LensSessions, error) {
	var session model.LensSessions
	err := f.getDB().WithContext(ctx).
		Where("safe_session_id = ?", safeSessionID).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateLastAccessed updates the last accessed time for a session
func (f *SessionFacade) UpdateLastAccessed(ctx context.Context, sessionID string) error {
	return f.getDB().WithContext(ctx).
		Model(&model.LensSessions{}).
		Where("id = ?", sessionID).
		Update("last_accessed", time.Now()).Error
}

// UpdateExpiresAt updates the expiration time for a session
func (f *SessionFacade) UpdateExpiresAt(ctx context.Context, sessionID string, expiresAt time.Time) error {
	return f.getDB().WithContext(ctx).
		Model(&model.LensSessions{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"expires_at":    expiresAt,
			"last_accessed": time.Now(),
		}).Error
}

// ListBySyncSource lists sessions by sync source (e.g., "safe", "local")
func (f *SessionFacade) ListBySyncSource(ctx context.Context, source string) ([]*model.LensSessions, error) {
	var sessions []*model.LensSessions
	err := f.getDB().WithContext(ctx).
		Where("sync_source = ?", source).
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// Ensure SessionFacade implements SessionFacadeInterface
var _ SessionFacadeInterface = (*SessionFacade)(nil)
