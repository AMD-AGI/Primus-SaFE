/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"fmt"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// TokenRefresher manages the background process for refreshing user tokens using OAuth2 refresh tokens.
type TokenRefresher struct {
	dbClient   dbclient.Interface
	ssoToken   *ssoToken
	interval   time.Duration // How often to check for tokens to refresh
	threshold  time.Duration // How close to expiration to trigger a refresh
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewTokenRefresher creates a new TokenRefresher instance.
func NewTokenRefresher(ctx context.Context, dbClient dbclient.Interface, ssoToken *ssoToken) *TokenRefresher {
	refreshInterval := time.Duration(commonconfig.GetTokenRefreshInterval()) * time.Second
	refreshThreshold := time.Duration(commonconfig.GetTokenRefreshThreshold()) * time.Second

	if refreshInterval <= 0 {
		refreshInterval = 20 * time.Minute // Default to 20 minutes
	}
	if refreshThreshold <= 0 {
		refreshThreshold = 30 * time.Minute // Default to 30 minutes
	}

	refresherCtx, cancel := context.WithCancel(ctx)
	return &TokenRefresher{
		dbClient:   dbClient,
		ssoToken:   ssoToken,
		interval:   refreshInterval,
		threshold:  refreshThreshold,
		ctx:        refresherCtx,
		cancelFunc: cancel,
	}
}

// Start begins the periodic token refreshing process.
func (r *TokenRefresher) Start() {
	if !commonconfig.IsTokenRefreshEnable() {
		klog.Info("Token refresher is disabled.")
		return
	}
	if r.dbClient == nil {
		klog.Warning("Token refresher cannot start: database client is nil.")
		return
	}
	if r.ssoToken == nil {
		klog.Warning("Token refresher cannot start: SSO token handler is nil.")
		return
	}

	klog.Infof("OAuth2 Token refresher started with interval: %v, threshold: %v", r.interval, r.threshold)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			klog.Info("Token refresher stopped.")
			return
		case <-ticker.C:
			r.refreshTokens()
		}
	}
}

// Stop gracefully stops the token refreshing process.
func (r *TokenRefresher) Stop() {
	r.cancelFunc()
}

// refreshTokens checks for tokens nearing expiration and attempts to refresh them using OAuth2.
func (r *TokenRefresher) refreshTokens() {
	klog.V(4).Info("Checking for tokens to refresh...")
	dbTags := dbclient.GetUserTokenFieldTags()
	now := time.Now().Unix()

	// Select tokens that are not expired yet, but will expire within the threshold, and have a refresh token.
	query := sqrl.And{
		sqrl.Gt{dbclient.GetFieldTag(dbTags, "ExpireTime"): now},
		sqrl.Lt{dbclient.GetFieldTag(dbTags, "ExpireTime"): now + int64(r.threshold.Seconds())},
		sqrl.Expr(fmt.Sprintf("%s IS NOT NULL AND %s != ''", dbclient.GetFieldTag(dbTags, "RefreshToken"), dbclient.GetFieldTag(dbTags, "RefreshToken"))),
	}

	userTokens, err := r.dbClient.SelectUserTokens(r.ctx, query, nil, 0, 0) // Fetch all matching tokens
	if err != nil {
		klog.ErrorS(err, "Failed to select user tokens for refresh")
		return
	}

	if len(userTokens) == 0 {
		klog.V(4).Info("No tokens found needing refresh.")
		return
	}

	klog.Infof("Found %d tokens to refresh using OAuth2", len(userTokens))

	refreshedCount := 0
	failedCount := 0
	for _, token := range userTokens {
		klog.V(4).Infof("Attempting to refresh token for user %s (session: %s)", token.UserId, token.SessionId)
		updatedToken, err := r.ssoToken.RefreshWithOAuth2(r.ctx, token)
		if err != nil {
			klog.ErrorS(err, "Failed to refresh token for user", "userId", token.UserId, "sessionId", token.SessionId)
			failedCount++
			continue
		}
		klog.Infof("Successfully refreshed token for user %s (session: %s), new expire time: %v", updatedToken.UserId, updatedToken.SessionId, time.Unix(updatedToken.ExpireTime, 0))
		refreshedCount++
	}
	klog.Infof("OAuth2 token refresh completed: %d refreshed, %d failed", refreshedCount, failedCount)
}
