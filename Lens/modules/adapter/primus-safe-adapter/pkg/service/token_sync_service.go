// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	cpmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	safedal "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	safemodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

const (
	// SyncSourceSafe is the sync source value for tokens synced from SaFE
	SyncSourceSafe = "safe"
	// SyncSourceLocal is the sync source value for locally created sessions
	SyncSourceLocal = "local"
)

// TokenSyncService syncs tokens from SaFE to Lens Control Plane
type TokenSyncService struct {
	safeDB *gorm.DB // SaFE database (read-only)
	lensDB *gorm.DB // Lens Control Plane database (read-write)
}

// NewTokenSyncService creates a new token sync service
func NewTokenSyncService(safeDB, lensDB *gorm.DB) *TokenSyncService {
	return &TokenSyncService{
		safeDB: safeDB,
		lensDB: lensDB,
	}
}

// Name returns the task name
func (s *TokenSyncService) Name() string {
	return "token-sync"
}

// Run executes the token sync task
func (s *TokenSyncService) Run(ctx context.Context) error {
	log.Debug("Starting token sync")

	// 1. Get active tokens from SaFE database
	safeTokens, err := s.getSafeTokens(ctx)
	if err != nil {
		log.Errorf("Failed to get SaFE tokens: %v", err)
		return err
	}

	if len(safeTokens) == 0 {
		log.Debug("No active tokens found in SaFE")
		return nil
	}

	// 2. Sync each token to Lens Control Plane
	syncedCount := 0
	for _, token := range safeTokens {
		if err := s.syncToken(ctx, token); err != nil {
			log.Errorf("Failed to sync token for user %s: %v", token.UserID, err)
			continue
		}
		syncedCount++
	}

	// 3. Cleanup deleted tokens
	deletedCount, err := s.cleanupDeletedTokens(ctx, safeTokens)
	if err != nil {
		log.Errorf("Failed to cleanup deleted tokens: %v", err)
	}

	log.Infof("Token sync completed: synced=%d, deleted=%d", syncedCount, deletedCount)
	return nil
}

// getSafeTokens retrieves active tokens from SaFE database
func (s *TokenSyncService) getSafeTokens(ctx context.Context) ([]*safemodel.UserToken, error) {
	dal := safedal.Use(s.safeDB)
	ut := dal.UserToken

	now := time.Now().Unix()
	// Get tokens that are not expired (expire_time > now or expire_time < 0 for never-expire)
	tokens, err := ut.WithContext(ctx).
		Where(ut.ExpireTime.Gt(now)).
		Or(ut.ExpireTime.Lt(0)).
		Find()
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// syncToken syncs a single token from SaFE to Lens
func (s *TokenSyncService) syncToken(ctx context.Context, safeToken *safemodel.UserToken) error {
	// Check if session already exists in Lens
	lensSessionFacade := cpdb.GetFacade().GetSession()

	existing, err := lensSessionFacade.GetBySafeSessionID(ctx, safeToken.SessionID)
	if err != nil {
		// Only return error if it's not a "not found" situation
		// Note: GORM callback may convert ErrRecordNotFound to nil
		return err
	}

	expiresAt := time.Unix(safeToken.ExpireTime, 0)
	if safeToken.ExpireTime < 0 {
		expiresAt = time.Now().Add(365 * 24 * time.Hour) // 1 year for never-expire
	}

	// Check if session was actually found (handle GORM callback issue)
	if existing != nil && existing.ID != "" {
		// Update expiration if changed
		if !existing.ExpiresAt.Equal(expiresAt) {
			return lensSessionFacade.UpdateExpiresAt(ctx, existing.ID, expiresAt)
		}
		return nil
	}

	// Create new session in Lens Control Plane
	session := &cpmodel.LensSessions{
		ID:            generateSessionID(),
		UserID:        safeToken.UserID,
		TokenHash:     hashToken(safeToken.SessionID),
		SafeSessionID: safeToken.SessionID,
		SyncSource:    SyncSourceSafe,
		CreatedAt:     time.Unix(safeToken.CreationTime, 0),
		ExpiresAt:     expiresAt,
		LastAccessed:  time.Now(),
		IsRevoked:     false,
	}

	return lensSessionFacade.Create(ctx, session)
}

// cleanupDeletedTokens removes sessions for tokens that no longer exist in SaFE
func (s *TokenSyncService) cleanupDeletedTokens(ctx context.Context, activeTokens []*safemodel.UserToken) (int64, error) {
	// Build set of active session IDs
	activeSet := make(map[string]bool)
	for _, t := range activeTokens {
		activeSet[t.SessionID] = true
	}

	// Get all synced sessions from Lens
	lensSessionFacade := cpdb.GetFacade().GetSession()
	syncedSessions, err := lensSessionFacade.ListBySyncSource(ctx, SyncSourceSafe)
	if err != nil {
		return 0, err
	}

	// Revoke sessions that are no longer in SaFE
	var deletedCount int64
	for _, session := range syncedSessions {
		if session.SafeSessionID != "" && !activeSet[session.SafeSessionID] && !session.IsRevoked {
			if err := lensSessionFacade.Revoke(ctx, session.ID, "Token deleted in SaFE"); err != nil {
				log.Errorf("Failed to revoke session %s: %v", session.ID, err)
				continue
			}
			deletedCount++
		}
	}

	return deletedCount, nil
}

func generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("sess-%s", hex.EncodeToString(bytes))
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
