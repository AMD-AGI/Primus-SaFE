package scraper

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/exporter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/transformer"
	dto "github.com/prometheus/client_model/go"
)

// TargetConfig contains configuration for a scrape target
// This is used to avoid circular imports between scraper and task packages
type TargetConfig struct {
	WorkloadUID    string
	Framework      string
	Namespace      string
	PodName        string
	PodIP          string
	MetricsURL     string
	Labels         map[string]string
	ScrapeInterval time.Duration
	ScrapeTimeout  time.Duration
}

// TargetStatus represents the health status of a scrape target
type TargetStatus string

const (
	TargetStatusUnknown   TargetStatus = "unknown"
	TargetStatusHealthy   TargetStatus = "healthy"
	TargetStatusUnhealthy TargetStatus = "unhealthy"
	TargetStatusDown      TargetStatus = "down"
)

// ScrapeTarget represents a single scrape target with its configuration and state
type ScrapeTarget struct {
	// Configuration from task
	WorkloadUID string
	Framework   string
	Namespace   string
	PodName     string
	PodIP       string
	MetricsURL  string
	Labels      map[string]string

	// Scrape configuration
	ScrapeInterval time.Duration
	ScrapeTimeout  time.Duration

	// Dependencies
	client      *http.Client
	parser      *MetricsParser
	transformer transformer.MetricsTransformer
	metricsExp  *exporter.MetricsExporter

	// State
	mu              sync.RWMutex
	status          TargetStatus
	lastScrapeTime  time.Time
	lastScrapeDur   time.Duration
	lastError       error
	scrapeCount     int64
	errorCount      int64
	consecutiveErrs int
	lastMetrics     []*dto.MetricFamily
	lastTransformed []*dto.MetricFamily

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// NewScrapeTargetFromConfig creates a new scrape target from config
func NewScrapeTargetFromConfig(cfg *TargetConfig, metricsExp *exporter.MetricsExporter) *ScrapeTarget {
	timeout := cfg.ScrapeTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	interval := cfg.ScrapeInterval
	if interval == 0 {
		interval = 15 * time.Second
	}

	// Build workload labels for metric enrichment
	workloadLabels := transformer.BuildWorkloadLabels(cfg.WorkloadUID, cfg.Namespace, cfg.PodName, "")
	for k, v := range cfg.Labels {
		workloadLabels[k] = v
	}

	return &ScrapeTarget{
		WorkloadUID:    cfg.WorkloadUID,
		Framework:      cfg.Framework,
		Namespace:      cfg.Namespace,
		PodName:        cfg.PodName,
		PodIP:          cfg.PodIP,
		MetricsURL:     cfg.MetricsURL,
		Labels:         workloadLabels,
		ScrapeInterval: interval,
		ScrapeTimeout:  timeout,
		client: &http.Client{
			Timeout: timeout,
		},
		parser:      NewMetricsParser(),
		transformer: transformer.DefaultRegistry.GetOrCreate(cfg.Framework),
		metricsExp:  metricsExp,
		status:      TargetStatusUnknown,
	}
}

// Start begins the scrape loop for this target
func (t *ScrapeTarget) Start(parentCtx context.Context) {
	t.ctx, t.cancel = context.WithCancel(parentCtx)

	log.Infof("Starting scrape target %s (url=%s, interval=%v)", t.WorkloadUID, t.MetricsURL, t.ScrapeInterval)

	go t.scrapeLoop()
}

// Stop stops the scrape loop
func (t *ScrapeTarget) Stop() {
	if t.cancel != nil {
		log.Infof("Stopping scrape target %s", t.WorkloadUID)
		t.cancel()
	}
}

// scrapeLoop is the main scraping loop
func (t *ScrapeTarget) scrapeLoop() {
	// Initial scrape
	t.scrape()

	ticker := time.NewTicker(t.ScrapeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			log.Debugf("Scrape loop stopped for %s", t.WorkloadUID)
			return
		case <-ticker.C:
			t.scrape()
		}
	}
}

// scrape performs a single scrape operation
func (t *ScrapeTarget) scrape() {
	startTime := time.Now()

	metrics, err := t.doScrape()

	duration := time.Since(startTime)

	t.mu.Lock()
	t.lastScrapeTime = startTime
	t.lastScrapeDur = duration
	t.scrapeCount++

	if err != nil {
		t.lastError = err
		t.errorCount++
		t.consecutiveErrs++
		t.updateStatus()
		t.mu.Unlock()

		log.Warnf("Scrape failed for %s: %v (consecutive errors: %d)", t.WorkloadUID, err, t.consecutiveErrs)
		exporter.ScrapeErrorsTotal.WithLabelValues("scrape_failed").Inc()
	} else {
		t.lastError = nil
		t.consecutiveErrs = 0
		t.lastMetrics = metrics

		// Transform metrics using the framework-specific transformer
		transformedMetrics, transformErr := t.transformer.Transform(metrics, t.Labels)
		if transformErr != nil {
			log.Warnf("Transform failed for %s: %v", t.WorkloadUID, transformErr)
			exporter.ScrapeErrorsTotal.WithLabelValues("transform_failed").Inc()
		} else {
			t.lastTransformed = transformedMetrics
		}

		t.updateStatus()
		t.mu.Unlock()

		// Update metrics in exporter with transformed metrics for /metrics endpoint
		if len(transformedMetrics) > 0 {
			t.metricsExp.UpdateWorkloadMetrics(t.WorkloadUID, t.Framework, t.Labels, transformedMetrics)
		}

		log.Debugf("Scrape succeeded for %s: %d raw, %d transformed metric families in %v",
			t.WorkloadUID, len(metrics), len(transformedMetrics), duration)
	}

	// Record metrics
	exporter.ScrapeTotal.Inc()
	exporter.ScrapeLatencySeconds.Observe(duration.Seconds())
}

// doScrape performs the actual HTTP request and parsing
func (t *ScrapeTarget) doScrape() ([]*dto.MetricFamily, error) {
	if t.MetricsURL == "" {
		return nil, fmt.Errorf("empty metrics URL")
	}

	req, err := http.NewRequestWithContext(t.ctx, http.MethodGet, t.MetricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "text/plain;version=0.0.4;q=1,*/*;q=0.1")
	req.Header.Set("User-Agent", "inference-metrics-exporter/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	metrics, err := t.parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse metrics: %w", err)
	}

	return metrics, nil
}

// updateStatus updates the target status based on current state
// Must be called with lock held
func (t *ScrapeTarget) updateStatus() {
	if t.consecutiveErrs == 0 {
		t.status = TargetStatusHealthy
	} else if t.consecutiveErrs < 3 {
		t.status = TargetStatusUnhealthy
	} else {
		t.status = TargetStatusDown
	}
}

// GetStatus returns the current target status
func (t *ScrapeTarget) GetStatus() TargetStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

// GetStats returns statistics for this target
func (t *ScrapeTarget) GetStats() TargetStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var lastErr string
	if t.lastError != nil {
		lastErr = t.lastError.Error()
	}

	return TargetStats{
		WorkloadUID:       t.WorkloadUID,
		Framework:         t.Framework,
		MetricsURL:        t.MetricsURL,
		Status:            t.status,
		LastScrapeTime:    t.lastScrapeTime,
		LastScrapeDuration: t.lastScrapeDur,
		LastError:         lastErr,
		ScrapeCount:       t.scrapeCount,
		ErrorCount:        t.errorCount,
		ConsecutiveErrors: t.consecutiveErrs,
		MetricFamilies:    len(t.lastMetrics),
	}
}

// TargetStats contains statistics for a scrape target
type TargetStats struct {
	WorkloadUID        string        `json:"workload_uid"`
	Framework          string        `json:"framework"`
	MetricsURL         string        `json:"metrics_url"`
	Status             TargetStatus  `json:"status"`
	LastScrapeTime     time.Time     `json:"last_scrape_time"`
	LastScrapeDuration time.Duration `json:"last_scrape_duration"`
	LastError          string        `json:"last_error,omitempty"`
	ScrapeCount        int64         `json:"scrape_count"`
	ErrorCount         int64         `json:"error_count"`
	ConsecutiveErrors  int           `json:"consecutive_errors"`
	MetricFamilies     int           `json:"metric_families"`
}

// IsHealthy returns true if the target is healthy
func (t *ScrapeTarget) IsHealthy() bool {
	return t.GetStatus() == TargetStatusHealthy
}

// GetTransformedMetrics returns the last transformed metrics
func (t *ScrapeTarget) GetTransformedMetrics() []*dto.MetricFamily {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastTransformed
}

// GetRawMetrics returns the last raw (untransformed) metrics
func (t *ScrapeTarget) GetRawMetrics() []*dto.MetricFamily {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastMetrics
}

