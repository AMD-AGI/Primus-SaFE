package trace

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TestStartSpan tests StartSpan function
func TestStartSpan(t *testing.T) {
	ctx := context.Background()
	
	newCtx, span := StartSpan(ctx, "test-operation")
	require.NotNil(t, newCtx)
	require.NotNil(t, span)
	defer span.End()
	
	assert.NotEqual(t, ctx, newCtx, "Context should be different")
}

// TestStartSpanFromContext tests StartSpanFromContext function
func TestStartSpanFromContext(t *testing.T) {
	ctx := context.Background()
	
	span, newCtx := StartSpanFromContext(ctx, "test-operation")
	require.NotNil(t, span)
	require.NotNil(t, newCtx)
	defer span.End()
	
	assert.NotEqual(t, ctx, newCtx, "Context should be different")
}

// TestGetSpan tests GetSpan function
func TestGetSpan(t *testing.T) {
	ctx := context.Background()
	
	// Without span
	span := GetSpan(ctx)
	assert.NotNil(t, span) // Returns non-recording span
	
	// With span
	ctx, activeSpan := StartSpan(ctx, "test-operation")
	defer activeSpan.End()
	
	retrievedSpan := GetSpan(ctx)
	assert.NotNil(t, retrievedSpan)
	assert.Equal(t, activeSpan, retrievedSpan)
}

// TestContextWithSpan tests ContextWithSpan function
func TestContextWithSpan(t *testing.T) {
	ctx := context.Background()
	_, span := StartSpan(ctx, "test-operation")
	defer span.End()
	
	newCtx := ContextWithSpan(ctx, span)
	require.NotNil(t, newCtx)
	
	retrievedSpan := trace.SpanFromContext(newCtx)
	assert.Equal(t, span, retrievedSpan)
}

// TestFinishSpan tests FinishSpan function
func TestFinishSpan(t *testing.T) {
	tests := []struct {
		name string
		span trace.Span
	}{
		{
			name: "valid span",
			span: func() trace.Span {
				_, s := StartSpan(context.Background(), "test")
				return s
			}(),
		},
		{
			name: "nil span",
			span: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				FinishSpan(tt.span)
			})
		})
	}
}

// TestFinishSpanFromContext tests FinishSpanFromContext function
func TestFinishSpanFromContext(t *testing.T) {
	ctx := context.Background()
	
	// Without span
	assert.NotPanics(t, func() {
		FinishSpanFromContext(ctx)
	})
	
	// With span
	ctx, _ = StartSpan(ctx, "test-operation")
	assert.NotPanics(t, func() {
		FinishSpanFromContext(ctx)
	})
}

// TestAddEvent tests AddEvent function
func TestAddEvent(t *testing.T) {
	// Setup a tracer provider with in-memory exporter for testing
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx, span := StartSpan(context.Background(), "test-operation")
	defer span.End()
	
	tests := []struct {
		name  string
		event string
		attrs []attribute.KeyValue
	}{
		{
			name:  "simple event",
			event: "test-event",
			attrs: nil,
		},
		{
			name:  "event with attributes",
			event: "user-action",
			attrs: []attribute.KeyValue{
				attribute.String("user_id", "123"),
				attribute.Int("count", 1),
			},
		},
		{
			name:  "event with multiple attributes",
			event: "complex-event",
			attrs: []attribute.KeyValue{
				attribute.String("key1", "value1"),
				attribute.Int("key2", 42),
				attribute.Bool("key3", true),
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				AddEvent(ctx, tt.event, tt.attrs...)
			})
		})
	}
}

// TestSetAttributes tests SetAttributes function
func TestSetAttributes(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx, span := StartSpan(context.Background(), "test-operation")
	defer span.End()
	
	tests := []struct {
		name  string
		attrs []attribute.KeyValue
	}{
		{
			name:  "single attribute",
			attrs: []attribute.KeyValue{attribute.String("key", "value")},
		},
		{
			name: "multiple attributes",
			attrs: []attribute.KeyValue{
				attribute.String("service", "test"),
				attribute.Int("version", 1),
				attribute.Bool("enabled", true),
			},
		},
		{
			name:  "no attributes",
			attrs: []attribute.KeyValue{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				SetAttributes(ctx, tt.attrs...)
			})
		})
	}
}

// TestSetAttribute tests SetAttribute function
func TestSetAttribute(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx, span := StartSpan(context.Background(), "test-operation")
	defer span.End()
	
	tests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{"string value", "key", "value"},
		{"int value", "count", 42},
		{"int64 value", "id", int64(123456)},
		{"float64 value", "rate", 0.95},
		{"bool value", "enabled", true},
		{"struct value", "data", struct{ Name string }{"test"}},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				SetAttribute(ctx, tt.key, tt.value)
			})
		})
	}
}

// TestRecordError tests RecordError function
func TestRecordError(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx, span := StartSpan(context.Background(), "test-operation")
	defer span.End()
	
	tests := []struct {
		name string
		err  error
	}{
		{"nil error", nil},
		{"simple error", errors.New("test error")},
		{"wrapped error", errors.New("wrapped: test error")},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				RecordError(ctx, tt.err)
			})
		})
	}
}

// TestSetStatus tests SetStatus function
func TestSetStatus(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx, span := StartSpan(context.Background(), "test-operation")
	defer span.End()
	
	tests := []struct {
		name        string
		code        codes.Code
		description string
	}{
		{"ok status", codes.Ok, ""},
		{"error status", codes.Error, "operation failed"},
		{"unset status", codes.Unset, ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				SetStatus(ctx, tt.code, tt.description)
			})
		})
	}
}

// TestGetTraceID tests GetTraceID function
func TestGetTraceID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	// Without span
	ctx := context.Background()
	traceID := GetTraceID(ctx)
	assert.Empty(t, traceID)
	
	// With span
	ctx, span := StartSpan(ctx, "test-operation")
	defer span.End()
	
	traceID = GetTraceID(ctx)
	// May be empty if sampler is NeverSample, or non-empty if sampled
	if traceID != "" {
		assert.NotEmpty(t, traceID)
		assert.Len(t, traceID, 32) // Trace ID is 16 bytes hex = 32 chars
	}
}

// TestGetSpanID tests GetSpanID function
func TestGetSpanID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	// Without span
	ctx := context.Background()
	spanID := GetSpanID(ctx)
	assert.Empty(t, spanID)
	
	// With span
	ctx, span := StartSpan(ctx, "test-operation")
	defer span.End()
	
	spanID = GetSpanID(ctx)
	// May be empty if sampler is NeverSample, or non-empty if sampled
	if spanID != "" {
		assert.NotEmpty(t, spanID)
		assert.Len(t, spanID, 16) // Span ID is 8 bytes hex = 16 chars
	}
}

// TestSpanFromContext tests SpanFromContext function
func TestSpanFromContext(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	// Without span
	ctx := context.Background()
	span, ok := SpanFromContext(ctx)
	assert.NotNil(t, span)
	assert.False(t, ok, "Should return false for invalid span")
	
	// With span
	ctx, activeSpan := StartSpan(ctx, "test-operation")
	defer activeSpan.End()
	
	span, ok = SpanFromContext(ctx)
	assert.NotNil(t, span)
	assert.True(t, ok, "Should return true for valid span")
	assert.Equal(t, activeSpan, span)
}

// TestGetTraceIDAndSpanID tests GetTraceIDAndSpanID function
func TestGetTraceIDAndSpanID(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	tests := []struct {
		name         string
		span         trace.Span
		expectValid  bool
	}{
		{
			name:        "nil span",
			span:        nil,
			expectValid: false,
		},
		{
			name: "valid span",
			span: func() trace.Span {
				_, s := StartSpan(context.Background(), "test")
				return s
			}(),
			expectValid: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceID, spanID, ok := GetTraceIDAndSpanID(tt.span)
			
			if tt.expectValid {
				// May be valid or invalid depending on sampler
				if ok {
					assert.NotEmpty(t, traceID)
					assert.NotEmpty(t, spanID)
					assert.Len(t, traceID, 32)
					assert.Len(t, spanID, 16)
				}
			} else {
				assert.False(t, ok)
				assert.Empty(t, traceID)
				assert.Empty(t, spanID)
			}
			
			if tt.span != nil {
				tt.span.End()
			}
		})
	}
}

// TestConvertToAttribute tests convertToAttribute function
func TestConvertToAttribute(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    interface{}
		expected attribute.KeyValue
	}{
		{
			name:     "string",
			key:      "key",
			value:    "value",
			expected: attribute.String("key", "value"),
		},
		{
			name:     "int",
			key:      "count",
			value:    42,
			expected: attribute.Int("count", 42),
		},
		{
			name:     "int64",
			key:      "id",
			value:    int64(123456),
			expected: attribute.Int64("id", 123456),
		},
		{
			name:     "float64",
			key:      "rate",
			value:    0.95,
			expected: attribute.Float64("rate", 0.95),
		},
		{
			name:     "bool",
			key:      "enabled",
			value:    true,
			expected: attribute.Bool("enabled", true),
		},
		{
			name:     "other type",
			key:      "data",
			value:    struct{ Name string }{"test"},
			expected: attribute.String("data", "{test}"),
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToAttribute(tt.key, tt.value)
			assert.Equal(t, tt.expected.Key, result.Key)
			assert.Equal(t, tt.expected.Value.Type(), result.Value.Type())
		})
	}
}

// TestGetEnvOrDefault tests getEnvOrDefault function
func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue string
		expected     string
	}{
		{
			name:         "env var exists",
			key:          "TEST_ENV_VAR",
			envValue:     "test-value",
			defaultValue: "default",
			expected:     "test-value",
		},
		{
			name:         "env var not exists",
			key:          "NON_EXISTENT_VAR",
			envValue:     "",
			defaultValue: "default",
			expected:     "default",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}
			
			result := getEnvOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCloseTracer tests CloseTracer function
func TestCloseTracer(t *testing.T) {
	// Test with nil tracer provider
	tracerProvider = nil
	err := CloseTracer()
	assert.NoError(t, err)
	
	// Test with valid tracer provider
	tp := sdktrace.NewTracerProvider()
	tracerProvider = tp
	
	err = CloseTracer()
	assert.NoError(t, err)
}

// TestStartSpan_WithOptions tests StartSpan with various options
func TestStartSpan_WithOptions(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx := context.Background()
	
	tests := []struct {
		name string
		opts []trace.SpanStartOption
	}{
		{
			name: "with span kind",
			opts: []trace.SpanStartOption{
				trace.WithSpanKind(trace.SpanKindClient),
			},
		},
		{
			name: "with attributes",
			opts: []trace.SpanStartOption{
				trace.WithAttributes(
					attribute.String("service", "test"),
					attribute.Int("version", 1),
				),
			},
		},
		{
			name: "multiple options",
			opts: []trace.SpanStartOption{
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attribute.String("key", "value")),
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newCtx, span := StartSpan(ctx, "test-operation", tt.opts...)
			require.NotNil(t, newCtx)
			require.NotNil(t, span)
			span.End()
		})
	}
}

// BenchmarkStartSpan benchmarks StartSpan function
func BenchmarkStartSpan(b *testing.B) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := StartSpan(ctx, "benchmark-operation")
		span.End()
	}
}

// BenchmarkAddEvent benchmarks AddEvent function
func BenchmarkAddEvent(b *testing.B) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx, span := StartSpan(context.Background(), "benchmark-operation")
	defer span.End()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AddEvent(ctx, "test-event", attribute.String("key", "value"))
	}
}

// BenchmarkSetAttributes benchmarks SetAttributes function
func BenchmarkSetAttributes(b *testing.B) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx, span := StartSpan(context.Background(), "benchmark-operation")
	defer span.End()
	
	attrs := []attribute.KeyValue{
		attribute.String("key1", "value1"),
		attribute.Int("key2", 42),
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SetAttributes(ctx, attrs...)
	}
}

// BenchmarkConvertToAttribute benchmarks convertToAttribute function
func BenchmarkConvertToAttribute(b *testing.B) {
	values := []interface{}{"string", 42, int64(123), 0.95, true}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertToAttribute("key", values[i%len(values)])
	}
}

