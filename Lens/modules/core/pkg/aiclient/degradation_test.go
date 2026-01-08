package aiclient

import (
	"context"
	"errors"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDegradationHandler(t *testing.T) {
	handler := NewDegradationHandler()
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.handlers)
}

func TestDegradationHandler_RegisterFallback(t *testing.T) {
	handler := NewDegradationHandler()
	topic := "test-topic"

	called := false
	fallback := func(ctx context.Context, originalError error) (*aitopics.Response, error) {
		called = true
		return &aitopics.Response{Status: aitopics.StatusSuccess}, nil
	}

	handler.RegisterFallback(topic, fallback)

	// Execute the handler
	resp, err := handler.Handle(context.Background(), topic, errors.New("test error"))
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, aitopics.StatusSuccess, resp.Status)
}

func TestDegradationHandler_Handle_WithFallback(t *testing.T) {
	handler := NewDegradationHandler()
	topic := "custom-topic"
	originalErr := errors.New("original error")

	// Register a custom fallback
	handler.RegisterFallback(topic, func(ctx context.Context, err error) (*aitopics.Response, error) {
		assert.Equal(t, originalErr, err)
		return &aitopics.Response{
			Status:  aitopics.StatusSuccess,
			Message: "custom fallback response",
		}, nil
	})

	resp, err := handler.Handle(context.Background(), topic, originalErr)
	assert.NoError(t, err)
	assert.Equal(t, aitopics.StatusSuccess, resp.Status)
	assert.Equal(t, "custom fallback response", resp.Message)
}

func TestDegradationHandler_Handle_DefaultFallback(t *testing.T) {
	handler := NewDegradationHandler()
	topic := "unknown-topic"

	resp, err := handler.Handle(context.Background(), topic, errors.New("some error"))
	assert.Equal(t, ErrDegradationApplied, err)
	assert.NotNil(t, resp)
	assert.Equal(t, aitopics.StatusError, resp.Status)
	assert.Equal(t, aitopics.CodeAgentUnavailable, resp.Code)
	assert.Contains(t, resp.Message, "fallback")
}

func TestDefaultDegradationConfig(t *testing.T) {
	cfg := DefaultDegradationConfig()
	assert.NotNil(t, cfg)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.True(t, cfg.EmitMetrics)
	assert.NotNil(t, cfg.Fallbacks)
}

func TestEmptyResultFallback(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		topic string
	}{
		{"AggregateWorkloads", aitopics.TopicAlertAdvisorAggregateWorkloads},
		{"GenerateSuggestions", aitopics.TopicAlertAdvisorGenerateSuggestions},
		{"AnalyzeAlert", aitopics.TopicAlertHandlerAnalyze},
		{"UnknownTopic", "unknown.topic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := EmptyResultFallback(tt.topic)
			assert.NotNil(t, handler)

			resp, err := handler(ctx, errors.New("test error"))
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Contains(t, resp.Message, "Degraded")
		})
	}
}

func TestEmptyResultFallback_AggregateWorkloads(t *testing.T) {
	handler := EmptyResultFallback(aitopics.TopicAlertAdvisorAggregateWorkloads)
	resp, err := handler(context.Background(), errors.New("test"))
	require.NoError(t, err)
	require.NotNil(t, resp)

	var output aitopics.AggregateWorkloadsOutput
	err = resp.UnmarshalPayload(&output)
	require.NoError(t, err)
	assert.Empty(t, output.Groups)
	assert.Empty(t, output.Ungrouped)
}

func TestEmptyResultFallback_GenerateSuggestions(t *testing.T) {
	handler := EmptyResultFallback(aitopics.TopicAlertAdvisorGenerateSuggestions)
	resp, err := handler(context.Background(), errors.New("test"))
	require.NoError(t, err)
	require.NotNil(t, resp)

	var output aitopics.GenerateSuggestionsOutput
	err = resp.UnmarshalPayload(&output)
	require.NoError(t, err)
	assert.Empty(t, output.Suggestions)
}

func TestEmptyResultFallback_AnalyzeAlert(t *testing.T) {
	handler := EmptyResultFallback(aitopics.TopicAlertHandlerAnalyze)
	resp, err := handler(context.Background(), errors.New("test"))
	require.NoError(t, err)
	require.NotNil(t, resp)

	var output aitopics.AnalyzeAlertOutput
	err = resp.UnmarshalPayload(&output)
	require.NoError(t, err)
	assert.Empty(t, output.Analysis.Recommendations)
}

func TestSkipFallback(t *testing.T) {
	handler := SkipFallback()
	resp, err := handler(context.Background(), errors.New("test error"))
	assert.Nil(t, resp)
	assert.Nil(t, err)
}

func TestErrorFallback(t *testing.T) {
	handler := ErrorFallback()
	originalErr := errors.New("original error")

	resp, err := handler(context.Background(), originalErr)
	assert.Nil(t, resp)
	assert.Equal(t, originalErr, err)
}

func TestGetDegradationMode(t *testing.T) {
	tests := []struct {
		topic string
		want  DegradationMode
	}{
		{aitopics.TopicAlertAdvisorAggregateWorkloads, DegradationModeSkip},
		{aitopics.TopicAlertAdvisorGenerateSuggestions, DegradationModeEmpty},
		{aitopics.TopicAlertHandlerAnalyze, DegradationModeEmpty},
		{aitopics.TopicScanIdentifyComponent, DegradationModeSkip},
		{"unknown.topic", DegradationModeError},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := GetDegradationMode(tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDegradationModeConstants(t *testing.T) {
	// Verify the constants are unique
	modes := []DegradationMode{
		DegradationModeSkip,
		DegradationModeEmpty,
		DegradationModeError,
		DegradationModeCache,
	}

	seen := make(map[DegradationMode]bool)
	for _, mode := range modes {
		assert.False(t, seen[mode], "duplicate mode: %d", mode)
		seen[mode] = true
	}
}

func TestDegradationHandler_Concurrency(t *testing.T) {
	handler := NewDegradationHandler()
	done := make(chan bool)

	// Register handlers concurrently
	for i := 0; i < 10; i++ {
		go func(idx int) {
			topic := "topic-" + string(rune('0'+idx))
			handler.RegisterFallback(topic, func(ctx context.Context, err error) (*aitopics.Response, error) {
				return &aitopics.Response{}, nil
			})
			done <- true
		}(i)
	}

	// Handle requests concurrently
	for i := 0; i < 10; i++ {
		go func(idx int) {
			topic := "topic-" + string(rune('0'+idx))
			_, _ = handler.Handle(context.Background(), topic, errors.New("test"))
			done <- true
		}(i)
	}

	// Wait for all
	for i := 0; i < 20; i++ {
		<-done
	}
}

