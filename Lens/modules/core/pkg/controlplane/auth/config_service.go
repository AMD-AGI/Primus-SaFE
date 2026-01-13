// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"sync"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// SafeConfig holds Safe mode configuration
type SafeConfig struct {
	AdapterURL   string `json:"adapter_url"`
	LoginURL     string `json:"login_url"`
	CallbackPath string `json:"callback_path"`
}

// AuthConfig holds the full authentication configuration
type AuthConfig struct {
	Mode        AuthMode    `json:"mode"`
	Initialized bool        `json:"initialized"`
	Safe        *SafeConfig `json:"safe,omitempty"`
}

// cacheEntry represents a cached value with expiration
type cacheEntry struct {
	value      interface{}
	expiration time.Time
}

// AuthConfigService provides authentication configuration from database
type AuthConfigService struct {
	facade   cpdb.FacadeInterface
	cache    map[string]*cacheEntry
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
}

var (
	globalConfigService *AuthConfigService
	configServiceOnce   sync.Once
)

// GetAuthConfigService returns the global AuthConfigService instance
func GetAuthConfigService() *AuthConfigService {
	configServiceOnce.Do(func() {
		globalConfigService = NewAuthConfigService(cpdb.GetFacade())
	})
	return globalConfigService
}

// NewAuthConfigService creates a new AuthConfigService
func NewAuthConfigService(facade cpdb.FacadeInterface) *AuthConfigService {
	return &AuthConfigService{
		facade:   facade,
		cache:    make(map[string]*cacheEntry),
		cacheTTL: 30 * time.Second,
	}
}

// GetAuthMode returns the current authentication mode
func (s *AuthConfigService) GetAuthMode(ctx context.Context) (AuthMode, error) {
	// Check cache first
	if cached := s.getFromCache(ConfigKeyAuthMode); cached != nil {
		return cached.(AuthMode), nil
	}

	// Query database
	value, err := s.getConfigString(ctx, ConfigKeyAuthMode)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AuthModeNone, nil
		}
		log.Debugf("Failed to get auth mode from DB: %v", err)
		return AuthModeNone, nil
	}

	mode := AuthMode(value)
	s.setToCache(ConfigKeyAuthMode, mode)
	return mode, nil
}

// IsInitialized checks if authentication is initialized
func (s *AuthConfigService) IsInitialized(ctx context.Context) (bool, error) {
	cacheKey := ConfigKeyAuthInitialized
	if cached := s.getFromCache(cacheKey); cached != nil {
		return cached.(bool), nil
	}

	value, err := s.getConfigBool(ctx, ConfigKeyAuthInitialized)
	if err != nil {
		return false, nil
	}

	s.setToCache(cacheKey, value)
	return value, nil
}

// GetSafeConfig returns Safe mode configuration
func (s *AuthConfigService) GetSafeConfig(ctx context.Context) (*SafeConfig, error) {
	cacheKey := "safe.config"
	if cached := s.getFromCache(cacheKey); cached != nil {
		return cached.(*SafeConfig), nil
	}

	adapterURL, _ := s.getConfigString(ctx, ConfigKeySafeAdapterURL)
	loginURL, _ := s.getConfigString(ctx, ConfigKeySafeLoginURL)
	callbackPath, _ := s.getConfigString(ctx, ConfigKeySafeCallbackPath)

	// Use defaults if not configured
	if callbackPath == "" {
		callbackPath = "/lens/sso-bridge"
	}

	config := &SafeConfig{
		AdapterURL:   adapterURL,
		LoginURL:     loginURL,
		CallbackPath: callbackPath,
	}

	s.setToCache(cacheKey, config)
	return config, nil
}

// GetFullConfig returns the full authentication configuration
func (s *AuthConfigService) GetFullConfig(ctx context.Context) (*AuthConfig, error) {
	mode, err := s.GetAuthMode(ctx)
	if err != nil {
		return nil, err
	}

	initialized, _ := s.IsInitialized(ctx)

	config := &AuthConfig{
		Mode:        mode,
		Initialized: initialized,
	}

	// Include Safe config if in Safe mode
	if mode == AuthModeSaFE {
		safeConfig, _ := s.GetSafeConfig(ctx)
		config.Safe = safeConfig
	}

	return config, nil
}

// InvalidateCache clears all cached values
func (s *AuthConfigService) InvalidateCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = make(map[string]*cacheEntry)
	log.Debug("Auth config cache invalidated")
}

// InvalidateCacheKey clears a specific cached value
func (s *AuthConfigService) InvalidateCacheKey(key string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	delete(s.cache, key)
}

// getConfigString gets a string value from system config
func (s *AuthConfigService) getConfigString(ctx context.Context, key string) (string, error) {
	config, err := s.facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		return "", err
	}

	if val, ok := config.Value["value"]; ok {
		if str, ok := val.(string); ok {
			return str, nil
		}
	}

	return "", nil
}

// getConfigBool gets a boolean value from system config
func (s *AuthConfigService) getConfigBool(ctx context.Context, key string) (bool, error) {
	config, err := s.facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		return false, err
	}

	if val, ok := config.Value["value"]; ok {
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			return v == "true", nil
		}
	}

	return false, nil
}

// getFromCache retrieves a value from cache if not expired
func (s *AuthConfigService) getFromCache(key string) interface{} {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, ok := s.cache[key]
	if !ok {
		return nil
	}

	if time.Now().After(entry.expiration) {
		return nil
	}

	return entry.value
}

// setToCache stores a value in cache with TTL
func (s *AuthConfigService) setToCache(key string, value interface{}) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cache[key] = &cacheEntry{
		value:      value,
		expiration: time.Now().Add(s.cacheTTL),
	}
}
