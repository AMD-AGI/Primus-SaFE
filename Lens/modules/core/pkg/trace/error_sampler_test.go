package trace

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// mockSpanExporter is a mock implementation of sdktrace.SpanExporter for testing
type mockSpanExporter struct {
	mu            sync.Mutex
	exportedSpans []sdktrace.ReadOnlySpan
	exportCount   int32
	shutdownCalls int32
	exportErr     error
}

func newMockSpanExporter() *mockSpanExporter {
	return &mockSpanExporter{
		exportedSpans: make([]sdktrace.ReadOnlySpan, 0),
	}
}

func (m *mockSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	atomic.AddInt32(&m.exportCount, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exportedSpans = append(m.exportedSpans, spans...)
	return m.exportErr
}

func (m *mockSpanExporter) Shutdown(ctx context.Context) error {
	atomic.AddInt32(&m.shutdownCalls, 1)
	return nil
}

func (m *mockSpanExporter) GetExportedSpans() []sdktrace.ReadOnlySpan {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]sdktrace.ReadOnlySpan, len(m.exportedSpans))
	copy(result, m.exportedSpans)
	return result
}

func (m *mockSpanExporter) GetExportCount() int32 {
	return atomic.LoadInt32(&m.exportCount)
}

func (m *mockSpanExporter) GetShutdownCalls() int32 {
	return atomic.LoadInt32(&m.shutdownCalls)
}

func (m *mockSpanExporter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exportedSpans = make([]sdktrace.ReadOnlySpan, 0)
	atomic.StoreInt32(&m.exportCount, 0)
	atomic.StoreInt32(&m.shutdownCalls, 0)
}

// TestTraceMode tests TraceMode constants
func TestTraceMode(t *testing.T) {
	assert.Equal(t, TraceMode("error_only"), TraceModeErrorOnly)
	assert.Equal(t, TraceMode("always"), TraceModeAlways)
}

// TestDefaultTraceOptions_ErrorSampler tests DefaultTraceOptions function
func TestDefaultTraceOptions_ErrorSampler(t *testing.T) {
	opts := DefaultTraceOptions()

	assert.Equal(t, TraceModeErrorOnly, opts.Mode)
	assert.Equal(t, 0.1, opts.SamplingRatio)
	assert.Equal(t, 1.0, opts.ErrorSamplingRatio)
}

// TestTraceOptions_Struct tests TraceOptions struct
func TestTraceOptions_Struct(t *testing.T) {
	tests := []struct {
		name               string
		mode               TraceMode
		samplingRatio      float64
		errorSamplingRatio float64
	}{
		{
			name:               "error_only mode",
			mode:               TraceModeErrorOnly,
			samplingRatio:      0.1,
			errorSamplingRatio: 1.0,
		},
		{
			name:               "always mode",
			mode:               TraceModeAlways,
			samplingRatio:      0.5,
			errorSamplingRatio: 0.8,
		},
		{
			name:               "zero ratios",
			mode:               TraceModeAlways,
			samplingRatio:      0.0,
			errorSamplingRatio: 0.0,
		},
		{
			name:               "full ratios",
			mode:               TraceModeErrorOnly,
			samplingRatio:      1.0,
			errorSamplingRatio: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := TraceOptions{
				Mode:               tt.mode,
				SamplingRatio:      tt.samplingRatio,
				ErrorSamplingRatio: tt.errorSamplingRatio,
			}

			assert.Equal(t, tt.mode, opts.Mode)
			assert.Equal(t, tt.samplingRatio, opts.SamplingRatio)
			assert.Equal(t, tt.errorSamplingRatio, opts.ErrorSamplingRatio)
		})
	}
}

// TestNewErrorOnlySpanProcessor tests NewErrorOnlySpanProcessor function
func TestNewErrorOnlySpanProcessor(t *testing.T) {
	exporter := newMockSpanExporter()
	processor := NewErrorOnlySpanProcessor(exporter, 1.0)

	require.NotNil(t, processor)
	assert.NotNil(t, processor.exporter)
	assert.Equal(t, 1.0, processor.errorSamplingRatio)
	assert.NotNil(t, processor.traces)
	assert.NotNil(t, processor.rand)
}

// TestErrorOnlySpanProcessor_OnStart tests OnStart method
func TestErrorOnlySpanProcessor_OnStart(t *testing.T) {
	exporter := newMockSpanExporter()
	processor := NewErrorOnlySpanProcessor(exporter, 1.0)

	// OnStart should not panic and do nothing
	assert.NotPanics(t, func() {
		processor.OnStart(context.Background(), nil)
	})
}

// setupTestTracerProvider creates a tracer provider with ErrorOnlySpanProcessor for testing
func setupTestTracerProvider(exporter *mockSpanExporter, errorSamplingRatio float64) *sdktrace.TracerProvider {
	processor := NewErrorOnlySpanProcessor(exporter, errorSamplingRatio)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(processor),
	)
	return tp
}

// TestErrorOnlySpanProcessor_Integration_NoError tests that spans without errors are not exported
func TestErrorOnlySpanProcessor_Integration_NoError(t *testing.T) {
	exporter := newMockSpanExporter()
	tp := setupTestTracerProvider(exporter, 1.0)
	defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("test")

	// Create a span without error
	_, span := tracer.Start(context.Background(), "test-span-no-error")
	span.SetStatus(codes.Ok, "")
	span.End()

	// Force flush
	tp.ForceFlush(context.Background())

	// Should NOT export spans without error
	assert.Equal(t, int32(0), exporter.GetExportCount())
}

// TestErrorOnlySpanProcessor_Integration_WithError tests that spans with errors are exported
func TestErrorOnlySpanProcessor_Integration_WithError(t *testing.T) {
	exporter := newMockSpanExporter()
	tp := setupTestTracerProvider(exporter, 1.0)
	defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("test")

	// Create a span with error
	_, span := tracer.Start(context.Background(), "test-span-with-error")
	span.SetStatus(codes.Error, "test error")
	span.End()

	// Force flush
	tp.ForceFlush(context.Background())

	// Should export spans with error
	assert.Equal(t, int32(1), exporter.GetExportCount())
	spans := exporter.GetExportedSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, "test-span-with-error", spans[0].Name())
}

// TestErrorOnlySpanProcessor_Integration_ChildSpanWithError tests that traces with child errors are exported
func TestErrorOnlySpanProcessor_Integration_ChildSpanWithError(t *testing.T) {
	exporter := newMockSpanExporter()
	tp := setupTestTracerProvider(exporter, 1.0)
	defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("test")

	// Create parent span
	ctx, parentSpan := tracer.Start(context.Background(), "parent-span")

	// Create child span with error
	_, childSpan := tracer.Start(ctx, "child-span-with-error")
	childSpan.SetStatus(codes.Error, "child error")
	childSpan.End()

	// End parent span (no error on parent)
	parentSpan.SetStatus(codes.Ok, "")
	parentSpan.End()

	// Force flush
	tp.ForceFlush(context.Background())

	// Should export because child had error
	assert.GreaterOrEqual(t, exporter.GetExportCount(), int32(1))
}

// TestErrorOnlySpanProcessor_ShouldSample tests shouldSample method
func TestErrorOnlySpanProcessor_ShouldSample(t *testing.T) {
	tests := []struct {
		name               string
		errorSamplingRatio float64
		iterations         int
		expectSome         bool
		expectAll          bool
	}{
		{
			name:               "100% sampling",
			errorSamplingRatio: 1.0,
			iterations:         100,
			expectSome:         true,
			expectAll:          true,
		},
		{
			name:               "0% sampling",
			errorSamplingRatio: 0.0,
			iterations:         100,
			expectSome:         false,
			expectAll:          false,
		},
		{
			name:               "50% sampling",
			errorSamplingRatio: 0.5,
			iterations:         1000,
			expectSome:         true,
			expectAll:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := newMockSpanExporter()
			processor := NewErrorOnlySpanProcessor(exporter, tt.errorSamplingRatio)

			sampledCount := 0
			for i := 0; i < tt.iterations; i++ {
				if processor.shouldSample() {
					sampledCount++
				}
			}

			if tt.expectAll {
				assert.Equal(t, tt.iterations, sampledCount)
			} else if tt.expectSome {
				assert.Greater(t, sampledCount, 0)
				assert.Less(t, sampledCount, tt.iterations)
			} else {
				assert.Equal(t, 0, sampledCount)
			}
		})
	}
}

// TestErrorOnlySpanProcessor_Shutdown tests Shutdown method
func TestErrorOnlySpanProcessor_Shutdown(t *testing.T) {
	exporter := newMockSpanExporter()
	processor := NewErrorOnlySpanProcessor(exporter, 1.0)

	// Shutdown
	err := processor.Shutdown(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, int32(1), exporter.GetShutdownCalls())
}

// TestErrorOnlySpanProcessor_ForceFlush tests ForceFlush method
func TestErrorOnlySpanProcessor_ForceFlush(t *testing.T) {
	exporter := newMockSpanExporter()
	processor := NewErrorOnlySpanProcessor(exporter, 1.0)

	err := processor.ForceFlush(context.Background())
	assert.NoError(t, err)
}

// TestErrorOnlySpanProcessor_Concurrent tests concurrent access
func TestErrorOnlySpanProcessor_Concurrent(t *testing.T) {
	exporter := newMockSpanExporter()
	tp := setupTestTracerProvider(exporter, 1.0)
	defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("test")

	var wg sync.WaitGroup
	numGoroutines := 10
	spansPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < spansPerGoroutine; j++ {
				_, span := tracer.Start(context.Background(), "concurrent-span")
				if j%2 == 0 {
					span.SetStatus(codes.Error, "test error")
				} else {
					span.SetStatus(codes.Ok, "")
				}
				span.End()
			}
		}(i)
	}

	wg.Wait()

	// Should not panic
	assert.NotPanics(t, func() {
		tp.ForceFlush(context.Background())
	})
}

// TestNewSampledSpanProcessor tests NewSampledSpanProcessor function
func TestNewSampledSpanProcessor(t *testing.T) {
	exporter := newMockSpanExporter()
	baseProcessor := NewErrorOnlySpanProcessor(exporter, 1.0)
	processor := NewSampledSpanProcessor(baseProcessor, 0.5)

	require.NotNil(t, processor)
	assert.NotNil(t, processor.processor)
	assert.Equal(t, 0.5, processor.samplingRatio)
	assert.NotNil(t, processor.rand)
}

// TestSampledSpanProcessor_OnStart tests OnStart method
func TestSampledSpanProcessor_OnStart(t *testing.T) {
	exporter := newMockSpanExporter()
	baseProcessor := NewErrorOnlySpanProcessor(exporter, 1.0)
	processor := NewSampledSpanProcessor(baseProcessor, 0.5)

	assert.NotPanics(t, func() {
		processor.OnStart(context.Background(), nil)
	})
}

// TestSampledSpanProcessor_Shutdown tests Shutdown method
func TestSampledSpanProcessor_Shutdown(t *testing.T) {
	exporter := newMockSpanExporter()
	baseProcessor := NewErrorOnlySpanProcessor(exporter, 1.0)
	processor := NewSampledSpanProcessor(baseProcessor, 0.5)

	err := processor.Shutdown(context.Background())
	assert.NoError(t, err)
}

// TestSampledSpanProcessor_ForceFlush tests ForceFlush method
func TestSampledSpanProcessor_ForceFlush(t *testing.T) {
	exporter := newMockSpanExporter()
	baseProcessor := NewErrorOnlySpanProcessor(exporter, 1.0)
	processor := NewSampledSpanProcessor(baseProcessor, 0.5)

	err := processor.ForceFlush(context.Background())
	assert.NoError(t, err)
}

// TestErrorOnlySpanProcessor_Integration_MultipleTraces tests handling multiple traces
func TestErrorOnlySpanProcessor_Integration_MultipleTraces(t *testing.T) {
	exporter := newMockSpanExporter()
	tp := setupTestTracerProvider(exporter, 1.0)
	defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("test")

	// Trace 1: has error
	_, span1 := tracer.Start(context.Background(), "trace1-error")
	span1.SetStatus(codes.Error, "error 1")
	span1.End()

	// Trace 2: no error
	_, span2 := tracer.Start(context.Background(), "trace2-ok")
	span2.SetStatus(codes.Ok, "")
	span2.End()

	// Trace 3: has error
	_, span3 := tracer.Start(context.Background(), "trace3-error")
	span3.SetStatus(codes.Error, "error 3")
	span3.End()

	// Force flush
	tp.ForceFlush(context.Background())

	// Should export 2 traces with errors
	assert.Equal(t, int32(2), exporter.GetExportCount())
	spans := exporter.GetExportedSpans()
	assert.Len(t, spans, 2)
}

// TestErrorOnlySpanProcessor_ZeroSamplingRatio tests with 0 sampling ratio
func TestErrorOnlySpanProcessor_ZeroSamplingRatio(t *testing.T) {
	exporter := newMockSpanExporter()
	tp := setupTestTracerProvider(exporter, 0.0) // 0% error sampling
	defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("test")

	// Create multiple spans with errors
	for i := 0; i < 10; i++ {
		_, span := tracer.Start(context.Background(), "test-span")
		span.SetStatus(codes.Error, "error")
		span.End()
	}

	// Force flush
	tp.ForceFlush(context.Background())

	// Should NOT export any spans with 0% sampling
	assert.Equal(t, int32(0), exporter.GetExportCount())
}

// Benchmark tests
func BenchmarkNewErrorOnlySpanProcessor(b *testing.B) {
	exporter := newMockSpanExporter()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewErrorOnlySpanProcessor(exporter, 1.0)
	}
}

func BenchmarkErrorOnlySpanProcessor_ShouldSample(b *testing.B) {
	exporter := newMockSpanExporter()
	processor := NewErrorOnlySpanProcessor(exporter, 0.5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = processor.shouldSample()
	}
}

func BenchmarkErrorOnlySpanProcessor_Integration(b *testing.B) {
	exporter := newMockSpanExporter()
	tp := setupTestTracerProvider(exporter, 1.0)
	defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := tracer.Start(context.Background(), "benchmark-span")
		if i%2 == 0 {
			span.SetStatus(codes.Error, "error")
		}
		span.End()
	}
}
