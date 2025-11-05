package config_test

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/helper/config"
)

// ExampleGetOrInitConfigManager 演示如何获取单例配置管理器
func ExampleGetOrInitConfigManager() {
	ctx := context.Background()

	// 获取默认集群的配置管理器（单例）
	manager := config.GetOrInitConfigManager("")

	// 定义配置结构
	type AppConfig struct {
		AppName string `json:"app_name"`
		Version string `json:"version"`
		Debug   bool   `json:"debug"`
	}

	// 设置配置
	appConfig := AppConfig{
		AppName: "Primus Lens",
		Version: "1.0.0",
		Debug:   false,
	}

	err := manager.Set(ctx, "app.config", appConfig,
		config.WithDescription("应用配置"),
		config.WithCategory("application"),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// 读取配置
	var result AppConfig
	err = manager.Get(ctx, "app.config", &result)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("App: %s, Version: %s\n", result.AppName, result.Version)
}

// ExampleGetDefaultConfigManager 演示使用默认配置管理器
func ExampleGetDefaultConfigManager() {
	// 获取默认集群的配置管理器
	// 多次调用返回同一个实例
	manager1 := config.GetDefaultConfigManager()
	manager2 := config.GetDefaultConfigManager()

	// manager1 和 manager2 是同一个实例
	fmt.Printf("Same instance: %v\n", manager1 == manager2)
	// Output: Same instance: true
}

// ExampleGetConfigManagerForCluster 演示获取特定集群的配置管理器
func ExampleGetConfigManagerForCluster() {

	// 获取 cluster-a 的配置管理器
	managerA := config.GetConfigManagerForCluster("cluster-a")

	// 获取 cluster-b 的配置管理器
	managerB := config.GetConfigManagerForCluster("cluster-b")

	// 两个集群使用不同的管理器实例
	fmt.Printf("Different instances: %v\n", managerA != managerB)

	// 但同一个集群的多次获取是同一个实例
	managerA2 := config.GetConfigManagerForCluster("cluster-a")
	fmt.Printf("Same cluster, same instance: %v\n", managerA == managerA2)
}

// ExampleGetDefaultCachedConfigManager 演示使用带缓存的配置管理器
func ExampleGetDefaultCachedConfigManager() {
	ctx := context.Background()

	// 获取默认的带缓存配置管理器
	cachedManager := config.GetDefaultCachedConfigManager()

	type ServiceConfig struct {
		Host    string `json:"host"`
		Port    int    `json:"port"`
		Timeout int    `json:"timeout"`
	}

	serviceConfig := ServiceConfig{
		Host:    "api.example.com",
		Port:    8080,
		Timeout: 30,
	}

	// 设置配置
	err := cachedManager.SetCached(ctx, "service.config", serviceConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// 第一次读取（从数据库）
	var result1 ServiceConfig
	err = cachedManager.GetCached(ctx, "service.config", &result1)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// 第二次读取（从缓存，更快）
	var result2 ServiceConfig
	err = cachedManager.GetCached(ctx, "service.config", &result2)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Service: %s:%d\n", result1.Host, result1.Port)
}

// ExampleGetOrInitCachedConfigManager 演示自定义缓存 TTL
func ExampleGetOrInitCachedConfigManager() {
	// 获取带 10 分钟缓存的配置管理器
	cachedManager := config.GetOrInitCachedConfigManager("default", 10*time.Minute)

	// 获取缓存统计
	stats := cachedManager.GetCacheStats()
	fmt.Printf("Cache TTL: %v\n", stats["cache_ttl"])
}

// ExampleGetConfigManagerStats 演示获取配置管理器统计信息
func ExampleGetConfigManagerStats() {
	// 初始化多个集群的管理器
	config.GetOrInitConfigManager("")
	config.GetConfigManagerForCluster("cluster-a")
	config.GetConfigManagerForCluster("cluster-b")
	config.GetDefaultCachedConfigManager()

	// 获取统计信息
	stats := config.GetConfigManagerStats()

	fmt.Printf("Total managers: %d\n", stats.TotalManagers)
	fmt.Printf("Total cached managers: %d\n", stats.TotalCachedManagers)
	fmt.Printf("Clusters: %v\n", stats.Clusters)
	fmt.Printf("Cached clusters: %v\n", stats.CachedClusters)
	fmt.Printf("Default cache TTL: %s\n", stats.DefaultCacheTTL)
}

// ExampleSetDefaultCacheTTL 演示设置默认缓存 TTL
func ExampleSetDefaultCacheTTL() {
	// 设置默认缓存 TTL 为 15 分钟
	config.SetDefaultCacheTTL(15 * time.Minute)

	// 获取默认 TTL
	ttl := config.GetDefaultCacheTTL()
	fmt.Printf("Default cache TTL: %s\n", ttl)

	// 之后创建的缓存管理器将使用新的 TTL
	cachedManager := config.GetDefaultCachedConfigManager()
	_ = cachedManager // 使用管理器
}

// ExampleResetConfigManager 演示重置配置管理器
func ExampleResetConfigManager() {
	// 初始化管理器
	manager1 := config.GetConfigManagerForCluster("test-cluster")

	// 重置指定集群的管理器
	config.ResetConfigManager("test-cluster")

	// 再次获取将创建新的实例
	manager2 := config.GetConfigManagerForCluster("test-cluster")

	// manager1 和 manager2 是不同的实例
	fmt.Printf("Different instances after reset: %v\n", manager1 != manager2)
}

// Example_simpleUsage 演示最简单的使用方式
func Example_simpleUsage() {
	ctx := context.Background()

	// 1. 获取配置管理器（单例）
	manager := config.GetDefaultConfigManager()

	// 2. 定义配置结构
	type MyConfig struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	// 3. 设置配置
	cfg := MyConfig{Host: "localhost", Port: 8080}
	manager.Set(ctx, "my.service", cfg)

	// 4. 读取配置
	var result MyConfig
	manager.Get(ctx, "my.service", &result)

	fmt.Printf("%s:%d\n", result.Host, result.Port)
	// Output: localhost:8080
}

// Example_multiCluster 演示多集群场景
func Example_multiCluster() {
	ctx := context.Background()

	// 不同集群使用不同的配置管理器
	clusterA := config.GetConfigManagerForCluster("cluster-a")
	clusterB := config.GetConfigManagerForCluster("cluster-b")

	type ClusterConfig struct {
		Name string `json:"name"`
	}

	// 为 cluster-a 设置配置
	clusterA.Set(ctx, "cluster.info", ClusterConfig{Name: "Cluster A"})

	// 为 cluster-b 设置配置
	clusterB.Set(ctx, "cluster.info", ClusterConfig{Name: "Cluster B"})

	// 读取各自的配置
	var cfgA, cfgB ClusterConfig
	clusterA.Get(ctx, "cluster.info", &cfgA)
	clusterB.Get(ctx, "cluster.info", &cfgB)

	fmt.Printf("Cluster A: %s\n", cfgA.Name)
	fmt.Printf("Cluster B: %s\n", cfgB.Name)
}

// Example_withCache 演示使用缓存提升性能
func Example_withCache() {
	ctx := context.Background()

	// 获取带缓存的配置管理器
	cachedManager := config.GetDefaultCachedConfigManager()

	type Config struct {
		Value string `json:"value"`
	}

	// 预加载配置
	configs := []string{"config1", "config2", "config3"}
	cachedManager.Preload(ctx, configs)

	// 后续读取将从缓存中获取
	var cfg Config
	cachedManager.GetCached(ctx, "config1", &cfg)

	// 查看缓存统计
	stats := cachedManager.GetCacheStats()
	fmt.Printf("Cache stats: %+v\n", stats)
}

// Example_threadSafe 演示线程安全的单例获取
func Example_threadSafe() {
	// 多个 goroutine 同时获取配置管理器
	// 保证只创建一个实例
	done := make(chan *config.Manager, 10)

	for i := 0; i < 10; i++ {
		go func() {
			manager := config.GetDefaultConfigManager()
			done <- manager
		}()
	}

	// 收集所有管理器实例
	managers := make([]*config.Manager, 10)
	for i := 0; i < 10; i++ {
		managers[i] = <-done
	}

	// 验证都是同一个实例
	allSame := true
	for i := 1; i < 10; i++ {
		if managers[i] != managers[0] {
			allSame = false
			break
		}
	}

	fmt.Printf("All instances are the same: %v\n", allSame)
	// Output: All instances are the same: true
}
