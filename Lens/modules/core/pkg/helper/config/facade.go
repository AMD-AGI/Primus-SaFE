// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"gorm.io/gorm"
)

// Global singleton manager map, stored by cluster name
var (
	// managers stores singleton instances of regular configuration managers
	managers = make(map[string]*Manager)
	// cachedManagers stores singleton instances of cached configuration managers
	cachedManagers = make(map[string]*CachedManager)
	// managerMutex protects concurrent access to managers map
	managerMutex sync.RWMutex
	// cachedManagerMutex protects concurrent access to cachedManagers map
	cachedManagerMutex sync.RWMutex
	// defaultCacheTTL is the default cache TTL
	defaultCacheTTL = 5 * time.Minute
)

// GetOrInitConfigManager gets or initializes the configuration manager for a specified cluster (singleton)
// clusterName: cluster name, empty string indicates default cluster
// Returns the singleton configuration manager for the cluster
func GetOrInitConfigManager(clusterName string) *Manager {
	// Try to read first
	managerMutex.RLock()
	if mgr, exists := managers[clusterName]; exists {
		managerMutex.RUnlock()
		return mgr
	}
	managerMutex.RUnlock()

	// Need to create a new manager
	managerMutex.Lock()
	defer managerMutex.Unlock()

	// Double-check to avoid duplicate creation
	if mgr, exists := managers[clusterName]; exists {
		return mgr
	}

	// Create new manager
	var mgr *Manager
	if clusterName == "" {
		// Use default database
		facade := database.GetFacade()
		mgr = &Manager{
			db: facade.GetSystemConfig().GetDB(),
		}
	} else {
		// Use database for specified cluster
		mgr = NewManagerForCluster(clusterName)
	}

	managers[clusterName] = mgr
	return mgr
}

// GetOrInitCachedConfigManager gets or initializes the cached configuration manager for a specified cluster (singleton)
// clusterName: cluster name, empty string indicates default cluster
// cacheTTL: cache TTL, if 0 uses default value (5 minutes)
// Returns the singleton cached configuration manager for the cluster
func GetOrInitCachedConfigManager(clusterName string, cacheTTL time.Duration) *CachedManager {
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}

	// Try to read first
	cachedManagerMutex.RLock()
	if mgr, exists := cachedManagers[clusterName]; exists {
		cachedManagerMutex.RUnlock()
		return mgr
	}
	cachedManagerMutex.RUnlock()

	// Need to create a new manager
	cachedManagerMutex.Lock()
	defer cachedManagerMutex.Unlock()

	// Double-check to avoid duplicate creation
	if mgr, exists := cachedManagers[clusterName]; exists {
		return mgr
	}

	// Create new cached manager
	baseManager := GetOrInitConfigManager(clusterName)
	cachedMgr := NewCachedManager(baseManager, cacheTTL)

	cachedManagers[clusterName] = cachedMgr
	return cachedMgr
}

// GetDefaultConfigManager gets the configuration manager for the default cluster (singleton)
// Equivalent to GetOrInitConfigManager("")
func GetDefaultConfigManager() *Manager {
	return GetOrInitConfigManager("")
}

// GetDefaultCachedConfigManager gets the cached configuration manager for the default cluster (singleton)
// Uses default cache TTL (5 minutes)
func GetDefaultCachedConfigManager() *CachedManager {
	return GetOrInitCachedConfigManager("", defaultCacheTTL)
}

// GetConfigManagerForCluster gets the configuration manager for a specified cluster (singleton)
// Equivalent to GetOrInitConfigManager(clusterName)
func GetConfigManagerForCluster(clusterName string) *Manager {
	return GetOrInitConfigManager(clusterName)
}

// GetCachedConfigManagerForCluster gets the cached configuration manager for a specified cluster (singleton)
// Uses default cache TTL (5 minutes)
func GetCachedConfigManagerForCluster(clusterName string) *CachedManager {
	return GetOrInitCachedConfigManager(clusterName, defaultCacheTTL)
}

// ResetConfigManager resets the configuration manager for a specified cluster
// Usually used for testing or scenarios requiring reinitialization
func ResetConfigManager(clusterName string) {
	managerMutex.Lock()
	delete(managers, clusterName)
	managerMutex.Unlock()

	cachedManagerMutex.Lock()
	delete(cachedManagers, clusterName)
	cachedManagerMutex.Unlock()
}

// ResetAllConfigManagers resets all configuration managers
// Usually used for testing scenarios
func ResetAllConfigManagers() {
	managerMutex.Lock()
	managers = make(map[string]*Manager)
	managerMutex.Unlock()

	cachedManagerMutex.Lock()
	cachedManagers = make(map[string]*CachedManager)
	cachedManagerMutex.Unlock()
}

// SetDefaultCacheTTL sets the default cache TTL
// Only affects subsequently created cache managers, does not affect existing ones
func SetDefaultCacheTTL(ttl time.Duration) {
	if ttl > 0 {
		defaultCacheTTL = ttl
	}
}

// GetDefaultCacheTTL gets the default cache TTL
func GetDefaultCacheTTL() time.Duration {
	return defaultCacheTTL
}

// ConfigManagerStats contains configuration manager statistics
type ConfigManagerStats struct {
	TotalManagers       int                    `json:"total_managers"`        // Total number of managers
	TotalCachedManagers int                    `json:"total_cached_managers"` // Total number of cached managers
	Clusters            []string               `json:"clusters"`              // List of clusters
	CachedClusters      []string               `json:"cached_clusters"`       // List of clusters using cache
	DefaultCacheTTL     string                 `json:"default_cache_ttl"`     // Default cache TTL
	CacheStats          map[string]interface{} `json:"cache_stats,omitempty"` // Cache statistics (if available)
}

// GetConfigManagerStats gets statistics for configuration managers
func GetConfigManagerStats() *ConfigManagerStats {
	stats := &ConfigManagerStats{
		DefaultCacheTTL: defaultCacheTTL.String(),
		CacheStats:      make(map[string]interface{}),
	}

	// Count regular managers
	managerMutex.RLock()
	stats.TotalManagers = len(managers)
	stats.Clusters = make([]string, 0, len(managers))
	for cluster := range managers {
		if cluster == "" {
			stats.Clusters = append(stats.Clusters, "<default>")
		} else {
			stats.Clusters = append(stats.Clusters, cluster)
		}
	}
	managerMutex.RUnlock()

	// Count cached managers
	cachedManagerMutex.RLock()
	stats.TotalCachedManagers = len(cachedManagers)
	stats.CachedClusters = make([]string, 0, len(cachedManagers))
	for cluster, cachedMgr := range cachedManagers {
		clusterName := cluster
		if cluster == "" {
			clusterName = "<default>"
		}
		stats.CachedClusters = append(stats.CachedClusters, clusterName)
		// Get cache statistics for each cluster
		stats.CacheStats[clusterName] = cachedMgr.GetCacheStats()
	}
	cachedManagerMutex.RUnlock()

	return stats
}

// WithDB creates a configuration manager using a specified database connection
// Does not use singleton pattern, returns new instance on each call
// db: GORM database connection
func WithDB(db *gorm.DB) *Manager {
	return NewManager(db)
}

// WithDBAndCache creates a cached configuration manager using a specified database connection
// Does not use singleton pattern, returns new instance on each call
// db: GORM database connection
// cacheTTL: cache TTL
func WithDBAndCache(db *gorm.DB, cacheTTL time.Duration) *CachedManager {
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}
	return NewCachedManager(NewManager(db), cacheTTL)
}
