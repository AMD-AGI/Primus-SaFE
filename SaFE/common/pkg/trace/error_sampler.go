// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package trace

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TraceOptions contains configuration options for tracing
type TraceOptions struct {
	// Mode specifies the tracing mode: "error_only" or "all"
	Mode string
}

// DefaultTraceOptions returns the default trace options
func DefaultTraceOptions() TraceOptions {
	return TraceOptions{
		Mode: "error_only",
	}
}

// ErrorOnlySpanProcessor is a SpanProcessor that only exports spans with errors
// It buffers spans and only exports them when the root span has an error status
type ErrorOnlySpanProcessor struct {
	exporter           sdktrace.SpanExporter
	errorSamplingRatio float64

	mu     sync.Mutex
	traces map[string][]sdktrace.ReadOnlySpan // traceID -> spans
	rand   *rand.Rand
}

// NewErrorOnlySpanProcessor creates a new ErrorOnlySpanProcessor
func NewErrorOnlySpanProcessor(exporter sdktrace.SpanExporter, errorSamplingRatio float64) *ErrorOnlySpanProcessor {
	return &ErrorOnlySpanProcessor{
		exporter:           exporter,
		errorSamplingRatio: errorSamplingRatio,
		traces:             make(map[string][]sdktrace.ReadOnlySpan),
		rand:               rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// OnStart is called when a span is started
func (p *ErrorOnlySpanProcessor) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	// Nothing to do on start
}

// OnEnd is called when a span is ended
func (p *ErrorOnlySpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	if !s.SpanContext().IsSampled() {
		return
	}

	traceID := s.SpanContext().TraceID().String()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Add span to the trace buffer
	p.traces[traceID] = append(p.traces[traceID], s)

	// Check if this span has an error
	hasError := s.Status().Code == codes.Error

	// Check if this is a root span (no parent or remote parent)
	isRootSpan := !s.Parent().IsValid() || s.Parent().IsRemote()

	// If it's a root span, we can decide whether to export the trace
	if isRootSpan {
		spans := p.traces[traceID]
		delete(p.traces, traceID)

		// Check if any span in the trace has an error
		traceHasError := hasError
		if !traceHasError {
			for _, span := range spans {
				if span.Status().Code == codes.Error {
					traceHasError = true
					break
				}
			}
		}

		// Only export if trace has error and passes sampling
		if traceHasError && p.shouldSample() {
			ctx := context.Background()
			if err := p.exporter.ExportSpans(ctx, spans); err != nil {
				// Log error but don't fail
			}
		}
	}
}

// shouldSample returns true if this error trace should be sampled
func (p *ErrorOnlySpanProcessor) shouldSample() bool {
	if p.errorSamplingRatio >= 1.0 {
		return true
	}
	if p.errorSamplingRatio <= 0.0 {
		return false
	}
	return p.rand.Float64() < p.errorSamplingRatio
}

// Shutdown shuts down the processor
func (p *ErrorOnlySpanProcessor) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Export any remaining error traces
	for traceID, spans := range p.traces {
		// Check if any span has error
		hasError := false
		for _, span := range spans {
			if span.Status().Code == codes.Error {
				hasError = true
				break
			}
		}
		if hasError && p.shouldSample() {
			_ = p.exporter.ExportSpans(ctx, spans)
		}
		delete(p.traces, traceID)
	}

	return p.exporter.Shutdown(ctx)
}

// ForceFlush forces flush of pending error spans without shutting down the exporter
func (p *ErrorOnlySpanProcessor) ForceFlush(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Export any traces that contain errors
	for traceID, spans := range p.traces {
		hasError := false
		for _, span := range spans {
			if span.Status().Code == codes.Error {
				hasError = true
				break
			}
		}
		if hasError && p.shouldSample() {
			if err := p.exporter.ExportSpans(ctx, spans); err != nil {
				return err
			}
		}
		delete(p.traces, traceID)
	}
	return nil
}
