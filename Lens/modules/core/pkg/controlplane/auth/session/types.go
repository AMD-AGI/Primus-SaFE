// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package session

import (
	"context"
	"errors"
	"time"
)

// Common errors
var (
	ErrInvalidSession = errors.New("invalid or expired session")
	ErrSessionRevoked = errors.New("session has been revoked")
	ErrTokenExpired   = errors.New("token has expired")
)

// CreateOptions contains options for creating a new session
type CreateOptions struct {
	UserID    string
	Username  string
	Email     string
	UserAgent string
	IPAddress string
	Duration  time.Duration
	// SyncSource indicates where the session comes from ("local", "safe", "ldap")
	SyncSource string
	// SafeSessionID is the original session ID from SaFE (if synced from SaFE)
	SafeSessionID string
}

// SessionInfo contains session information
type SessionInfo struct {
	SessionID    string
	Token        string // Only populated on creation
	UserID       string
	Username     string
	DisplayName  string
	Email        string
	IsAdmin      bool
	SyncSource   string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastAccessed time.Time
}

// Manager defines the interface for session management
type Manager interface {
	// Create creates a new session and returns the session info with token
	Create(ctx context.Context, opts *CreateOptions) (*SessionInfo, error)

	// Validate validates a session token and returns session info
	Validate(ctx context.Context, token string) (*SessionInfo, error)

	// Refresh extends a session's expiration and returns updated session info
	Refresh(ctx context.Context, token string) (*SessionInfo, error)

	// Revoke invalidates a session
	Revoke(ctx context.Context, token string, reason string) error

	// RevokeByID invalidates a session by its ID
	RevokeByID(ctx context.Context, sessionID string, reason string) error

	// RevokeAllForUser invalidates all sessions for a user
	RevokeAllForUser(ctx context.Context, userID string, reason string) error

	// GetUserSessions returns all active sessions for a user
	GetUserSessions(ctx context.Context, userID string) ([]*SessionInfo, error)

	// CleanupExpired removes expired and old revoked sessions
	CleanupExpired(ctx context.Context) (int64, error)
}

// Config contains session manager configuration
type Config struct {
	// DefaultTTL is the default session TTL
	DefaultTTL time.Duration
	// RefreshThreshold is the time before expiration when refresh is allowed
	RefreshThreshold time.Duration
	// CleanupRetentionDays is how long to keep expired/revoked sessions
	CleanupRetentionDays int
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		DefaultTTL:           24 * time.Hour,
		RefreshThreshold:     30 * time.Minute,
		CleanupRetentionDays: 7,
	}
}
