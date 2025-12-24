package detection

import (
	"sync"
)

// FrameworkLayerInfo holds layer information for a framework
type FrameworkLayerInfo struct {
	Layer    string // wrapper, orchestration, runtime, inference
	Priority int    // Layer priority for winner selection
}

// FrameworkLayerResolver resolves framework layer from configuration
type FrameworkLayerResolver struct {
	configManager *FrameworkConfigManager
	cache         map[string]*FrameworkLayerInfo
	mu            sync.RWMutex
}

// NewFrameworkLayerResolver creates a new resolver
func NewFrameworkLayerResolver(configManager *FrameworkConfigManager) *FrameworkLayerResolver {
	return &FrameworkLayerResolver{
		configManager: configManager,
		cache:         make(map[string]*FrameworkLayerInfo),
	}
}

// GetLayerInfo returns complete layer information for a framework
func (r *FrameworkLayerResolver) GetLayerInfo(frameworkName string) *FrameworkLayerInfo {
	r.mu.RLock()
	if info, ok := r.cache[frameworkName]; ok {
		r.mu.RUnlock()
		return info
	}
	r.mu.RUnlock()

	// Load from config
	patterns := r.configManager.GetFramework(frameworkName)
	if patterns == nil {
		return &FrameworkLayerInfo{Layer: FrameworkLayerRuntime, Priority: 1}
	}

	info := &FrameworkLayerInfo{
		Layer:    patterns.GetLayer(),
		Priority: patterns.GetLayerPriority(),
	}

	// Cache it
	r.mu.Lock()
	r.cache[frameworkName] = info
	r.mu.Unlock()

	return info
}

// GetLayer returns the layer for a framework
func (r *FrameworkLayerResolver) GetLayer(frameworkName string) string {
	return r.GetLayerInfo(frameworkName).Layer
}

// GetPriority returns the layer priority for a framework
func (r *FrameworkLayerResolver) GetPriority(frameworkName string) int {
	return r.GetLayerInfo(frameworkName).Priority
}

// IsWrapper checks if framework is wrapper layer
func (r *FrameworkLayerResolver) IsWrapper(frameworkName string) bool {
	return r.GetLayer(frameworkName) == FrameworkLayerWrapper
}

// IsOrchestration checks if framework is orchestration layer
func (r *FrameworkLayerResolver) IsOrchestration(frameworkName string) bool {
	return r.GetLayer(frameworkName) == FrameworkLayerOrchestration
}

// IsRuntime checks if framework is runtime layer
func (r *FrameworkLayerResolver) IsRuntime(frameworkName string) bool {
	return r.GetLayer(frameworkName) == FrameworkLayerRuntime
}

// IsInference checks if framework is inference layer
func (r *FrameworkLayerResolver) IsInference(frameworkName string) bool {
	return r.GetLayer(frameworkName) == FrameworkLayerInference
}

// AreConflicting checks if two frameworks conflict (same layer)
func (r *FrameworkLayerResolver) AreConflicting(fw1, fw2 string) bool {
	return r.GetLayer(fw1) == r.GetLayer(fw2)
}

// RefreshCache clears the layer cache
func (r *FrameworkLayerResolver) RefreshCache() {
	r.mu.Lock()
	r.cache = make(map[string]*FrameworkLayerInfo)
	r.mu.Unlock()
}

// GetLayerForFrameworkWithFallback returns the layer for a framework
// First checks the evidence layer, then falls back to config lookup
func (r *FrameworkLayerResolver) GetLayerForFrameworkWithFallback(framework, evidenceLayer string) string {
	if evidenceLayer != "" {
		return evidenceLayer
	}
	return r.GetLayer(framework)
}

