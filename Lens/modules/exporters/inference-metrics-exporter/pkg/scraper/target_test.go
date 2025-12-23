package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScrapeTargetFromConfig(t *testing.T) {
	exp := exporter.NewMetricsExporter()

	cfg := &TargetConfig{
		WorkloadUID:    "test-uid",
		Framework:      "vllm",
		Namespace:      "ml-serving",
		PodName:        "vllm-0",
		PodIP:          "10.0.1.1",
		MetricsURL:     "http://10.0.1.1:8000/metrics",
		ScrapeInterval: 30 * time.Second,
		ScrapeTimeout:  15 * time.Second,
		Labels: map[string]string{
			"env": "prod",
		},
	}

	target := NewScrapeTargetFromConfig(cfg, exp)

	assert.Equal(t, "test-uid", target.WorkloadUID)
	assert.Equal(t, "vllm", target.Framework)
	assert.Equal(t, "ml-serving", target.Namespace)
	assert.Equal(t, "vllm-0", target.PodName)
	assert.Equal(t, "http://10.0.1.1:8000/metrics", target.MetricsURL)
	assert.Equal(t, 30*time.Second, target.ScrapeInterval)
	assert.Equal(t, 15*time.Second, target.ScrapeTimeout)
	assert.Equal(t, "prod", target.Labels["env"])
	assert.Equal(t, TargetStatusUnknown, target.GetStatus())
}

func TestNewScrapeTargetFromConfig_Defaults(t *testing.T) {
	exp := exporter.NewMetricsExporter()

	cfg := &TargetConfig{
		WorkloadUID: "test-uid",
		MetricsURL:  "http://localhost/metrics",
		// No interval/timeout specified
	}

	target := NewScrapeTargetFromConfig(cfg, exp)

	assert.Equal(t, 15*time.Second, target.ScrapeInterval)
	assert.Equal(t, 10*time.Second, target.ScrapeTimeout)
}

func TestScrapeTarget_DoScrape(t *testing.T) {
	// Create a mock server
	metricsResponse := `
# TYPE test_metric counter
test_metric 123

# TYPE test_gauge gauge
test_gauge 45.6
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/metrics", r.URL.Path)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(metricsResponse))
	}))
	defer server.Close()

	exp := exporter.NewMetricsExporter()
	target := NewScrapeTargetFromConfig(&TargetConfig{
		WorkloadUID: "test",
		MetricsURL:  server.URL + "/metrics",
	}, exp)
	target.ctx = context.Background()

	metrics, err := target.doScrape()
	require.NoError(t, err)
	assert.Len(t, metrics, 2)
}

func TestScrapeTarget_DoScrape_EmptyURL(t *testing.T) {
	exp := exporter.NewMetricsExporter()
	target := NewScrapeTargetFromConfig(&TargetConfig{
		WorkloadUID: "test",
		MetricsURL:  "",
	}, exp)
	target.ctx = context.Background()

	metrics, err := target.doScrape()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty metrics URL")
	assert.Nil(t, metrics)
}

func TestScrapeTarget_DoScrape_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	exp := exporter.NewMetricsExporter()
	target := NewScrapeTargetFromConfig(&TargetConfig{
		WorkloadUID: "test",
		MetricsURL:  server.URL + "/metrics",
	}, exp)
	target.ctx = context.Background()

	metrics, err := target.doScrape()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 500")
	assert.Nil(t, metrics)
}

func TestScrapeTarget_GetStats(t *testing.T) {
	exp := exporter.NewMetricsExporter()
	target := NewScrapeTargetFromConfig(&TargetConfig{
		WorkloadUID: "test-uid",
		Framework:   "vllm",
		MetricsURL:  "http://localhost/metrics",
	}, exp)

	stats := target.GetStats()
	assert.Equal(t, "test-uid", stats.WorkloadUID)
	assert.Equal(t, "vllm", stats.Framework)
	assert.Equal(t, TargetStatusUnknown, stats.Status)
	assert.Zero(t, stats.ScrapeCount)
	assert.Zero(t, stats.ErrorCount)
}

func TestScrapeTarget_StatusTransitions(t *testing.T) {
	exp := exporter.NewMetricsExporter()
	target := NewScrapeTargetFromConfig(&TargetConfig{
		WorkloadUID: "test",
		MetricsURL:  "http://localhost/metrics",
	}, exp)

	// Initial state
	assert.Equal(t, TargetStatusUnknown, target.GetStatus())

	// Simulate successful scrape
	target.mu.Lock()
	target.consecutiveErrs = 0
	target.updateStatus()
	target.mu.Unlock()
	assert.Equal(t, TargetStatusHealthy, target.GetStatus())

	// Simulate 1 error
	target.mu.Lock()
	target.consecutiveErrs = 1
	target.updateStatus()
	target.mu.Unlock()
	assert.Equal(t, TargetStatusUnhealthy, target.GetStatus())

	// Simulate 3+ errors
	target.mu.Lock()
	target.consecutiveErrs = 3
	target.updateStatus()
	target.mu.Unlock()
	assert.Equal(t, TargetStatusDown, target.GetStatus())

	// Back to healthy
	target.mu.Lock()
	target.consecutiveErrs = 0
	target.updateStatus()
	target.mu.Unlock()
	assert.Equal(t, TargetStatusHealthy, target.GetStatus())
	assert.True(t, target.IsHealthy())
}
