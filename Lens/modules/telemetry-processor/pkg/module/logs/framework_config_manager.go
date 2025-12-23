package logs

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/sirupsen/logrus"
	
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
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
	
	logrus.Infof("Loaded framework config: %s (version: %s)", frameworkName, patterns.Version)
	return &patterns, nil
}

// LoadAllFrameworks loads all framework configurations
func (m *FrameworkConfigManager) LoadAllFrameworks(ctx context.Context) error {
	// Get list of framework names from config
	// For now, try loading known frameworks
	knownFrameworks := []string{"primus", "deepspeed", "megatron"}
	
	for _, name := range knownFrameworks {
		if _, err := m.LoadFrameworkConfig(ctx, name); err != nil {
			logrus.Warnf("Failed to load framework %s: %v", name, err)
			// Continue loading other frameworks
		}
	}
	
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

