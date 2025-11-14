package config_test

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
)

// ExampleGetOrInitConfigManager demonstrates how to get a singleton configuration manager
func ExampleGetOrInitConfigManager() {
	ctx := context.Background()

	// Get the configuration manager for the default cluster (singleton)
	manager := config.GetOrInitConfigManager("")

	// Define configuration structure
	type AppConfig struct {
		AppName string `json:"app_name"`
		Version string `json:"version"`
		Debug   bool   `json:"debug"`
	}

	// Set configuration
	appConfig := AppConfig{
		AppName: "Primus Lens",
		Version: "1.0.0",
		Debug:   false,
	}

	err := manager.Set(ctx, "app.config", appConfig,
		config.WithDescription("Application configuration"),
		config.WithCategory("application"),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Read configuration
	var result AppConfig
	err = manager.Get(ctx, "app.config", &result)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("App: %s, Version: %s\n", result.AppName, result.Version)
}

// ExampleGetDefaultConfigManager demonstrates using the default configuration manager
func ExampleGetDefaultConfigManager() {
	// Get the configuration manager for the default cluster
	// Multiple calls return the same instance
	manager1 := config.GetDefaultConfigManager()
	manager2 := config.GetDefaultConfigManager()

	// manager1 and manager2 are the same instance
	fmt.Printf("Same instance: %v\n", manager1 == manager2)
	// Output: Same instance: true
}

// ExampleGetConfigManagerForCluster demonstrates getting configuration manager for a specific cluster
func ExampleGetConfigManagerForCluster() {

	// Get configuration manager for cluster-a
	managerA := config.GetConfigManagerForCluster("cluster-a")

	// Get configuration manager for cluster-b
	managerB := config.GetConfigManagerForCluster("cluster-b")

	// Two clusters use different manager instances
	fmt.Printf("Different instances: %v\n", managerA != managerB)

	// But multiple gets for the same cluster return the same instance
	managerA2 := config.GetConfigManagerForCluster("cluster-a")
	fmt.Printf("Same cluster, same instance: %v\n", managerA == managerA2)
}

// ExampleGetDefaultCachedConfigManager demonstrates using cached configuration manager
func ExampleGetDefaultCachedConfigManager() {
	ctx := context.Background()

	// Get default cached configuration manager
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

	// Set configuration
	err := cachedManager.SetCached(ctx, "service.config", serviceConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// First read (from database)
	var result1 ServiceConfig
	err = cachedManager.GetCached(ctx, "service.config", &result1)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Second read (from cache, faster)
	var result2 ServiceConfig
	err = cachedManager.GetCached(ctx, "service.config", &result2)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Service: %s:%d\n", result1.Host, result1.Port)
}

// ExampleGetOrInitCachedConfigManager demonstrates custom cache TTL
func ExampleGetOrInitCachedConfigManager() {
	// Get configuration manager with 10 minute cache
	cachedManager := config.GetOrInitCachedConfigManager("default", 10*time.Minute)

	// Get cache statistics
	stats := cachedManager.GetCacheStats()
	fmt.Printf("Cache TTL: %v\n", stats["cache_ttl"])
}

// ExampleGetConfigManagerStats demonstrates getting configuration manager statistics
func ExampleGetConfigManagerStats() {
	// Initialize managers for multiple clusters
	config.GetOrInitConfigManager("")
	config.GetConfigManagerForCluster("cluster-a")
	config.GetConfigManagerForCluster("cluster-b")
	config.GetDefaultCachedConfigManager()

	// Get statistics
	stats := config.GetConfigManagerStats()

	fmt.Printf("Total managers: %d\n", stats.TotalManagers)
	fmt.Printf("Total cached managers: %d\n", stats.TotalCachedManagers)
	fmt.Printf("Clusters: %v\n", stats.Clusters)
	fmt.Printf("Cached clusters: %v\n", stats.CachedClusters)
	fmt.Printf("Default cache TTL: %s\n", stats.DefaultCacheTTL)
}

// ExampleSetDefaultCacheTTL demonstrates setting default cache TTL
func ExampleSetDefaultCacheTTL() {
	// Set default cache TTL to 15 minutes
	config.SetDefaultCacheTTL(15 * time.Minute)

	// Get default TTL
	ttl := config.GetDefaultCacheTTL()
	fmt.Printf("Default cache TTL: %s\n", ttl)

	// Cache managers created afterwards will use the new TTL
	cachedManager := config.GetDefaultCachedConfigManager()
	_ = cachedManager // Use manager
}

// ExampleResetConfigManager demonstrates resetting configuration manager
func ExampleResetConfigManager() {
	// Initialize manager
	manager1 := config.GetConfigManagerForCluster("test-cluster")

	// Reset manager for specified cluster
	config.ResetConfigManager("test-cluster")

	// Getting again will create a new instance
	manager2 := config.GetConfigManagerForCluster("test-cluster")

	// manager1 and manager2 are different instances
	fmt.Printf("Different instances after reset: %v\n", manager1 != manager2)
}

// Example_simpleUsage demonstrates the simplest usage
func Example_simpleUsage() {
	ctx := context.Background()

	// 1. Get configuration manager (singleton)
	manager := config.GetDefaultConfigManager()

	// 2. Define configuration structure
	type MyConfig struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	// 3. Set configuration
	cfg := MyConfig{Host: "localhost", Port: 8080}
	manager.Set(ctx, "my.service", cfg)

	// 4. Read configuration
	var result MyConfig
	manager.Get(ctx, "my.service", &result)

	fmt.Printf("%s:%d\n", result.Host, result.Port)
	// Output: localhost:8080
}

// Example_multiCluster demonstrates multi-cluster scenario
func Example_multiCluster() {
	ctx := context.Background()

	// Different clusters use different configuration managers
	clusterA := config.GetConfigManagerForCluster("cluster-a")
	clusterB := config.GetConfigManagerForCluster("cluster-b")

	type ClusterConfig struct {
		Name string `json:"name"`
	}

	// Set configuration for cluster-a
	clusterA.Set(ctx, "cluster.info", ClusterConfig{Name: "Cluster A"})

	// Set configuration for cluster-b
	clusterB.Set(ctx, "cluster.info", ClusterConfig{Name: "Cluster B"})

	// Read respective configurations
	var cfgA, cfgB ClusterConfig
	clusterA.Get(ctx, "cluster.info", &cfgA)
	clusterB.Get(ctx, "cluster.info", &cfgB)

	fmt.Printf("Cluster A: %s\n", cfgA.Name)
	fmt.Printf("Cluster B: %s\n", cfgB.Name)
}

// Example_withCache demonstrates using cache to improve performance
func Example_withCache() {
	ctx := context.Background()

	// Get cached configuration manager
	cachedManager := config.GetDefaultCachedConfigManager()

	type Config struct {
		Value string `json:"value"`
	}

	// Preload configurations
	configs := []string{"config1", "config2", "config3"}
	cachedManager.Preload(ctx, configs)

	// Subsequent reads will be from cache
	var cfg Config
	cachedManager.GetCached(ctx, "config1", &cfg)

	// View cache statistics
	stats := cachedManager.GetCacheStats()
	fmt.Printf("Cache stats: %+v\n", stats)
}

// Example_threadSafe demonstrates thread-safe singleton retrieval
func Example_threadSafe() {
	// Multiple goroutines get configuration manager simultaneously
	// Ensures only one instance is created
	done := make(chan *config.Manager, 10)

	for i := 0; i < 10; i++ {
		go func() {
			manager := config.GetDefaultConfigManager()
			done <- manager
		}()
	}

	// Collect all manager instances
	managers := make([]*config.Manager, 10)
	for i := 0; i < 10; i++ {
		managers[i] = <-done
	}

	// Verify all are the same instance
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
