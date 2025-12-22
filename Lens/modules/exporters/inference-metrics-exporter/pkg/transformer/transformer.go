package transformer

import (
	"fmt"
	"strings"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	dto "github.com/prometheus/client_model/go"
)

// MetricsTransformer transforms metrics from framework-specific names to unified names
type MetricsTransformer interface {
	// Transform transforms metrics according to the configuration
	Transform(metrics []*dto.MetricFamily, workloadLabels map[string]string) ([]*dto.MetricFamily, error)

	// GetFramework returns the framework name
	GetFramework() string

	// UpdateConfig updates the transformer configuration
	UpdateConfig(config *FrameworkMetricsConfig) error
}

// BaseTransformer provides common transformation logic
type BaseTransformer struct {
	config     *FrameworkMetricsConfig
	mappingIdx map[string]*MetricMapping // source name -> mapping
	mu         sync.RWMutex
}

// NewBaseTransformer creates a new base transformer
func NewBaseTransformer(config *FrameworkMetricsConfig) *BaseTransformer {
	t := &BaseTransformer{
		config:     config,
		mappingIdx: make(map[string]*MetricMapping),
	}
	t.buildMappingIndex()
	return t
}

// buildMappingIndex builds the mapping index for fast lookup
func (t *BaseTransformer) buildMappingIndex() {
	t.mappingIdx = make(map[string]*MetricMapping)
	for i := range t.config.Mappings {
		m := &t.config.Mappings[i]
		// Extract source metric name (remove framework prefix if present)
		sourceName := m.Source
		if idx := strings.Index(sourceName, ":"); idx >= 0 {
			sourceName = sourceName[idx+1:]
		}
		t.mappingIdx[sourceName] = m
	}
}

// GetFramework returns the framework name
func (t *BaseTransformer) GetFramework() string {
	return t.config.Framework
}

// UpdateConfig updates the transformer configuration
func (t *BaseTransformer) UpdateConfig(config *FrameworkMetricsConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.config = config
	t.buildMappingIndex()
	log.Infof("Updated transformer config for framework %s", config.Framework)
	return nil
}

// Transform transforms metrics according to the configuration
func (t *BaseTransformer) Transform(metrics []*dto.MetricFamily, workloadLabels map[string]string) ([]*dto.MetricFamily, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*dto.MetricFamily, 0, len(metrics))

	for _, mf := range metrics {
		if mf.Name == nil {
			continue
		}

		// Check if we have a mapping for this metric
		mapping, found := t.mappingIdx[*mf.Name]
		if !found {
			// No mapping, keep the original metric but add labels
			transformed := t.cloneWithLabels(mf, workloadLabels)
			result = append(result, transformed)
			continue
		}

		// Transform the metric
		transformed := t.transformMetric(mf, mapping, workloadLabels)
		if transformed != nil {
			result = append(result, transformed)
		}
	}

	return result, nil
}

// transformMetric transforms a single metric family
func (t *BaseTransformer) transformMetric(mf *dto.MetricFamily, mapping *MetricMapping, workloadLabels map[string]string) *dto.MetricFamily {
	// Clone the metric family
	newMF := &dto.MetricFamily{
		Name:   strPtr(mapping.Target),
		Type:   mf.Type,
		Metric: make([]*dto.Metric, 0, len(mf.Metric)),
	}

	if mapping.Help != "" {
		newMF.Help = strPtr(mapping.Help)
	} else if mf.Help != nil {
		newMF.Help = mf.Help
	}

	// Transform each metric
	for _, m := range mf.Metric {
		newMetric := t.transformSingleMetric(m, mapping, workloadLabels)
		if newMetric != nil {
			newMF.Metric = append(newMF.Metric, newMetric)
		}
	}

	return newMF
}

// transformSingleMetric transforms a single metric within a family
func (t *BaseTransformer) transformSingleMetric(m *dto.Metric, mapping *MetricMapping, workloadLabels map[string]string) *dto.Metric {
	newMetric := &dto.Metric{
		Label:       t.mergeLabels(m.Label, workloadLabels),
		TimestampMs: m.TimestampMs,
	}

	// Copy and optionally transform the value
	switch {
	case m.Counter != nil:
		val := m.Counter.GetValue()
		val = t.applyTransform(val, mapping.Transform)
		newMetric.Counter = &dto.Counter{Value: float64Ptr(val)}

	case m.Gauge != nil:
		val := m.Gauge.GetValue()
		val = t.applyTransform(val, mapping.Transform)
		newMetric.Gauge = &dto.Gauge{Value: float64Ptr(val)}

	case m.Histogram != nil:
		newMetric.Histogram = t.transformHistogram(m.Histogram, mapping.Transform)

	case m.Summary != nil:
		newMetric.Summary = t.transformSummary(m.Summary, mapping.Transform)

	case m.Untyped != nil:
		val := m.Untyped.GetValue()
		val = t.applyTransform(val, mapping.Transform)
		newMetric.Untyped = &dto.Untyped{Value: float64Ptr(val)}

	default:
		return nil
	}

	return newMetric
}

// transformHistogram transforms a histogram with optional value transformation
func (t *BaseTransformer) transformHistogram(h *dto.Histogram, transform string) *dto.Histogram {
	if h == nil {
		return nil
	}

	newH := &dto.Histogram{
		SampleCount: h.SampleCount,
		SampleSum:   h.SampleSum,
	}

	// Transform sum if needed
	if transform != "" && h.SampleSum != nil {
		val := t.applyTransform(*h.SampleSum, transform)
		newH.SampleSum = float64Ptr(val)
	}

	// Clone buckets with transformed bounds if needed
	if len(h.Bucket) > 0 {
		newH.Bucket = make([]*dto.Bucket, len(h.Bucket))
		for i, b := range h.Bucket {
			newBucket := &dto.Bucket{
				CumulativeCount: b.CumulativeCount,
				UpperBound:      b.UpperBound,
			}
			// Transform bucket bounds for unit conversion
			if transform != "" && b.UpperBound != nil {
				val := t.applyTransform(*b.UpperBound, transform)
				newBucket.UpperBound = float64Ptr(val)
			}
			newH.Bucket[i] = newBucket
		}
	}

	return newH
}

// transformSummary transforms a summary with optional value transformation
func (t *BaseTransformer) transformSummary(s *dto.Summary, transform string) *dto.Summary {
	if s == nil {
		return nil
	}

	newS := &dto.Summary{
		SampleCount: s.SampleCount,
		SampleSum:   s.SampleSum,
	}

	// Transform sum if needed
	if transform != "" && s.SampleSum != nil {
		val := t.applyTransform(*s.SampleSum, transform)
		newS.SampleSum = float64Ptr(val)
	}

	// Clone quantiles with transformed values
	if len(s.Quantile) > 0 {
		newS.Quantile = make([]*dto.Quantile, len(s.Quantile))
		for i, q := range s.Quantile {
			newQ := &dto.Quantile{
				Quantile: q.Quantile,
				Value:    q.Value,
			}
			if transform != "" && q.Value != nil {
				val := t.applyTransform(*q.Value, transform)
				newQ.Value = float64Ptr(val)
			}
			newS.Quantile[i] = newQ
		}
	}

	return newS
}

// applyTransform applies a value transformation
func (t *BaseTransformer) applyTransform(val float64, transform string) float64 {
	switch transform {
	case "divide_by_100":
		return val / 100.0
	case "divide_by_1000":
		return val / 1000.0
	case "multiply_by_1000":
		return val * 1000.0
	case "microseconds_to_seconds":
		return val / 1000000.0
	case "milliseconds_to_seconds":
		return val / 1000.0
	default:
		return val
	}
}

// mergeLabels merges metric labels with workload labels
func (t *BaseTransformer) mergeLabels(metricLabels []*dto.LabelPair, workloadLabels map[string]string) []*dto.LabelPair {
	// Start with workload labels
	labelMap := make(map[string]string)

	// Add always-add labels from config
	for k, v := range t.config.LabelsAlwaysAdd {
		labelMap[k] = v
	}

	// Add workload labels (can override always-add)
	for k, v := range workloadLabels {
		labelMap[k] = v
	}

	// Add existing metric labels (highest priority)
	for _, lp := range metricLabels {
		if lp.Name != nil && lp.Value != nil {
			labelMap[*lp.Name] = *lp.Value
		}
	}

	// Convert back to label pairs
	result := make([]*dto.LabelPair, 0, len(labelMap))
	for k, v := range labelMap {
		result = append(result, &dto.LabelPair{
			Name:  strPtr(k),
			Value: strPtr(v),
		})
	}

	return result
}

// cloneWithLabels clones a metric family and adds workload labels
func (t *BaseTransformer) cloneWithLabels(mf *dto.MetricFamily, workloadLabels map[string]string) *dto.MetricFamily {
	newMF := &dto.MetricFamily{
		Name:   mf.Name,
		Help:   mf.Help,
		Type:   mf.Type,
		Metric: make([]*dto.Metric, len(mf.Metric)),
	}

	for i, m := range mf.Metric {
		newMF.Metric[i] = &dto.Metric{
			Label:       t.mergeLabels(m.Label, workloadLabels),
			Counter:     m.Counter,
			Gauge:       m.Gauge,
			Histogram:   m.Histogram,
			Summary:     m.Summary,
			Untyped:     m.Untyped,
			TimestampMs: m.TimestampMs,
		}
	}

	return newMF
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

// Ensure interface compliance
var _ MetricsTransformer = (*BaseTransformer)(nil)

// TransformerRegistry manages transformers for different frameworks
type TransformerRegistry struct {
	transformers map[string]MetricsTransformer
	mu           sync.RWMutex
}

// NewTransformerRegistry creates a new transformer registry
func NewTransformerRegistry() *TransformerRegistry {
	return &TransformerRegistry{
		transformers: make(map[string]MetricsTransformer),
	}
}

// Register registers a transformer for a framework
func (r *TransformerRegistry) Register(framework string, transformer MetricsTransformer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transformers[framework] = transformer
}

// Get returns the transformer for a framework
func (r *TransformerRegistry) Get(framework string) (MetricsTransformer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.transformers[framework]
	return t, ok
}

// GetOrCreate returns an existing transformer or creates a new one with default config
func (r *TransformerRegistry) GetOrCreate(framework string) MetricsTransformer {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t, ok := r.transformers[framework]; ok {
		return t
	}

	// Create with default config
	config := GetDefaultConfig(framework)
	if config == nil {
		// Create a passthrough transformer for unknown frameworks
		config = &FrameworkMetricsConfig{
			Framework: framework,
			LabelsAlwaysAdd: map[string]string{
				"framework":      framework,
				"framework_type": "inference",
			},
		}
	}

	t := NewBaseTransformer(config)
	r.transformers[framework] = t
	return t
}

// UpdateConfig updates the configuration for a framework
func (r *TransformerRegistry) UpdateConfig(framework string, config *FrameworkMetricsConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t, ok := r.transformers[framework]; ok {
		return t.UpdateConfig(config)
	}

	// Create new transformer with the config
	r.transformers[framework] = NewBaseTransformer(config)
	return nil
}

// Frameworks returns the list of registered frameworks
func (r *TransformerRegistry) Frameworks() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	frameworks := make([]string, 0, len(r.transformers))
	for f := range r.transformers {
		frameworks = append(frameworks, f)
	}
	return frameworks
}

// DefaultRegistry is the global transformer registry
var DefaultRegistry = NewTransformerRegistry()

// InitDefaultTransformers initializes the default transformers
func InitDefaultTransformers() {
	DefaultRegistry.Register("vllm", NewBaseTransformer(DefaultVLLMConfig()))
	DefaultRegistry.Register("tgi", NewBaseTransformer(DefaultTGIConfig()))
	DefaultRegistry.Register("triton", NewBaseTransformer(DefaultTritonConfig()))
	log.Info("Initialized default metric transformers for vllm, tgi, triton")
}

// Transform is a convenience function to transform metrics using the default registry
func Transform(framework string, metrics []*dto.MetricFamily, labels map[string]string) ([]*dto.MetricFamily, error) {
	t := DefaultRegistry.GetOrCreate(framework)
	return t.Transform(metrics, labels)
}

// GetTransformer returns a transformer from the default registry
func GetTransformer(framework string) (MetricsTransformer, bool) {
	return DefaultRegistry.Get(framework)
}

// GetMappingInfo returns information about a metric mapping
func (t *BaseTransformer) GetMappingInfo(sourceName string) (*MetricMapping, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m, ok := t.mappingIdx[sourceName]
	return m, ok
}

// TransformResult contains the result of a transformation
type TransformResult struct {
	Framework         string `json:"framework"`
	SourceMetrics     int    `json:"source_metrics"`
	TransformedMetrics int    `json:"transformed_metrics"`
	MappedMetrics     int    `json:"mapped_metrics"`
	PassthroughMetrics int   `json:"passthrough_metrics"`
	Errors            []string `json:"errors,omitempty"`
}

// TransformWithStats transforms metrics and returns statistics
func (t *BaseTransformer) TransformWithStats(metrics []*dto.MetricFamily, labels map[string]string) (*TransformResult, []*dto.MetricFamily, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := &TransformResult{
		Framework:     t.config.Framework,
		SourceMetrics: len(metrics),
	}

	transformed := make([]*dto.MetricFamily, 0, len(metrics))

	for _, mf := range metrics {
		if mf.Name == nil {
			continue
		}

		mapping, found := t.mappingIdx[*mf.Name]
		if found {
			result.MappedMetrics++
			tm := t.transformMetric(mf, mapping, labels)
			if tm != nil {
				transformed = append(transformed, tm)
			}
		} else {
			result.PassthroughMetrics++
			tm := t.cloneWithLabels(mf, labels)
			transformed = append(transformed, tm)
		}
	}

	result.TransformedMetrics = len(transformed)
	return result, transformed, nil
}

// LabelInjector provides methods to inject labels into metrics
type LabelInjector struct{}

// NewLabelInjector creates a new label injector
func NewLabelInjector() *LabelInjector {
	return &LabelInjector{}
}

// InjectLabels adds labels to all metrics in a metric family
func (li *LabelInjector) InjectLabels(mf *dto.MetricFamily, labels map[string]string) *dto.MetricFamily {
	if mf == nil || len(labels) == 0 {
		return mf
	}

	newMF := &dto.MetricFamily{
		Name:   mf.Name,
		Help:   mf.Help,
		Type:   mf.Type,
		Metric: make([]*dto.Metric, len(mf.Metric)),
	}

	for i, m := range mf.Metric {
		newMetric := &dto.Metric{
			Counter:     m.Counter,
			Gauge:       m.Gauge,
			Histogram:   m.Histogram,
			Summary:     m.Summary,
			Untyped:     m.Untyped,
			TimestampMs: m.TimestampMs,
		}

		// Merge labels
		labelMap := make(map[string]string)
		for _, lp := range m.Label {
			if lp.Name != nil && lp.Value != nil {
				labelMap[*lp.Name] = *lp.Value
			}
		}
		for k, v := range labels {
			labelMap[k] = v
		}

		newMetric.Label = make([]*dto.LabelPair, 0, len(labelMap))
		for k, v := range labelMap {
			newMetric.Label = append(newMetric.Label, &dto.LabelPair{
				Name:  strPtr(k),
				Value: strPtr(v),
			})
		}

		newMF.Metric[i] = newMetric
	}

	return newMF
}

// BuildWorkloadLabels creates a standard set of workload labels
func BuildWorkloadLabels(workloadUID, namespace, podName, clusterName string) map[string]string {
	labels := map[string]string{
		"workload_uid": workloadUID,
	}
	if namespace != "" {
		labels["namespace"] = namespace
	}
	if podName != "" {
		labels["pod"] = podName
	}
	if clusterName != "" {
		labels["cluster"] = clusterName
	}
	return labels
}

// ValidateMapping validates a metric mapping configuration
func ValidateMapping(m *MetricMapping) error {
	if m.Source == "" {
		return fmt.Errorf("mapping source cannot be empty")
	}
	if m.Target == "" {
		return fmt.Errorf("mapping target cannot be empty")
	}
	validTypes := map[string]bool{
		"counter": true, "gauge": true, "histogram": true, "summary": true, "untyped": true,
	}
	if m.Type != "" && !validTypes[m.Type] {
		return fmt.Errorf("invalid metric type: %s", m.Type)
	}
	validTransforms := map[string]bool{
		"": true, "divide_by_100": true, "divide_by_1000": true,
		"multiply_by_1000": true, "microseconds_to_seconds": true, "milliseconds_to_seconds": true,
	}
	if !validTransforms[m.Transform] {
		return fmt.Errorf("invalid transform: %s", m.Transform)
	}
	return nil
}

