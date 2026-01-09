// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// SessionTokenPrefix is the prefix for Lens session tokens
	SessionTokenPrefix = "lens_"
	// TokenLength is the length of the random token bytes
	TokenLength = 32
	// DefaultSyncSource is the default sync source for locally created sessions
	DefaultSyncSource = "local"
)

// DBManager implements Manager using database storage
type DBManager struct {
	config *Config
}

// NewDBManager creates a new database-backed session manager
func NewDBManager(config *Config) *DBManager {
	if config == nil {
		config = DefaultConfig()
	}
	return &DBManager{
		config: config,
	}
}

// Create creates a new session
func (m *DBManager) Create(ctx context.Context, opts *CreateOptions) (*SessionInfo, error) {
	// Generate random token
	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash token for storage
	tokenHash := hashToken(token)

	duration := opts.Duration
	if duration == 0 {
		duration = m.config.DefaultTTL
	}

	syncSource := opts.SyncSource
	if syncSource == "" {
		syncSource = DefaultSyncSource
	}

	now := time.Now()
	session := &model.LensSessions{
		ID:            generateSessionID(),
		UserID:        opts.UserID,
		TokenHash:     tokenHash,
		UserAgent:     opts.UserAgent,
		IPAddress:     opts.IPAddress,
		CreatedAt:     now,
		ExpiresAt:     now.Add(duration),
		LastAccessed:  now,
		IsRevoked:     false,
		SyncSource:    syncSource,
		SafeSessionID: opts.SafeSessionID,
	}

	sessionFacade := cpdb.GetFacade().GetSession()
	if err := sessionFacade.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	log.Debugf("Created session %s for user %s", session.ID, opts.UserID)

	return &SessionInfo{
		SessionID:    session.ID,
		Token:        token, // Return plaintext token only on creation
		UserID:       session.UserID,
		Username:     opts.Username,
		Email:        opts.Email,
		SyncSource:   syncSource,
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    session.ExpiresAt,
		LastAccessed: session.LastAccessed,
	}, nil
}

// Validate validates a session token and returns session info
func (m *DBManager) Validate(ctx context.Context, token string) (*SessionInfo, error) {
	tokenHash := hashToken(token)

	sessionFacade := cpdb.GetFacade().GetSession()
	session, err := sessionFacade.GetValidByTokenHash(ctx, tokenHash)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	if session.IsRevoked {
		return nil, ErrSessionRevoked
	}

	if session.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	// Update last accessed time (async to avoid blocking)
	go func() {
		if err := sessionFacade.UpdateLastAccessed(context.Background(), session.ID); err != nil {
			log.Warnf("Failed to update last accessed for session %s: %v", session.ID, err)
		}
	}()

	// Get user info for additional details
	userFacade := cpdb.GetFacade().GetUser()
	user, err := userFacade.GetByID(ctx, session.UserID)
	var username, email string
	var isAdmin bool
	if err == nil && user != nil {
		username = user.Username
		email = user.Email
		isAdmin = user.IsAdmin
	}

	return &SessionInfo{
		SessionID:    session.ID,
		UserID:       session.UserID,
		Username:     username,
		Email:        email,
		IsAdmin:      isAdmin,
		SyncSource:   session.SyncSource,
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    session.ExpiresAt,
		LastAccessed: session.LastAccessed,
	}, nil
}

// Refresh extends a session's expiration
func (m *DBManager) Refresh(ctx context.Context, token string) (*SessionInfo, error) {
	tokenHash := hashToken(token)

	sessionFacade := cpdb.GetFacade().GetSession()
	session, err := sessionFacade.GetValidByTokenHash(ctx, tokenHash)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	if session.IsRevoked {
		return nil, ErrSessionRevoked
	}

	// Check if refresh is needed (within threshold)
	timeUntilExpiry := time.Until(session.ExpiresAt)
	if timeUntilExpiry > m.config.RefreshThreshold {
		// No need to refresh yet
		return &SessionInfo{
			SessionID:    session.ID,
			UserID:       session.UserID,
			SyncSource:   session.SyncSource,
			CreatedAt:    session.CreatedAt,
			ExpiresAt:    session.ExpiresAt,
			LastAccessed: session.LastAccessed,
		}, nil
	}

	// Extend session
	newExpiry := time.Now().Add(m.config.DefaultTTL)
	if err := sessionFacade.UpdateExpiresAt(ctx, session.ID, newExpiry); err != nil {
		return nil, fmt.Errorf("failed to refresh session: %w", err)
	}

	log.Debugf("Refreshed session %s, new expiry: %v", session.ID, newExpiry)

	return &SessionInfo{
		SessionID:    session.ID,
		UserID:       session.UserID,
		SyncSource:   session.SyncSource,
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    newExpiry,
		LastAccessed: time.Now(),
	}, nil
}

// Revoke invalidates a session by token
func (m *DBManager) Revoke(ctx context.Context, token string, reason string) error {
	tokenHash := hashToken(token)

	sessionFacade := cpdb.GetFacade().GetSession()
	session, err := sessionFacade.GetValidByTokenHash(ctx, tokenHash)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrInvalidSession
		}
		return fmt.Errorf("failed to query session: %w", err)
	}

	if err := sessionFacade.Revoke(ctx, session.ID, reason); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	log.Infof("Revoked session %s, reason: %s", session.ID, reason)
	return nil
}

// RevokeByID invalidates a session by its ID
func (m *DBManager) RevokeByID(ctx context.Context, sessionID string, reason string) error {
	sessionFacade := cpdb.GetFacade().GetSession()
	if err := sessionFacade.Revoke(ctx, sessionID, reason); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	log.Infof("Revoked session %s by ID, reason: %s", sessionID, reason)
	return nil
}

// RevokeAllForUser invalidates all sessions for a user
func (m *DBManager) RevokeAllForUser(ctx context.Context, userID string, reason string) error {
	sessionFacade := cpdb.GetFacade().GetSession()
	if err := sessionFacade.RevokeAllForUser(ctx, userID, reason); err != nil {
		return fmt.Errorf("failed to revoke all sessions for user: %w", err)
	}

	log.Infof("Revoked all sessions for user %s, reason: %s", userID, reason)
	return nil
}

// GetUserSessions returns all active sessions for a user
func (m *DBManager) GetUserSessions(ctx context.Context, userID string) ([]*SessionInfo, error) {
	sessionFacade := cpdb.GetFacade().GetSession()
	sessions, err := sessionFacade.GetUserSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}

	result := make([]*SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, &SessionInfo{
			SessionID:    s.ID,
			UserID:       s.UserID,
			SyncSource:   s.SyncSource,
			CreatedAt:    s.CreatedAt,
			ExpiresAt:    s.ExpiresAt,
			LastAccessed: s.LastAccessed,
		})
	}

	return result, nil
}

// CleanupExpired removes expired and old revoked sessions
func (m *DBManager) CleanupExpired(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(m.config.CleanupRetentionDays) * 24 * time.Hour)

	sessionFacade := cpdb.GetFacade().GetSession()
	count, err := sessionFacade.CleanupExpired(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	if count > 0 {
		log.Infof("Cleaned up %d expired/revoked sessions", count)
	}

	return count, nil
}

// Helper functions

func generateToken() (string, error) {
	bytes := make([]byte, TokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return SessionTokenPrefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("sess-%s", hex.EncodeToString(bytes))
}

// Ensure DBManager implements Manager interface
var _ Manager = (*DBManager)(nil)
