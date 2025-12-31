package aiclient

import (
	"context"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
)

// DegradationHandler handles graceful degradation when AI agents are unavailable
type DegradationHandler struct {
	mu       sync.RWMutex
	handlers map[string]FallbackHandler
}

// FallbackHandler is a function that provides fallback behavior for a topic
type FallbackHandler func(ctx context.Context, originalError error) (*aitopics.Response, error)

// NewDegradationHandler creates a new degradation handler
func NewDegradationHandler() *DegradationHandler {
	return &DegradationHandler{
		handlers: make(map[string]FallbackHandler),
	}
}

// RegisterFallback registers a fallback handler for a topic
func (d *DegradationHandler) RegisterFallback(topic string, handler FallbackHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[topic] = handler
}

// Handle attempts to handle the error gracefully
func (d *DegradationHandler) Handle(ctx context.Context, topic string, originalError error) (*aitopics.Response, error) {
	d.mu.RLock()
	handler, exists := d.handlers[topic]
	d.mu.RUnlock()

	if exists {
		return handler(ctx, originalError)
	}

	// Default fallback: return a degradation response
	return d.defaultFallback(topic, originalError)
}

// defaultFallback provides a default fallback response
func (d *DegradationHandler) defaultFallback(topic string, originalError error) (*aitopics.Response, error) {
	// Return an error response indicating degradation
	return &aitopics.Response{
		Status:  aitopics.StatusError,
		Code:    aitopics.CodeAgentUnavailable,
		Message: "AI features unavailable, using fallback behavior",
	}, ErrDegradationApplied
}

// DegradationConfig contains configuration for degradation behavior
type DegradationConfig struct {
	// Enabled controls whether degradation is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`

	// LogLevel controls the log level for degradation events
	LogLevel string `json:"log_level" yaml:"log_level"`

	// EmitMetrics controls whether to emit metrics for degradation events
	EmitMetrics bool `json:"emit_metrics" yaml:"emit_metrics"`

	// Fallbacks maps topics to fallback behavior
	Fallbacks map[string]string `json:"fallbacks" yaml:"fallbacks"`
}

// DefaultDegradationConfig returns default degradation configuration
func DefaultDegradationConfig() *DegradationConfig {
	return &DegradationConfig{
		Enabled:     true,
		LogLevel:    "warn",
		EmitMetrics: true,
		Fallbacks:   make(map[string]string),
	}
}

// Predefined fallback handlers for common topics

// EmptyResultFallback returns an empty result (no suggestions, no groups, etc.)
func EmptyResultFallback(topic string) FallbackHandler {
	return func(ctx context.Context, originalError error) (*aitopics.Response, error) {
		var payload interface{}

		switch topic {
		case aitopics.TopicAlertAdvisorAggregateWorkloads:
			payload = aitopics.AggregateWorkloadsOutput{
				Groups:    []aitopics.ComponentGroup{},
				Ungrouped: []string{},
				Stats: aitopics.AggregateStats{
					TotalWorkloads:   0,
					GroupedWorkloads: 0,
					TotalGroups:      0,
				},
			}
		case aitopics.TopicAlertAdvisorGenerateSuggestions:
			payload = aitopics.GenerateSuggestionsOutput{
				Suggestions: []aitopics.AlertSuggestion{},
			}
		case aitopics.TopicAlertHandlerAnalyze:
			payload = aitopics.AnalyzeAlertOutput{
				Analysis: aitopics.AlertAnalysis{
					Recommendations: []aitopics.Recommendation{},
				},
			}
		default:
			// Generic empty response
			payload = map[string]interface{}{}
		}

		resp, _ := aitopics.NewSuccessResponse("", payload)
		resp.Message = "Degraded response - AI unavailable"
		return resp, nil
	}
}

// SkipFallback simply skips the AI step without error
func SkipFallback() FallbackHandler {
	return func(ctx context.Context, originalError error) (*aitopics.Response, error) {
		return nil, nil // Return nil, nil to indicate "skip"
	}
}

// ErrorFallback returns the original error
func ErrorFallback() FallbackHandler {
	return func(ctx context.Context, originalError error) (*aitopics.Response, error) {
		return nil, originalError
	}
}

// DegradationMode represents different degradation modes
type DegradationMode int

const (
	// DegradationModeSkip skips the AI step entirely
	DegradationModeSkip DegradationMode = iota

	// DegradationModeEmpty returns empty results
	DegradationModeEmpty

	// DegradationModeError propagates the error
	DegradationModeError

	// DegradationModeCache returns cached results (if available)
	DegradationModeCache
)

// GetDegradationMode returns the appropriate degradation mode for a topic
func GetDegradationMode(topic string) DegradationMode {
	switch topic {
	case aitopics.TopicAlertAdvisorAggregateWorkloads:
		// Aggregation can be skipped - use rule-based fallback
		return DegradationModeSkip
	case aitopics.TopicAlertAdvisorGenerateSuggestions:
		// Suggestions can return empty - user can try later
		return DegradationModeEmpty
	case aitopics.TopicAlertHandlerAnalyze:
		// Analysis can return empty - show "analysis unavailable"
		return DegradationModeEmpty
	case aitopics.TopicScanIdentifyComponent:
		// Identification can be skipped - use pattern matching
		return DegradationModeSkip
	default:
		return DegradationModeError
	}
}
