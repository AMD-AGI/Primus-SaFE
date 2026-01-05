package collector

import (
	"context"
	"fmt"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CollectorFactory is a function that creates a collector
type CollectorFactory func(name string, config *CollectorConfig, k8sClient client.Client) (Collector, error)

// Manager manages multiple collectors
type Manager struct {
	collectors map[string]Collector
	factories  map[GatewayType]CollectorFactory
	k8sClient  client.Client
	mu         sync.RWMutex
}

// NewManager creates a new collector manager
func NewManager(k8sClient client.Client) *Manager {
	m := &Manager{
		collectors: make(map[string]Collector),
		factories:  make(map[GatewayType]CollectorFactory),
		k8sClient:  k8sClient,
	}

	return m
}

// RegisterFactory registers a collector factory for a gateway type
func (m *Manager) RegisterFactory(gatewayType GatewayType, factory CollectorFactory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.factories[gatewayType] = factory
}

// AddCollector adds a collector based on configuration
func (m *Manager) AddCollector(config *CollectorConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	factory, ok := m.factories[config.Type]
	if !ok {
		return fmt.Errorf("no factory registered for gateway type: %s", config.Type)
	}

	name := fmt.Sprintf("%s-%s", config.Type, config.Namespace)
	collector, err := factory(name, config, m.k8sClient)
	if err != nil {
		return fmt.Errorf("failed to create collector: %w", err)
	}

	m.collectors[name] = collector
	return nil
}

// CollectAll collects metrics from all collectors
func (m *Manager) CollectAll(ctx context.Context) ([]model.RawTrafficMetric, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allMetrics []model.RawTrafficMetric
	var errors []error

	for name, collector := range m.collectors {
		metrics, err := collector.Collect(ctx)
		if err != nil {
			errors = append(errors, fmt.Errorf("collector %s: %w", name, err))
			continue
		}
		allMetrics = append(allMetrics, metrics...)
	}

	if len(errors) > 0 && len(allMetrics) == 0 {
		return nil, fmt.Errorf("all collectors failed: %v", errors)
	}

	return allMetrics, nil
}

// HealthCheck checks health of all collectors
func (m *Manager) HealthCheck(ctx context.Context) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for name, collector := range m.collectors {
		results[name] = collector.HealthCheck(ctx)
	}

	return results
}

// ListCollectors returns all registered collectors
func (m *Manager) ListCollectors() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.collectors))
	for name := range m.collectors {
		names = append(names, name)
	}
	return names
}
