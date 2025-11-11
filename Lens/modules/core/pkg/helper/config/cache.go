package config

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CachedManager is a configuration manager with caching
type CachedManager struct {
	*Manager
	cache       sync.Map // key -> cacheEntry
	cacheTTL    time.Duration
	autoRefresh bool
}

type cacheEntry struct {
	value      interface{}
	expireTime time.Time
}

// NewCachedManager creates a cached configuration manager
func NewCachedManager(manager *Manager, cacheTTL time.Duration) *CachedManager {
	return &CachedManager{
		Manager:  manager,
		cacheTTL: cacheTTL,
	}
}

// NewCachedManagerForCluster creates a cached configuration manager for a specific cluster
func NewCachedManagerForCluster(clusterName string, cacheTTL time.Duration) *CachedManager {
	return NewCachedManager(NewManagerForCluster(clusterName), cacheTTL)
}

// GetCached retrieves configuration from cache, loads from database if cache doesn't exist or is expired
func (cm *CachedManager) GetCached(ctx context.Context, key string, dest interface{}) error {
	// Check cache
	if entry, ok := cm.cache.Load(key); ok {
		cached := entry.(cacheEntry)
		if time.Now().Before(cached.expireTime) {
			// Cache is valid
			return unmarshalExtType(cached.value, dest)
		}
		// Cache expired, delete it
		cm.cache.Delete(key)
	}

	// Load from database
	if err := cm.Manager.Get(ctx, key, dest); err != nil {
		return err
	}

	// Update cache
	cm.cache.Store(key, cacheEntry{
		value:      dest,
		expireTime: time.Now().Add(cm.cacheTTL),
	})

	return nil
}

// SetCached sets configuration and updates cache
func (cm *CachedManager) SetCached(ctx context.Context, key string, value interface{}, opts ...SetOption) error {
	if err := cm.Manager.Set(ctx, key, value, opts...); err != nil {
		return err
	}

	// Update cache
	cm.cache.Store(key, cacheEntry{
		value:      value,
		expireTime: time.Now().Add(cm.cacheTTL),
	})

	return nil
}

// InvalidateCache invalidates the cache for a specific key
func (cm *CachedManager) InvalidateCache(key string) {
	cm.cache.Delete(key)
}

// InvalidateAllCache 清空所有缓存
func (cm *CachedManager) InvalidateAllCache() {
	cm.cache.Range(func(key, value interface{}) bool {
		cm.cache.Delete(key)
		return true
	})
}

// RefreshCache 刷新指定键的缓存
func (cm *CachedManager) RefreshCache(ctx context.Context, key string) error {
	cm.InvalidateCache(key)

	var temp interface{}
	if err := cm.Manager.Get(ctx, key, &temp); err != nil {
		return fmt.Errorf("failed to refresh cache for key '%s': %w", key, err)
	}

	cm.cache.Store(key, cacheEntry{
		value:      temp,
		expireTime: time.Now().Add(cm.cacheTTL),
	})

	return nil
}

// Preload 预加载指定的配置键到缓存
func (cm *CachedManager) Preload(ctx context.Context, keys []string) error {
	configs, err := cm.Manager.BatchGet(ctx, keys)
	if err != nil {
		return fmt.Errorf("failed to preload configs: %w", err)
	}

	now := time.Now()
	for key, config := range configs {
		cm.cache.Store(key, cacheEntry{
			value:      config.Value,
			expireTime: now.Add(cm.cacheTTL),
		})
	}

	return nil
}

// GetCacheStats 获取缓存统计信息
func (cm *CachedManager) GetCacheStats() map[string]interface{} {
	var (
		total   int
		expired int
	)

	now := time.Now()
	cm.cache.Range(func(key, value interface{}) bool {
		total++
		entry := value.(cacheEntry)
		if now.After(entry.expireTime) {
			expired++
		}
		return true
	})

	return map[string]interface{}{
		"total_entries":   total,
		"expired_entries": expired,
		"valid_entries":   total - expired,
		"cache_ttl":       cm.cacheTTL.String(),
	}
}
