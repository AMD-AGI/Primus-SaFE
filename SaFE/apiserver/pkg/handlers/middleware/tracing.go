// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package middleware

import (
	"bytes"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// tracingSampleRateKey is used to store the sample rate in gin.Context
	tracingSampleRateKey = "tracing_sample_rate"
	// maxResponseBodySize is the maximum response body size to capture (4KB)
	maxResponseBodySize = 4096
)

// responseBodyWriter wraps gin.ResponseWriter to capture response body and inject headers
type responseBodyWriter struct {
	gin.ResponseWriter
	body           *bytes.Buffer
	traceId        string
	headerInjected bool
}

func (w *responseBodyWriter) WriteHeader(code int) {
	// Inject X-Trace-Id header before writing headers if status >= 400
	if !w.headerInjected && code >= 400 && w.traceId != "" {
		w.Header().Set("X-Trace-Id", w.traceId)
		w.headerInjected = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	// Capture response body (up to maxResponseBodySize)
	if w.body.Len() < maxResponseBodySize {
		remaining := maxResponseBodySize - w.body.Len()
		if len(b) <= remaining {
			w.body.Write(b)
		} else {
			w.body.Write(b[:remaining])
		}
	}
	return w.ResponseWriter.Write(b)
}

// WithTracingRate sets a custom tracing sample rate for specific endpoints
// Sample rate range: 0.0 - 1.0, where 1.0 means 100% sampling, 0.1 means 10% sampling
// Usage example: group.POST("wandb/batch", middleware.WithTracingRate(0.1), logs.ReceiveWandBBatch)
func WithTracingRate(rate float64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Ensure sample rate is within valid range [0.0, 1.0]
		if rate < 0.0 {
			rate = 0.0
		}
		if rate > 1.0 {
			rate = 1.0
		}
		c.Set(tracingSampleRateKey, rate)
		c.Next()
	}
}

// shouldSample determines whether to sample based on the sample rate
func shouldSample(sampleRate float64) bool {
	if sampleRate >= 1.0 {
		return true
	}
	if sampleRate <= 0.0 {
		return false
	}
	return rand.Float64() < sampleRate
}

// HandleTracing creates a tracing middleware that only records failed requests (status >= 400).
// This is an error-only tracing mode to reduce overhead and focus on problematic requests.
func HandleTracing() gin.HandlerFunc {
	return HandleTracingErrorOnly()
}

// HandleTracingErrorOnly creates a tracing middleware that only records failed requests.
// Spans are only created and exported when the HTTP status code is >= 400.
// The response body and traceId are captured for debugging purposes.
func HandleTracingErrorOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip tracing if not enabled
		if os.Getenv("OTEL_TRACING_ENABLE") != "true" {
			c.Next()
			return
		}

		// Record start time for duration calculation
		startTime := time.Now()

		ctx := c.Request.Context()

		// Extract trace context from HTTP headers
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, &httpHeaderCarrier{header: c.Request.Header})

		// Pre-create span to get trace ID (we'll only export it if there's an error)
		operationName := c.Request.Method + " " + c.Request.URL.Path
		tracer := otel.Tracer("primus-safe-apiserver")
		ctx, span := tracer.Start(ctx, operationName,
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithTimestamp(startTime),
		)
		defer span.End()

		// Get traceId for potential header injection
		traceId := span.SpanContext().TraceID().String()

		// Wrap response writer to capture response body and inject X-Trace-Id header
		bodyWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
			traceId:        traceId,
		}
		c.Writer = bodyWriter

		// Update context in request
		c.Request = c.Request.WithContext(ctx)

		// Execute the request
		c.Next()

		// Get status code
		statusCode := c.Writer.Status()

		// Only record span details if request failed (status >= 400)
		if statusCode < 400 {
			// For successful requests, just return (span.End() called by defer)
			return
		}

		// Calculate request duration
		duration := time.Since(startTime)

		// Set HTTP-related attributes
		span.SetAttributes(
			semconv.HTTPMethod(c.Request.Method),
			semconv.HTTPURL(c.Request.URL.String()),
			semconv.HTTPRoute(c.Request.URL.Path),
			semconv.HTTPTarget(c.Request.URL.Path),
			semconv.HTTPStatusCode(statusCode),
			attribute.String("component", "gin-http"),
			attribute.String("http.path", c.Request.URL.Path),
				attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
				attribute.Int64("http.duration_ns", duration.Nanoseconds()),
			attribute.String("trace.id", traceId),
		)

		// Capture response body for error debugging
		responseBody := bodyWriter.body.String()
		if responseBody != "" {
			// Truncate if too long for display
			if len(responseBody) > maxResponseBodySize {
				responseBody = responseBody[:maxResponseBodySize] + "...(truncated)"
			}
			span.SetAttributes(attribute.String("http.response.body", responseBody))
		}

		// Record error status
		span.SetStatus(codes.Error, "HTTP error: "+responseBody)

		// Add error details if available from gin context
		if len(c.Errors) > 0 {
			for i, err := range c.Errors {
				span.SetAttributes(attribute.String("gin.error."+strconv.Itoa(i), err.Error()))
			}
			span.RecordError(c.Errors.Last())
		}
	}
}

// httpHeaderCarrier implements propagation.TextMapCarrier for HTTP headers
type httpHeaderCarrier struct {
	header http.Header
}

func (h *httpHeaderCarrier) Get(key string) string {
	return h.header.Get(key)
}

func (h *httpHeaderCarrier) Set(key, val string) {
	h.header.Set(key, val)
}

func (h *httpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h.header))
	for k := range h.header {
		keys = append(keys, k)
	}
	return keys
}
