package config

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CachedManager 带缓存的配置管理器
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

// NewCachedManager 创建带缓存的配置管理器
func NewCachedManager(manager *Manager, cacheTTL time.Duration) *CachedManager {
	return &CachedManager{
		Manager:  manager,
		cacheTTL: cacheTTL,
	}
}

// NewCachedManagerForCluster 根据集群名称创建带缓存的配置管理器
func NewCachedManagerForCluster(clusterName string, cacheTTL time.Duration) *CachedManager {
	return NewCachedManager(NewManagerForCluster(clusterName), cacheTTL)
}

// GetCached 从缓存中获取配置，如果缓存不存在或过期则从数据库加载
func (cm *CachedManager) GetCached(ctx context.Context, key string, dest interface{}) error {
	// 检查缓存
	if entry, ok := cm.cache.Load(key); ok {
		cached := entry.(cacheEntry)
		if time.Now().Before(cached.expireTime) {
			// 缓存有效
			return unmarshalExtType(cached.value, dest)
		}
		// 缓存过期，删除
		cm.cache.Delete(key)
	}

	// 从数据库加载
	if err := cm.Manager.Get(ctx, key, dest); err != nil {
		return err
	}

	// 更新缓存
	cm.cache.Store(key, cacheEntry{
		value:      dest,
		expireTime: time.Now().Add(cm.cacheTTL),
	})

	return nil
}

// SetCached 设置配置并更新缓存
func (cm *CachedManager) SetCached(ctx context.Context, key string, value interface{}, opts ...SetOption) error {
	if err := cm.Manager.Set(ctx, key, value, opts...); err != nil {
		return err
	}

	// 更新缓存
	cm.cache.Store(key, cacheEntry{
		value:      value,
		expireTime: time.Now().Add(cm.cacheTTL),
	})

	return nil
}

// InvalidateCache 使指定键的缓存失效
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
