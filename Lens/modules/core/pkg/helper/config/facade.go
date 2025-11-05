package config

import (
	"sync"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"gorm.io/gorm"
)

// 全局单例管理器映射，按集群名称存储
var (
	// managers 存储普通配置管理器的单例
	managers = make(map[string]*Manager)
	// cachedManagers 存储带缓存的配置管理器的单例
	cachedManagers = make(map[string]*CachedManager)
	// managerMutex 保护 managers 映射的并发访问
	managerMutex sync.RWMutex
	// cachedManagerMutex 保护 cachedManagers 映射的并发访问
	cachedManagerMutex sync.RWMutex
	// defaultCacheTTL 默认缓存有效期
	defaultCacheTTL = 5 * time.Minute
)

// GetOrInitConfigManager 获取或初始化指定集群的配置管理器（单例）
// clusterName: 集群名称，空字符串表示默认集群
// 返回该集群的单例配置管理器
func GetOrInitConfigManager(clusterName string) *Manager {
	// 先尝试读取
	managerMutex.RLock()
	if mgr, exists := managers[clusterName]; exists {
		managerMutex.RUnlock()
		return mgr
	}
	managerMutex.RUnlock()

	// 需要创建新的管理器
	managerMutex.Lock()
	defer managerMutex.Unlock()

	// 双重检查，避免重复创建
	if mgr, exists := managers[clusterName]; exists {
		return mgr
	}

	// 创建新的管理器
	var mgr *Manager
	if clusterName == "" {
		// 使用默认数据库
		facade := database.GetFacade()
		mgr = &Manager{
			db: facade.GetSystemConfig().GetDB(),
		}
	} else {
		// 使用指定集群的数据库
		mgr = NewManagerForCluster(clusterName)
	}

	managers[clusterName] = mgr
	return mgr
}

// GetOrInitCachedConfigManager 获取或初始化指定集群的带缓存配置管理器（单例）
// clusterName: 集群名称，空字符串表示默认集群
// cacheTTL: 缓存有效期，如果为 0 则使用默认值（5分钟）
// 返回该集群的单例带缓存配置管理器
func GetOrInitCachedConfigManager(clusterName string, cacheTTL time.Duration) *CachedManager {
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}

	// 先尝试读取
	cachedManagerMutex.RLock()
	if mgr, exists := cachedManagers[clusterName]; exists {
		cachedManagerMutex.RUnlock()
		return mgr
	}
	cachedManagerMutex.RUnlock()

	// 需要创建新的管理器
	cachedManagerMutex.Lock()
	defer cachedManagerMutex.Unlock()

	// 双重检查，避免重复创建
	if mgr, exists := cachedManagers[clusterName]; exists {
		return mgr
	}

	// 创建新的带缓存管理器
	baseManager := GetOrInitConfigManager(clusterName)
	cachedMgr := NewCachedManager(baseManager, cacheTTL)

	cachedManagers[clusterName] = cachedMgr
	return cachedMgr
}

// GetDefaultConfigManager 获取默认集群的配置管理器（单例）
// 等同于 GetOrInitConfigManager("")
func GetDefaultConfigManager() *Manager {
	return GetOrInitConfigManager("")
}

// GetDefaultCachedConfigManager 获取默认集群的带缓存配置管理器（单例）
// 使用默认缓存有效期（5分钟）
func GetDefaultCachedConfigManager() *CachedManager {
	return GetOrInitCachedConfigManager("", defaultCacheTTL)
}

// GetConfigManagerForCluster 获取指定集群的配置管理器（单例）
// 等同于 GetOrInitConfigManager(clusterName)
func GetConfigManagerForCluster(clusterName string) *Manager {
	return GetOrInitConfigManager(clusterName)
}

// GetCachedConfigManagerForCluster 获取指定集群的带缓存配置管理器（单例）
// 使用默认缓存有效期（5分钟）
func GetCachedConfigManagerForCluster(clusterName string) *CachedManager {
	return GetOrInitCachedConfigManager(clusterName, defaultCacheTTL)
}

// ResetConfigManager 重置指定集群的配置管理器
// 通常用于测试或需要重新初始化的场景
func ResetConfigManager(clusterName string) {
	managerMutex.Lock()
	delete(managers, clusterName)
	managerMutex.Unlock()

	cachedManagerMutex.Lock()
	delete(cachedManagers, clusterName)
	cachedManagerMutex.Unlock()
}

// ResetAllConfigManagers 重置所有配置管理器
// 通常用于测试场景
func ResetAllConfigManagers() {
	managerMutex.Lock()
	managers = make(map[string]*Manager)
	managerMutex.Unlock()

	cachedManagerMutex.Lock()
	cachedManagers = make(map[string]*CachedManager)
	cachedManagerMutex.Unlock()
}

// SetDefaultCacheTTL 设置默认缓存有效期
// 仅影响后续创建的缓存管理器，不影响已存在的
func SetDefaultCacheTTL(ttl time.Duration) {
	if ttl > 0 {
		defaultCacheTTL = ttl
	}
}

// GetDefaultCacheTTL 获取默认缓存有效期
func GetDefaultCacheTTL() time.Duration {
	return defaultCacheTTL
}

// ConfigManagerStats 配置管理器统计信息
type ConfigManagerStats struct {
	TotalManagers       int            `json:"total_managers"`        // 总管理器数量
	TotalCachedManagers int            `json:"total_cached_managers"` // 总缓存管理器数量
	Clusters            []string       `json:"clusters"`              // 集群列表
	CachedClusters      []string       `json:"cached_clusters"`       // 使用缓存的集群列表
	DefaultCacheTTL     string         `json:"default_cache_ttl"`     // 默认缓存TTL
	CacheStats          map[string]interface{} `json:"cache_stats,omitempty"` // 缓存统计（如果有）
}

// GetConfigManagerStats 获取配置管理器的统计信息
func GetConfigManagerStats() *ConfigManagerStats {
	stats := &ConfigManagerStats{
		DefaultCacheTTL: defaultCacheTTL.String(),
		CacheStats:      make(map[string]interface{}),
	}

	// 统计普通管理器
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

	// 统计缓存管理器
	cachedManagerMutex.RLock()
	stats.TotalCachedManagers = len(cachedManagers)
	stats.CachedClusters = make([]string, 0, len(cachedManagers))
	for cluster, cachedMgr := range cachedManagers {
		clusterName := cluster
		if cluster == "" {
			clusterName = "<default>"
		}
		stats.CachedClusters = append(stats.CachedClusters, clusterName)
		// 获取每个集群的缓存统计
		stats.CacheStats[clusterName] = cachedMgr.GetCacheStats()
	}
	cachedManagerMutex.RUnlock()

	return stats
}

// WithDB 创建一个使用指定数据库连接的配置管理器
// 不使用单例模式，每次调用都返回新实例
// db: GORM 数据库连接
func WithDB(db *gorm.DB) *Manager {
	return NewManager(db)
}

// WithDBAndCache 创建一个使用指定数据库连接的带缓存配置管理器
// 不使用单例模式，每次调用都返回新实例
// db: GORM 数据库连接
// cacheTTL: 缓存有效期
func WithDBAndCache(db *gorm.DB, cacheTTL time.Duration) *CachedManager {
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}
	return NewCachedManager(NewManager(db), cacheTTL)
}

