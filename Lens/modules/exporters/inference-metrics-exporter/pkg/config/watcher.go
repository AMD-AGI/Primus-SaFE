package config

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	configHelper "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/exporter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/transformer"
)

const (
	// ConfigKeyPrefix is the prefix for inference metrics config keys
	ConfigKeyPrefix = "inference.metrics.config."
)

// ConfigWatcher watches for configuration changes and hot-reloads them
type ConfigWatcher struct {
	configMgr *configHelper.Manager
	interval  time.Duration

	// Track config hashes to detect changes
	configHashes map[string]string
	hashMu       sync.RWMutex

	// Reload callbacks
	onReload func(framework string)

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewConfigWatcher creates a new config watcher
func NewConfigWatcher(interval time.Duration) *ConfigWatcher {
	return &ConfigWatcher{
		configMgr:    configHelper.GetDefaultConfigManager(),
		interval:     interval,
		configHashes: make(map[string]string),
	}
}

// SetReloadCallback sets the callback function called when config is reloaded
func (w *ConfigWatcher) SetReloadCallback(fn func(framework string)) {
	w.onReload = fn
}

// Start starts the config watcher
func (w *ConfigWatcher) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	// Initial load
	if err := w.loadAllConfigs(); err != nil {
		log.Errorf("Failed to load initial configs: %v", err)
		// Continue anyway, configs can be loaded later
	}

	// Start watch loop
	w.wg.Add(1)
	go w.watchLoop()

	log.Infof("Config watcher started with interval=%v", w.interval)
	return nil
}

// Stop stops the config watcher
func (w *ConfigWatcher) Stop() error {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	log.Info("Config watcher stopped")
	return nil
}

// watchLoop periodically checks for config changes
func (w *ConfigWatcher) watchLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			if err := w.checkForChanges(); err != nil {
				log.Errorf("Failed to check for config changes: %v", err)
			}
		}
	}
}

// loadAllConfigs loads all framework configs from the database
func (w *ConfigWatcher) loadAllConfigs() error {
	configs, err := w.configMgr.List(w.ctx, configHelper.WithKeyPrefixFilter(ConfigKeyPrefix))
	if err != nil {
		return fmt.Errorf("list configs: %w", err)
	}

	loadedCount := 0
	for _, cfg := range configs {
		framework := extractFrameworkFromKey(cfg.Key)
		if framework == "" {
			continue
		}

		if err := w.loadAndApplyConfig(framework, cfg.Value); err != nil {
			log.Errorf("Failed to load config for %s: %v", framework, err)
			continue
		}

		// Store hash for change detection
		hash := computeHash(cfg.Value)
		w.hashMu.Lock()
		w.configHashes[framework] = hash
		w.hashMu.Unlock()

		loadedCount++
	}

	log.Infof("Loaded %d framework configs from database", loadedCount)
	return nil
}

// checkForChanges checks if any config has changed
func (w *ConfigWatcher) checkForChanges() error {
	configs, err := w.configMgr.List(w.ctx, configHelper.WithKeyPrefixFilter(ConfigKeyPrefix))
	if err != nil {
		return fmt.Errorf("list configs: %w", err)
	}

	for _, cfg := range configs {
		framework := extractFrameworkFromKey(cfg.Key)
		if framework == "" {
			continue
		}

		newHash := computeHash(cfg.Value)

		w.hashMu.RLock()
		oldHash, exists := w.configHashes[framework]
		w.hashMu.RUnlock()

		if !exists || oldHash != newHash {
			log.Infof("Config change detected for framework %s", framework)

			if err := w.loadAndApplyConfig(framework, cfg.Value); err != nil {
				log.Errorf("Failed to apply config for %s: %v", framework, err)
				exporter.RecordConfigReload(framework, false)
				continue
			}

			// Update hash
			w.hashMu.Lock()
			w.configHashes[framework] = newHash
			w.hashMu.Unlock()

			exporter.RecordConfigReload(framework, true)

			// Call reload callback
			if w.onReload != nil {
				w.onReload(framework)
			}
		}
	}

	return nil
}

// loadAndApplyConfig loads and applies a framework config
func (w *ConfigWatcher) loadAndApplyConfig(framework string, value interface{}) error {
	// Convert to FrameworkMetricsConfig
	var cfg transformer.FrameworkMetricsConfig

	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, &cfg); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// Validate config
	if cfg.Framework == "" {
		cfg.Framework = framework
	}

	// Apply to transformer registry
	if err := transformer.DefaultRegistry.UpdateConfig(framework, &cfg); err != nil {
		return fmt.Errorf("update transformer: %w", err)
	}

	log.Infof("Applied config for framework %s: %d mappings", framework, len(cfg.Mappings))
	return nil
}

// ReloadFramework reloads config for a specific framework
func (w *ConfigWatcher) ReloadFramework(ctx context.Context, framework string) error {
	key := ConfigKeyPrefix + framework

	var cfg transformer.FrameworkMetricsConfig
	if err := w.configMgr.Get(ctx, key, &cfg); err != nil {
		return fmt.Errorf("get config: %w", err)
	}

	if cfg.Framework == "" {
		cfg.Framework = framework
	}

	if err := transformer.DefaultRegistry.UpdateConfig(framework, &cfg); err != nil {
		return fmt.Errorf("update transformer: %w", err)
	}

	// Update hash
	cfgBytes, _ := json.Marshal(cfg)
	w.hashMu.Lock()
	w.configHashes[framework] = computeHash(cfgBytes)
	w.hashMu.Unlock()

	log.Infof("Reloaded config for framework %s", framework)
	exporter.RecordConfigReload(framework, true)

	return nil
}

// ReloadAll reloads all configs
func (w *ConfigWatcher) ReloadAll(ctx context.Context) ([]string, error) {
	configs, err := w.configMgr.List(ctx, configHelper.WithKeyPrefixFilter(ConfigKeyPrefix))
	if err != nil {
		return nil, fmt.Errorf("list configs: %w", err)
	}

	reloaded := make([]string, 0)
	for _, cfg := range configs {
		framework := extractFrameworkFromKey(cfg.Key)
		if framework == "" {
			continue
		}

		if err := w.loadAndApplyConfig(framework, cfg.Value); err != nil {
			log.Errorf("Failed to reload config for %s: %v", framework, err)
			exporter.RecordConfigReload(framework, false)
			continue
		}

		// Update hash
		hash := computeHash(cfg.Value)
		w.hashMu.Lock()
		w.configHashes[framework] = hash
		w.hashMu.Unlock()

		exporter.RecordConfigReload(framework, true)
		reloaded = append(reloaded, framework)
	}

	return reloaded, nil
}

// GetLoadedFrameworks returns the list of frameworks with loaded configs
func (w *ConfigWatcher) GetLoadedFrameworks() []string {
	w.hashMu.RLock()
	defer w.hashMu.RUnlock()

	frameworks := make([]string, 0, len(w.configHashes))
	for f := range w.configHashes {
		frameworks = append(frameworks, f)
	}
	return frameworks
}

// GetConfigHash returns the hash of a framework's config
func (w *ConfigWatcher) GetConfigHash(framework string) (string, bool) {
	w.hashMu.RLock()
	defer w.hashMu.RUnlock()
	h, ok := w.configHashes[framework]
	return h, ok
}

// Helper functions

// extractFrameworkFromKey extracts framework name from config key
// e.g., "inference.metrics.config.vllm" -> "vllm"
func extractFrameworkFromKey(key string) string {
	if len(key) <= len(ConfigKeyPrefix) {
		return ""
	}
	return key[len(ConfigKeyPrefix):]
}

// computeHash computes MD5 hash of a value
func computeHash(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", md5.Sum(data))
}

// WatcherStats contains statistics about the config watcher
type WatcherStats struct {
	LoadedFrameworks []string          `json:"loaded_frameworks"`
	ConfigHashes     map[string]string `json:"config_hashes"`
	WatchInterval    string            `json:"watch_interval"`
}

// GetStats returns watcher statistics
func (w *ConfigWatcher) GetStats() WatcherStats {
	w.hashMu.RLock()
	defer w.hashMu.RUnlock()

	stats := WatcherStats{
		LoadedFrameworks: make([]string, 0, len(w.configHashes)),
		ConfigHashes:     make(map[string]string),
		WatchInterval:    w.interval.String(),
	}

	for f, h := range w.configHashes {
		stats.LoadedFrameworks = append(stats.LoadedFrameworks, f)
		stats.ConfigHashes[f] = h[:8] + "..." // Truncated hash
	}

	return stats
}
