// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package safe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	// RoleSystemAdmin is the system admin role in SaFE
	RoleSystemAdmin = "system-admin"
)

// UserInfo represents user information returned by the SaFE API
type UserInfo struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

// IsAdmin returns true if the user has the system-admin role
func (u *UserInfo) IsAdmin() bool {
	for _, r := range u.Roles {
		if r == RoleSystemAdmin {
			return true
		}
	}
	return false
}

// UserClient queries user information from the SaFE API server
type UserClient struct {
	baseURL    string
	httpClient *http.Client

	// Simple in-memory cache to avoid querying SaFE API on every request
	mu    sync.RWMutex
	cache map[string]*cacheEntry
}

type cacheEntry struct {
	info      *UserInfo
	expiresAt time.Time
}

const cacheTTL = 5 * time.Minute

// NewUserClient creates a new SaFE user client.
// Returns nil if baseURL is empty (feature disabled).
func NewUserClient(baseURL string) *UserClient {
	if baseURL == "" {
		return nil
	}
	return &UserClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: make(map[string]*cacheEntry),
	}
}

// GetUserInfo retrieves user information from the SaFE API.
// Results are cached for 5 minutes to reduce API calls.
func (c *UserClient) GetUserInfo(ctx context.Context, userID string) (*UserInfo, error) {
	if c == nil || userID == "" {
		return nil, nil
	}

	// Check cache
	c.mu.RLock()
	if entry, ok := c.cache[userID]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.info, nil
	}
	c.mu.RUnlock()

	// Query SaFE API: GET /api/v1/users/{userId}
	// Pass userId header for internal service authentication
	url := fmt.Sprintf("%s/api/v1/users/%s", c.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("userId", userID)
	req.Header.Set("User-Agent", "skills-repository/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[SafeAPI] failed to query user %s: %v", userID, err)
		return nil, fmt.Errorf("failed to query SaFE API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		log.Printf("[SafeAPI] user query failed: HTTP %d, body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("SaFE API returned HTTP %d", resp.StatusCode)
	}

	// Parse response: {"id": "...", "name": "...", "roles": ["system-admin", ...], ...}
	// SaFE API returns the user object directly (no wrapper)
	info := &UserInfo{}
	if err := json.NewDecoder(resp.Body).Decode(info); err != nil {
		return nil, fmt.Errorf("failed to decode SaFE API response: %w", err)
	}

	if info.ID == "" {
		info.ID = userID
	}

	log.Printf("[SafeAPI] user=%s name=%s roles=%v isAdmin=%v", info.ID, info.Name, info.Roles, info.IsAdmin())

	// Update cache
	c.mu.Lock()
	c.cache[userID] = &cacheEntry{
		info:      info,
		expiresAt: time.Now().Add(cacheTTL),
	}
	c.mu.Unlock()

	return info, nil
}
