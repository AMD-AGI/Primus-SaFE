// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package detection

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// ConfigKeyPrefix for framework log parser configurations
	ConfigKeyPrefix = "training.log.parser.framework"

	// Default cache TTL
	DefaultCacheTTL = 5 * time.Minute
)

// FrameworkConfigManager manages framework log parsing configurations
type FrameworkConfigManager struct {
	configManager  *config.Manager
	frameworkCache map[string]*FrameworkLogPatterns
	cacheTTL       time.Duration
	lastRefresh    time.Time
	mu             sync.RWMutex
}

// NewFrameworkConfigManager creates a new framework config manager
func NewFrameworkConfigManager(configMgr *config.Manager) *FrameworkConfigManager {
	return &FrameworkConfigManager{
		configManager:  configMgr,
		frameworkCache: make(map[string]*FrameworkLogPatterns),
		cacheTTL:       DefaultCacheTTL,
	}
}

// LoadFrameworkConfig loads configuration for a specific framework
func (m *FrameworkConfigManager) LoadFrameworkConfig(
	ctx context.Context,
	frameworkName string,
) (*FrameworkLogPatterns, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Check cache first
	if cached, ok := m.frameworkCache[frameworkName]; ok {
		if time.Since(m.lastRefresh) < m.cacheTTL {
			return cached, nil
		}
	}
	
	// Load from system_config
	configKey := fmt.Sprintf("%s.%s", ConfigKeyPrefix, frameworkName)
	var patterns FrameworkLogPatterns
	err := m.configManager.Get(ctx, configKey, &patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to load config for %s: %w", frameworkName, err)
	}
	
	// Validate
	if err := patterns.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config for %s: %w", frameworkName, err)
	}
	
	// Cache it
	m.frameworkCache[frameworkName] = &patterns
	m.lastRefresh = time.Now()
	
	log.Infof("Loaded framework config: %s (version: %s)", frameworkName, patterns.Version)
	return &patterns, nil
}

// LoadAllFrameworks loads all framework configurations dynamically from system_config
// It discovers all configs with prefix "training.log.parser.framework." and loads them
func (m *FrameworkConfigManager) LoadAllFrameworks(ctx context.Context) error {
	// Discover all framework configs from system_config by prefix
	frameworkNames, err := m.discoverFrameworkConfigs(ctx)
	if err != nil {
		log.Warnf("Failed to discover framework configs: %v, using empty list", err)
		frameworkNames = []string{}
	}

	log.Infof("Discovered %d framework configurations: %v", len(frameworkNames), frameworkNames)

	for _, name := range frameworkNames {
		if _, err := m.LoadFrameworkConfig(ctx, name); err != nil {
			log.Warnf("Failed to load framework %s: %v", name, err)
			// Continue loading other frameworks
		}
	}

	return nil
}

// discoverFrameworkConfigs discovers all framework config keys from system_config
func (m *FrameworkConfigManager) discoverFrameworkConfigs(ctx context.Context) ([]string, error) {
	// List all configs with the framework prefix
	configs, err := m.configManager.List(ctx, config.WithKeyPrefixFilter(ConfigKeyPrefix+"."))
	if err != nil {
		return nil, fmt.Errorf("failed to list framework configs: %w", err)
	}

	// Extract framework names from keys
	var frameworkNames []string
	prefix := ConfigKeyPrefix + "."
	for _, cfg := range configs {
		if strings.HasPrefix(cfg.Key, prefix) {
			name := strings.TrimPrefix(cfg.Key, prefix)
			// Skip if name contains more dots (sub-configs)
			if !strings.Contains(name, ".") && name != "" {
				frameworkNames = append(frameworkNames, name)
			}
		}
	}

	return frameworkNames, nil
}

// LoadTrainingFrameworks loads only training framework configurations
func (m *FrameworkConfigManager) LoadTrainingFrameworks(ctx context.Context) error {
	// First load all frameworks, then filter by type
	if err := m.LoadAllFrameworks(ctx); err != nil {
		return err
	}
	// The cache now contains all frameworks, GetTrainingFrameworks will filter them
	return nil
}

// LoadInferenceFrameworks loads only inference framework configurations
func (m *FrameworkConfigManager) LoadInferenceFrameworks(ctx context.Context) error {
	// First load all frameworks, then filter by type
	if err := m.LoadAllFrameworks(ctx); err != nil {
		return err
	}
	// The cache now contains all frameworks, GetInferenceFrameworks will filter them
	return nil
}

// GetFramework retrieves cached framework configuration
func (m *FrameworkConfigManager) GetFramework(frameworkName string) *FrameworkLogPatterns {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return m.frameworkCache[frameworkName]
}

// ListFrameworks returns all loaded framework names
func (m *FrameworkConfigManager) ListFrameworks() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.frameworkCache))
	for name, patterns := range m.frameworkCache {
		if patterns.Enabled {
			names = append(names, name)
		}
	}

	return names
}

// ListTrainingFrameworks returns all loaded training framework names
func (m *FrameworkConfigManager) ListTrainingFrameworks() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0)
	for name, patterns := range m.frameworkCache {
		if patterns.Enabled && patterns.IsTraining() {
			names = append(names, name)
		}
	}

	return names
}

// ListInferenceFrameworks returns all loaded inference framework names
func (m *FrameworkConfigManager) ListInferenceFrameworks() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0)
	for name, patterns := range m.frameworkCache {
		if patterns.Enabled && patterns.IsInference() {
			names = append(names, name)
		}
	}

	return names
}

// GetTrainingFrameworks returns all enabled training framework configs sorted by priority
func (m *FrameworkConfigManager) GetTrainingFrameworks() []*FrameworkLogPatterns {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var frameworks []*FrameworkLogPatterns
	for _, patterns := range m.frameworkCache {
		if patterns.Enabled && patterns.IsTraining() {
			frameworks = append(frameworks, patterns)
		}
	}

	// Sort by priority (higher priority first)
	sort.Slice(frameworks, func(i, j int) bool {
		return frameworks[i].Priority > frameworks[j].Priority
	})

	return frameworks
}

// GetInferenceFrameworks returns all enabled inference framework configs sorted by priority
func (m *FrameworkConfigManager) GetInferenceFrameworks() []*FrameworkLogPatterns {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var frameworks []*FrameworkLogPatterns
	for _, patterns := range m.frameworkCache {
		if patterns.Enabled && patterns.IsInference() {
			frameworks = append(frameworks, patterns)
		}
	}

	// Sort by priority (higher priority first)
	sort.Slice(frameworks, func(i, j int) bool {
		return frameworks[i].Priority > frameworks[j].Priority
	})

	return frameworks
}

// GetFrameworksByType returns all enabled frameworks of a specific type sorted by priority
func (m *FrameworkConfigManager) GetFrameworksByType(frameworkType string) []*FrameworkLogPatterns {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var frameworks []*FrameworkLogPatterns
	for _, patterns := range m.frameworkCache {
		if patterns.Enabled && patterns.GetType() == frameworkType {
			frameworks = append(frameworks, patterns)
		}
	}

	// Sort by priority (higher priority first)
	sort.Slice(frameworks, func(i, j int) bool {
		return frameworks[i].Priority > frameworks[j].Priority
	})

	return frameworks
}

// RefreshCache forces a cache refresh
func (m *FrameworkConfigManager) RefreshCache(ctx context.Context) error {
	m.mu.Lock()
	m.frameworkCache = make(map[string]*FrameworkLogPatterns)
	m.mu.Unlock()
	
	return m.LoadAllFrameworks(ctx)
}

// SetCacheTTL sets the cache TTL duration
func (m *FrameworkConfigManager) SetCacheTTL(ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheTTL = ttl
}

// IsExpired checks if cache is expired
func (m *FrameworkConfigManager) IsExpired() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Since(m.lastRefresh) >= m.cacheTTL
}

