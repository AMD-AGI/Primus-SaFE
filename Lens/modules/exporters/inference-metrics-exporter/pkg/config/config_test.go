package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadExporterConfig(t *testing.T) {
	t.Run("loads defaults", func(t *testing.T) {
		// Clear any environment variables
		os.Unsetenv("TASK_POLL_INTERVAL")
		os.Unsetenv("LOCK_DURATION")
		os.Unsetenv("POD_NAME")
		os.Unsetenv("HOSTNAME")

		cfg := LoadExporterConfig()

		assert.NotEmpty(t, cfg.InstanceID)
		assert.Equal(t, DefaultTaskPollInterval, cfg.TaskPollInterval)
		assert.Equal(t, DefaultLockDuration, cfg.LockDuration)
		assert.Equal(t, DefaultLockRenewInterval, cfg.LockRenewInterval)
		assert.Equal(t, DefaultScrapeInterval, cfg.DefaultScrapeInterval)
		assert.Equal(t, DefaultScrapeTimeout, cfg.DefaultScrapeTimeout)
		assert.Equal(t, DefaultMaxConcurrentScrapes, cfg.MaxConcurrentScrapes)
	})

	t.Run("uses POD_NAME for instance ID", func(t *testing.T) {
		os.Setenv("POD_NAME", "my-exporter-pod-0")
		defer os.Unsetenv("POD_NAME")

		cfg := LoadExporterConfig()
		assert.Equal(t, "my-exporter-pod-0", cfg.InstanceID)
	})

	t.Run("uses HOSTNAME as fallback", func(t *testing.T) {
		os.Unsetenv("POD_NAME")
		os.Setenv("HOSTNAME", "my-hostname")
		defer os.Unsetenv("HOSTNAME")

		cfg := LoadExporterConfig()
		assert.Equal(t, "my-hostname", cfg.InstanceID)
	})

	t.Run("parses duration from env", func(t *testing.T) {
		os.Setenv("TASK_POLL_INTERVAL", "10s")
		os.Setenv("LOCK_DURATION", "2m")
		defer os.Unsetenv("TASK_POLL_INTERVAL")
		defer os.Unsetenv("LOCK_DURATION")

		cfg := LoadExporterConfig()
		assert.Equal(t, 10*time.Second, cfg.TaskPollInterval)
		assert.Equal(t, 2*time.Minute, cfg.LockDuration)
	})

	t.Run("parses int from env", func(t *testing.T) {
		os.Setenv("MAX_CONCURRENT_SCRAPES", "100")
		defer os.Unsetenv("MAX_CONCURRENT_SCRAPES")

		cfg := LoadExporterConfig()
		assert.Equal(t, 100, cfg.MaxConcurrentScrapes)
	})

	t.Run("uses default for invalid duration", func(t *testing.T) {
		os.Setenv("TASK_POLL_INTERVAL", "invalid")
		defer os.Unsetenv("TASK_POLL_INTERVAL")

		cfg := LoadExporterConfig()
		assert.Equal(t, DefaultTaskPollInterval, cfg.TaskPollInterval)
	})

	t.Run("uses default for invalid int", func(t *testing.T) {
		os.Setenv("MAX_CONCURRENT_SCRAPES", "not-a-number")
		defer os.Unsetenv("MAX_CONCURRENT_SCRAPES")

		cfg := LoadExporterConfig()
		assert.Equal(t, DefaultMaxConcurrentScrapes, cfg.MaxConcurrentScrapes)
	})
}

func TestTaskTypeConstant(t *testing.T) {
	assert.Equal(t, "inference_metrics_scrape", TaskTypeInferenceMetricsScrape)
}

