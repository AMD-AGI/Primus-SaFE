// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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

// TestDefaultTraceOptions tests DefaultTraceOptions function
func TestDefaultTraceOptions(t *testing.T) {
	opts := DefaultTraceOptions()

	assert.Equal(t, TraceModeErrorOnly, opts.Mode)
	assert.Equal(t, 0.1, opts.SamplingRatio)
	assert.Equal(t, 1.0, opts.ErrorSamplingRatio)
}

// TestGetTraceOptions tests GetTraceOptions function
func TestGetTraceOptions(t *testing.T) {
	// Set traceOptions
	expectedOpts := TraceOptions{
		Mode:               TraceModeAlways,
		SamplingRatio:      0.5,
		ErrorSamplingRatio: 0.8,
	}
	traceOptions = expectedOpts

	opts := GetTraceOptions()

	assert.Equal(t, expectedOpts.Mode, opts.Mode)
	assert.Equal(t, expectedOpts.SamplingRatio, opts.SamplingRatio)
	assert.Equal(t, expectedOpts.ErrorSamplingRatio, opts.ErrorSamplingRatio)
}

// TestIsErrorOnlyMode tests IsErrorOnlyMode function
func TestIsErrorOnlyMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     TraceMode
		expected bool
	}{
		{
			name:     "error_only mode",
			mode:     TraceModeErrorOnly,
			expected: true,
		},
		{
			name:     "always mode",
			mode:     TraceModeAlways,
			expected: false,
		},
		{
			name:     "empty mode (defaults to error_only)",
			mode:     "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceOptions = TraceOptions{Mode: tt.mode}
			result := IsErrorOnlyMode()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTraceOptionsFromConfig tests TraceOptionsFromConfig function
func TestTraceOptionsFromConfig(t *testing.T) {
	tests := []struct {
		name               string
		mode               string
		samplingRatio      float64
		errorSamplingRatio float64
		expectedMode       TraceMode
		expectedSampling   float64
		expectedError      float64
	}{
		{
			name:               "always mode",
			mode:               "always",
			samplingRatio:      0.5,
			errorSamplingRatio: 0.8,
			expectedMode:       TraceModeAlways,
			expectedSampling:   0.5,
			expectedError:      0.8,
		},
		{
			name:               "error_only mode",
			mode:               "error_only",
			samplingRatio:      0.3,
			errorSamplingRatio: 0.9,
			expectedMode:       TraceModeErrorOnly,
			expectedSampling:   0.3,
			expectedError:      0.9,
		},
		{
			name:               "unknown mode defaults to error_only",
			mode:               "unknown",
			samplingRatio:      0.5,
			errorSamplingRatio: 0.5,
			expectedMode:       TraceModeErrorOnly,
			expectedSampling:   0.5,
			expectedError:      0.5,
		},
		{
			name:               "empty mode defaults to error_only",
			mode:               "",
			samplingRatio:      0.5,
			errorSamplingRatio: 0.5,
			expectedMode:       TraceModeErrorOnly,
			expectedSampling:   0.5,
			expectedError:      0.5,
		},
		{
			name:               "zero ratios",
			mode:               "always",
			samplingRatio:      0.0,
			errorSamplingRatio: 0.0,
			expectedMode:       TraceModeAlways,
			expectedSampling:   0.0,
			expectedError:      0.0,
		},
		{
			name:               "full ratios",
			mode:               "always",
			samplingRatio:      1.0,
			errorSamplingRatio: 1.0,
			expectedMode:       TraceModeAlways,
			expectedSampling:   1.0,
			expectedError:      1.0,
		},
		{
			name:               "negative sampling ratio uses default",
			mode:               "always",
			samplingRatio:      -0.5,
			errorSamplingRatio: 0.5,
			expectedMode:       TraceModeAlways,
			expectedSampling:   0.1, // default
			expectedError:      0.5,
		},
		{
			name:               "sampling ratio > 1 uses default",
			mode:               "always",
			samplingRatio:      1.5,
			errorSamplingRatio: 0.5,
			expectedMode:       TraceModeAlways,
			expectedSampling:   0.1, // default
			expectedError:      0.5,
		},
		{
			name:               "negative error sampling ratio uses default",
			mode:               "error_only",
			samplingRatio:      0.5,
			errorSamplingRatio: -0.5,
			expectedMode:       TraceModeErrorOnly,
			expectedSampling:   0.5,
			expectedError:      1.0, // default
		},
		{
			name:               "error sampling ratio > 1 uses default",
			mode:               "error_only",
			samplingRatio:      0.5,
			errorSamplingRatio: 1.5,
			expectedMode:       TraceModeErrorOnly,
			expectedSampling:   0.5,
			expectedError:      1.0, // default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := TraceOptionsFromConfig(tt.mode, tt.samplingRatio, tt.errorSamplingRatio)

			assert.Equal(t, tt.expectedMode, opts.Mode)
			assert.Equal(t, tt.expectedSampling, opts.SamplingRatio)
			assert.Equal(t, tt.expectedError, opts.ErrorSamplingRatio)
		})
	}
}

// TestTraceMode_Constants tests TraceMode constants
func TestTraceMode_Constants(t *testing.T) {
	assert.Equal(t, TraceMode("error_only"), TraceModeErrorOnly)
	assert.Equal(t, TraceMode("always"), TraceModeAlways)
}

// TestTraceOptions_Integration tests TraceOptions with different configurations
func TestTraceOptions_Integration(t *testing.T) {
	tests := []struct {
		name string
		opts TraceOptions
	}{
		{
			name: "default options",
			opts: DefaultTraceOptions(),
		},
		{
			name: "custom always mode",
			opts: TraceOptions{
				Mode:               TraceModeAlways,
				SamplingRatio:      0.25,
				ErrorSamplingRatio: 0.75,
			},
		},
		{
			name: "custom error_only mode",
			opts: TraceOptions{
				Mode:               TraceModeErrorOnly,
				SamplingRatio:      0.1,
				ErrorSamplingRatio: 0.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set options and verify
			traceOptions = tt.opts

			retrieved := GetTraceOptions()
			assert.Equal(t, tt.opts.Mode, retrieved.Mode)
			assert.Equal(t, tt.opts.SamplingRatio, retrieved.SamplingRatio)
			assert.Equal(t, tt.opts.ErrorSamplingRatio, retrieved.ErrorSamplingRatio)

			// Test IsErrorOnlyMode
			expectedErrorOnly := tt.opts.Mode == TraceModeErrorOnly || tt.opts.Mode == ""
			assert.Equal(t, expectedErrorOnly, IsErrorOnlyMode())
		})
	}
}

// TestTraceOptionsFromConfig_EdgeCases tests edge cases
func TestTraceOptionsFromConfig_EdgeCases(t *testing.T) {
	tests := []struct {
		name               string
		mode               string
		samplingRatio      float64
		errorSamplingRatio float64
	}{
		{
			name:               "boundary 0",
			mode:               "always",
			samplingRatio:      0.0,
			errorSamplingRatio: 0.0,
		},
		{
			name:               "boundary 1",
			mode:               "always",
			samplingRatio:      1.0,
			errorSamplingRatio: 1.0,
		},
		{
			name:               "very small ratio",
			mode:               "always",
			samplingRatio:      0.001,
			errorSamplingRatio: 0.001,
		},
		{
			name:               "near 1 ratio",
			mode:               "always",
			samplingRatio:      0.999,
			errorSamplingRatio: 0.999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := TraceOptionsFromConfig(tt.mode, tt.samplingRatio, tt.errorSamplingRatio)
			assert.NotPanics(t, func() {
				_ = opts.Mode
				_ = opts.SamplingRatio
				_ = opts.ErrorSamplingRatio
			})
		})
	}
}

// Benchmark tests for new functions
func BenchmarkDefaultTraceOptions(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DefaultTraceOptions()
	}
}

func BenchmarkGetTraceOptions(b *testing.B) {
	traceOptions = DefaultTraceOptions()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetTraceOptions()
	}
}

func BenchmarkIsErrorOnlyMode(b *testing.B) {
	traceOptions = TraceOptions{Mode: TraceModeErrorOnly}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsErrorOnlyMode()
	}
}

func BenchmarkTraceOptionsFromConfig(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TraceOptionsFromConfig("always", 0.5, 0.8)
	}
}

