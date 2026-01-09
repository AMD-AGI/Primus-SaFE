// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"time"

	"gorm.io/gorm"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// DefaultCleanupRetentionDays is the default number of days to retain expired/revoked sessions
	DefaultCleanupRetentionDays = 7
)

// TokenCleanupService cleans up expired and revoked sessions from Lens Control Plane
type TokenCleanupService struct {
	lensDB        *gorm.DB
	retentionDays int
}

// NewTokenCleanupService creates a new token cleanup service
func NewTokenCleanupService(lensDB *gorm.DB) *TokenCleanupService {
	return &TokenCleanupService{
		lensDB:        lensDB,
		retentionDays: DefaultCleanupRetentionDays,
	}
}

// NewTokenCleanupServiceWithRetention creates a new token cleanup service with custom retention period
func NewTokenCleanupServiceWithRetention(lensDB *gorm.DB, retentionDays int) *TokenCleanupService {
	if retentionDays <= 0 {
		retentionDays = DefaultCleanupRetentionDays
	}
	return &TokenCleanupService{
		lensDB:        lensDB,
		retentionDays: retentionDays,
	}
}

// Name returns the task name
func (s *TokenCleanupService) Name() string {
	return "token-cleanup"
}

// Run executes the token cleanup task
func (s *TokenCleanupService) Run(ctx context.Context) error {
	log.Debug("Starting token cleanup")

	// Calculate the cutoff time
	cutoff := time.Now().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)

	// Cleanup expired sessions
	lensSessionFacade := cpdb.GetFacade().GetSession()
	deletedCount, err := lensSessionFacade.CleanupExpired(ctx, cutoff)
	if err != nil {
		log.Errorf("Failed to cleanup expired sessions: %v", err)
		return err
	}

	if deletedCount > 0 {
		log.Infof("Token cleanup completed: deleted %d expired/revoked sessions older than %d days",
			deletedCount, s.retentionDays)
	} else {
		log.Debug("Token cleanup completed: no sessions to delete")
	}

	return nil
}
