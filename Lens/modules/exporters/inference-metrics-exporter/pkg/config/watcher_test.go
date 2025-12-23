package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExtractFrameworkFromKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"vllm", "inference.metrics.config.vllm", "vllm"},
		{"tgi", "inference.metrics.config.tgi", "tgi"},
		{"triton", "inference.metrics.config.triton", "triton"},
		{"empty key", "", ""},
		{"short key", "inference.metrics.config.", ""},
		{"different prefix", "other.config.vllm", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFrameworkFromKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComputeHash(t *testing.T) {
	t.Run("same value same hash", func(t *testing.T) {
		value := map[string]interface{}{
			"framework": "vllm",
			"mappings":  []string{"a", "b"},
		}
		hash1 := computeHash(value)
		hash2 := computeHash(value)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different values different hashes", func(t *testing.T) {
		value1 := map[string]interface{}{"framework": "vllm"}
		value2 := map[string]interface{}{"framework": "tgi"}
		hash1 := computeHash(value1)
		hash2 := computeHash(value2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("nil value empty hash", func(t *testing.T) {
		// nil marshals to "null" which gives a consistent hash
		hash := computeHash(nil)
		assert.NotEmpty(t, hash)
	})
}

func TestNewConfigWatcher(t *testing.T) {
	watcher := NewConfigWatcher(30 * time.Second)
	assert.NotNil(t, watcher)
	assert.Equal(t, 30*time.Second, watcher.interval)
	assert.NotNil(t, watcher.configHashes)
}

func TestConfigWatcher_GetLoadedFrameworks(t *testing.T) {
	watcher := NewConfigWatcher(time.Minute)

	// Initially empty
	frameworks := watcher.GetLoadedFrameworks()
	assert.Empty(t, frameworks)

	// Add some hashes
	watcher.hashMu.Lock()
	watcher.configHashes["vllm"] = "abc123"
	watcher.configHashes["tgi"] = "def456"
	watcher.hashMu.Unlock()

	frameworks = watcher.GetLoadedFrameworks()
	assert.Len(t, frameworks, 2)
	assert.Contains(t, frameworks, "vllm")
	assert.Contains(t, frameworks, "tgi")
}

func TestConfigWatcher_GetConfigHash(t *testing.T) {
	watcher := NewConfigWatcher(time.Minute)

	// Non-existent
	hash, ok := watcher.GetConfigHash("unknown")
	assert.False(t, ok)
	assert.Empty(t, hash)

	// Add hash
	watcher.hashMu.Lock()
	watcher.configHashes["vllm"] = "abc123"
	watcher.hashMu.Unlock()

	hash, ok = watcher.GetConfigHash("vllm")
	assert.True(t, ok)
	assert.Equal(t, "abc123", hash)
}

func TestConfigWatcher_GetStats(t *testing.T) {
	watcher := NewConfigWatcher(30 * time.Second)

	watcher.hashMu.Lock()
	watcher.configHashes["vllm"] = "abcdef1234567890"
	watcher.configHashes["tgi"] = "1234567890abcdef"
	watcher.hashMu.Unlock()

	stats := watcher.GetStats()
	assert.Len(t, stats.LoadedFrameworks, 2)
	assert.Len(t, stats.ConfigHashes, 2)
	assert.Equal(t, "30s", stats.WatchInterval)

	// Hashes should be truncated
	for _, h := range stats.ConfigHashes {
		assert.Contains(t, h, "...")
	}
}

func TestConfigKeyPrefix(t *testing.T) {
	assert.Equal(t, "inference.metrics.config.", ConfigKeyPrefix)
}
