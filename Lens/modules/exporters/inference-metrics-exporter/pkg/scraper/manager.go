// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package scraper

import (
	"context"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/exporter"
)

// ScrapeManager manages all scrape targets
type ScrapeManager struct {
	metricsExporter *exporter.MetricsExporter

	// Active targets
	targets map[string]*ScrapeTarget // workloadUID -> target
	mu      sync.RWMutex

	// Lifecycle
	ctx context.Context
}

// NewScrapeManager creates a new scrape manager
func NewScrapeManager(exp *exporter.MetricsExporter) *ScrapeManager {
	return &ScrapeManager{
		metricsExporter: exp,
		targets:         make(map[string]*ScrapeTarget),
	}
}

// Start initializes the scrape manager
func (m *ScrapeManager) Start(ctx context.Context) error {
	m.ctx = ctx
	log.Info("Scrape manager started")
	return nil
}

// Stop stops all scrape targets
func (m *ScrapeManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for uid, target := range m.targets {
		log.Debugf("Stopping target %s", uid)
		target.Stop()
	}
	m.targets = make(map[string]*ScrapeTarget)

	log.Info("Scrape manager stopped")
	return nil
}

// AddTarget adds a new scrape target using TargetConfig
func (m *ScrapeManager) AddTarget(cfg *TargetConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already exists
	if existing, ok := m.targets[cfg.WorkloadUID]; ok {
		log.Debugf("Target %s already exists, updating", cfg.WorkloadUID)
		existing.Stop()
	}

	// Create new target
	target := NewScrapeTargetFromConfig(cfg, m.metricsExporter)
	m.targets[cfg.WorkloadUID] = target

	// Start scraping
	target.Start(m.ctx)

	exporter.UpdateScrapeTargets(len(m.targets))
	log.Infof("Added scrape target %s (total: %d)", cfg.WorkloadUID, len(m.targets))

	return nil
}

// RemoveTarget removes a scrape target
func (m *ScrapeManager) RemoveTarget(workloadUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if target, ok := m.targets[workloadUID]; ok {
		target.Stop()
		delete(m.targets, workloadUID)
		exporter.UpdateScrapeTargets(len(m.targets))
		log.Infof("Removed scrape target %s (total: %d)", workloadUID, len(m.targets))
	}

	return nil
}

// UpdateTarget updates an existing target's configuration
func (m *ScrapeManager) UpdateTarget(cfg *TargetConfig) error {
	// For now, just remove and re-add
	m.RemoveTarget(cfg.WorkloadUID)
	return m.AddTarget(cfg)
}

// GetTarget returns a specific target
func (m *ScrapeManager) GetTarget(workloadUID string) (*ScrapeTarget, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	target, ok := m.targets[workloadUID]
	return target, ok
}

// GetTargets returns all targets
func (m *ScrapeManager) GetTargets() []*ScrapeTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	targets := make([]*ScrapeTarget, 0, len(m.targets))
	for _, t := range m.targets {
		targets = append(targets, t)
	}
	return targets
}

// GetTargetCount returns the number of active targets
func (m *ScrapeManager) GetTargetCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.targets)
}

// GetStats returns statistics for all targets
func (m *ScrapeManager) GetStats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := ManagerStats{
		TotalTargets: len(m.targets),
		ByStatus:     make(map[TargetStatus]int),
		ByFramework:  make(map[string]int),
	}

	for _, t := range m.targets {
		status := t.GetStatus()
		stats.ByStatus[status]++
		stats.ByFramework[t.Framework]++
	}

	return stats
}

// ManagerStats contains statistics for the scrape manager
type ManagerStats struct {
	TotalTargets int                   `json:"total_targets"`
	ByStatus     map[TargetStatus]int  `json:"by_status"`
	ByFramework  map[string]int        `json:"by_framework"`
}

// GetHealthyCount returns the number of healthy targets
func (m *ScrapeManager) GetHealthyCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, t := range m.targets {
		if t.IsHealthy() {
			count++
		}
	}
	return count
}

// GetUnhealthyTargets returns all unhealthy targets
func (m *ScrapeManager) GetUnhealthyTargets() []*ScrapeTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	targets := make([]*ScrapeTarget, 0)
	for _, t := range m.targets {
		if !t.IsHealthy() {
			targets = append(targets, t)
		}
	}
	return targets
}

